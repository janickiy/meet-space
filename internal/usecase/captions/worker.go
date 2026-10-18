package captions

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/config"
	domain "github.com/janickiy/meet-space/internal/domain/captions"
	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/janickiy/meet-space/internal/domain/realtime"
	"github.com/janickiy/meet-space/internal/operations"
)

// Worker runs a bounded number of conference and track sessions in a process separate from the
// SFU and recorder.
type Worker struct {
	drainMu    sync.Mutex
	draining   bool
	workActive int
	Repo       Repository
	Tap        AudioTap
	Decoder    Decoder
	Provider   domain.LiveTranscriptionProvider
	Events     Events
	Config     config.StageEightConfig
	slots      chan struct{}
	statusMu   sync.Mutex
	statuses   map[string]statusStamp
	active     atomic.Int64
}

// statusStamp limits repeated identical status notifications when many audio frames are dropped.
type statusStamp struct {
	value string
	at    time.Time
}

// BeginDrain stops accepting new work while existing operations finish.
func (w *Worker) BeginDrain() { w.drainMu.Lock(); w.draining = true; w.drainMu.Unlock() }

// Active returns the count of component-owned operations still requiring completion before
// shutdown.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
func (w *Worker) Active() int { w.drainMu.Lock(); defer w.drainMu.Unlock(); return w.workActive }

// finishWork records the worker outcome and releases the active-work accounting.
func (w *Worker) finishWork() { w.drainMu.Lock(); w.workActive--; w.drainMu.Unlock() }

// Run runs live captions until the context is canceled or processing fails.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
func (w *Worker) Run(ctx context.Context) {
	w.slots = make(chan struct{}, w.Config.LiveSessions)
	conferences := make(chan struct{}, w.Config.LiveConferences)
	var work sync.WaitGroup
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer work.Wait()
	if !w.Config.LiveEnabled && !w.Config.AnalyticsEnabled {
		return
	}
	for ctx.Err() == nil {
		select {
		case conferences <- struct{}{}:
			w.drainMu.Lock()
			if w.draining {
				w.drainMu.Unlock()
				<-conferences
				return
			}
			w.workActive++
			w.drainMu.Unlock()
			op, cancel := context.WithTimeout(ctx, 3*time.Second)
			lease, err := w.Repo.Claim(op, w.Config.AnalyticsEnabled, w.Config.LiveConferences, w.Config.LiveAttempts)
			cancel()
			if err != nil {
				w.finishWork()
				<-conferences
			} else {
				work.Add(1)
				go func() {
					defer work.Done()
					defer w.finishWork()
					defer func() { <-conferences }()
					w.conference(ctx, lease)
				}()
			}
		default:
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// conference monitors the live-caption lease and maximum session duration independently of
// incoming audio activity.
//
// Parameters:
//   - parent: Context controlling cancellation, deadlines and operation lifetime.
//   - lease: Current lease identity used to fence stale workers.
func (w *Worker) conference(parent context.Context, lease Lease) {
	deadline := lease.EnabledAt.Add(w.Config.LiveMaxDuration)
	if !lease.Enabled {
		deadline = lease.Origin.Add(w.Config.LiveMaxDuration)
	}
	ctx, cancel := context.WithDeadline(parent, deadline)
	defer cancel()
	defer func() { w.statusMu.Lock(); delete(w.statuses, lease.Token); w.statusMu.Unlock() }()
	var validUntil atomic.Int64
	var interrupted atomic.Bool
	validUntil.Store(time.Now().Add(10 * time.Second).UnixNano())
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				op, stop := context.WithTimeout(ctx, 2*time.Second)
				ok, err := w.Repo.Renew(op, lease)
				stop()
				if err != nil || !ok {
					if err != nil {
						interrupted.Store(true)
					}
					cancel()
					return
				}
				validUntil.Store(time.Now().Add(10 * time.Second).UnixNano())
			}
		}
	}()
	outcome := "failed"
	meter := &activityMeter{}
	for attempt := 0; attempt < w.Config.LiveAttempts && ctx.Err() == nil; attempt++ {
		if attempt > 0 {
			operations.Event("live_reconnect")
			timer := time.NewTimer(time.Duration(attempt) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			break
		}
		err := w.stream(ctx, lease, &validUntil, meter)
		if err == nil || ctx.Err() != nil {
			outcome = "completed"
			break
		}
		w.status(ctx, lease, "degraded")
	}
	if ctx.Err() != nil {
		outcome = "completed"
	}
	if parent.Err() != nil || interrupted.Load() {
		outcome = "queued"
	}
	cancel()
	<-renewDone
	op, stop := context.WithTimeout(context.WithoutCancel(parent), 3*time.Second)
	defer stop()
	_ = w.Repo.Finish(op, lease, outcome)
	w.publish(op, lease.ConferenceID, "caption.status", map[string]any{"sessionId": lease.SessionID, "generation": lease.Generation, "status": outcome, "enabled": lease.Enabled})
}

// stream distributes audio frames to bounded caption queues so overflow does not block
// conference media.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - lease: Current lease identity used to fence stale workers.
//   - validUntil: Conservative deadline after which the current lease cannot be used.
//   - meter: Conference-wide audio activity aggregator.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (w *Worker) stream(ctx context.Context, lease Lease, validUntil *atomic.Int64, meter *activityMeter) error {
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	input, err := w.Tap.Open(child, lease.ConferenceID, lease.SessionID)
	if err != nil {
		return err
	}
	defer input.Close()
	tracks := map[string]chan media.EgressFrame{}
	var work sync.WaitGroup
	defer func() {
		for _, ch := range tracks {
			close(ch)
		}
		cancel()
		input.Close()
		work.Wait()
	}()
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 8192), 64<<10)
	w.status(ctx, lease, "active")
	for scanner.Scan() {
		if child.Err() != nil || time.Now().UnixNano() >= validUntil.Load() {
			return context.Canceled
		}
		var frame media.EgressFrame
		if json.Unmarshal(scanner.Bytes(), &frame) != nil {
			return fmt.Errorf("invalid audio tap")
		}
		switch frame.Type {
		case "track":
			if frame.Track == nil || frame.Track.Kind != media.KindAudio || frame.Track.Source != media.SourceMicrophone {
				continue
			}
			if _, e := uuid.Parse(frame.Track.ID); e != nil {
				return fmt.Errorf("invalid audio track")
			}
			if _, exists := tracks[frame.TrackID]; exists {
				continue
			}
			select {
			case w.slots <- struct{}{}:
			default:
				w.status(ctx, lease, "degraded")
				operations.Event("live_capacity")
				continue
			}
			op, stop := context.WithTimeout(ctx, 2*time.Second)
			speaker, e := w.Repo.Speaker(op, lease.ConferenceID, frame.Track.ParticipantID)
			stop()
			if e != nil {
				<-w.slots
				continue
			}
			ch := make(chan media.EgressFrame, w.Config.LiveQueue)
			tracks[frame.TrackID] = ch
			track := *frame.Track
			work.Add(1)
			go func() {
				defer work.Done()
				operations.State("live_tracks", float64(len(w.slots)))
				defer func() { <-w.slots; operations.State("live_tracks", float64(len(w.slots))) }()
				w.track(child, lease, track, speaker, ch, validUntil, meter)
			}()
		case "rtp":
			if ch := tracks[frame.TrackID]; ch != nil {
				select {
				case ch <- frame:
				default:
					operations.Event("live_audio_dropped")
					w.status(ctx, lease, "degraded")
				}
			}
		case "track.end":
			if ch := tracks[frame.TrackID]; ch != nil {
				close(ch)
				delete(tracks, frame.TrackID)
			}
		case "error":
			return fmt.Errorf("audio tap overflow")
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("audio tap closed")
}

// track decodes one audio track instance without creating a second decoder when the provider
// reconnects.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - lease: Current lease identity used to fence stale workers.
//   - track: Media-track metadata or negotiated track instance.
//   - speaker: Verified speaker identity or public speaker label.
//   - packets: Encoded media-frame input channel.
//   - validUntil: Conservative deadline after which the current lease cannot be used.
//   - meter: Conference-wide audio activity aggregator.
func (w *Worker) track(ctx context.Context, lease Lease, track media.EgressTrack, speaker string, packets <-chan media.EgressFrame, validUntil *atomic.Int64, meter *activityMeter) {
	var first media.EgressFrame
	var ok bool
	select {
	case <-ctx.Done():
		return
	case first, ok = <-packets:
		if !ok {
			return
		}
	}
	// Return the first chunk to the decoder through bounded input without buffering the entire
	// stream.
	input := make(chan media.EgressFrame, 1)
	input <- first
	copyCtx, stopCopy := context.WithCancel(ctx)
	defer stopCopy()
	copyDone := make(chan struct{})
	go func() {
		defer close(copyDone)
		defer close(input)
		for {
			select {
			case <-copyCtx.Done():
				return
			case p, ok := <-packets:
				if !ok {
					return
				}
				select {
				case input <- p:
				case <-copyCtx.Done():
					return
				}
			}
		}
	}()
	var session domain.Session
	var eventDone chan struct{}
	attempts := 0
	retryAt := time.Time{}
	var samples, observed, speech int64
	origin := max(int64(0), time.Unix(0, first.CapturedAt).Sub(lease.Origin).Milliseconds())
	flush := func() {
		if !w.Config.AnalyticsEnabled || observed == 0 && speech == 0 {
			return
		}
		op, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		err := w.Repo.Observe(op, lease, track.ParticipantID, speech, observed)
		cancel()
		if err != nil {
			operations.Event("analytics_failed")
		}
		speech = 0
		observed = 0
	}
	closeSession := func() {
		if session != nil {
			_ = session.Close()
			<-eventDone
			session = nil
			operations.State("live_stt_active", float64(w.active.Add(-1)))
		}
	}
	defer func() { closeSession(); flush(); stopCopy(); <-copyDone }()
	err := w.Decoder.Decode(ctx, track, input, func(pcm []byte) error {
		if time.Now().UnixNano() >= validUntil.Load() {
			return context.Canceled
		}
		ms := int64(len(pcm)) * 1000 / 32000
		elapsed := samples * 1000 / 16000
		if w.Config.AnalyticsEnabled {
			s, o := meter.observe(track.ParticipantID, origin+elapsed, origin+elapsed+ms, domain.AudioActive(pcm))
			observed += o
			speech += s
		}
		if observed >= 1000 || speech >= 1000 {
			flush()
		}
		samples += int64(len(pcm) / 2)
		if !lease.Enabled || !w.Config.LiveEnabled {
			return nil
		}
		if session != nil {
			select {
			case <-eventDone:
				closeSession()
				retryAt = time.Now().Add(time.Second)
				w.status(ctx, lease, "degraded")
			default:
			}
		}
		if session == nil && attempts < w.Config.LiveAttempts && !time.Now().Before(retryAt) {
			attempts++
			runID := uuid.NewString()
			var e error
			session, e = w.Provider.StartSession(ctx, domain.SessionConfig{SessionID: runID, TrackInstanceID: track.ID, Language: lease.Language, SampleRate: 16000, Channels: 1, Format: "pcm_s16le"})
			if e != nil {
				session = nil
				retryAt = time.Now().Add(time.Duration(attempts) * time.Second)
				w.status(ctx, lease, "degraded")
				operations.Event("live_provider_failed")
			} else {
				operations.State("live_stt_active", float64(w.active.Add(1)))
				if attempts > 1 {
					operations.Event("live_reconnect")
				}
				eventDone = make(chan struct{})
				current := session
				done := eventDone
				start := origin + elapsed
				go func() { defer close(done); w.receive(ctx, lease, track, speaker, runID, start, current.Events()) }()
				w.status(ctx, lease, "active")
			}
		}
		if session != nil {
			op, stop := context.WithTimeout(ctx, w.Config.ProviderTimeout)
			e := session.WriteAudio(op, pcm)
			stop()
			if e != nil {
				closeSession()
				retryAt = time.Now().Add(time.Duration(attempts) * time.Second)
				w.status(ctx, lease, "degraded")
				operations.Event("live_provider_failed")
			}
		}
		return nil
	})
	if err != nil && ctx.Err() == nil {
		w.status(ctx, lease, "degraded")
		operations.Event("live_decoder_failed")
	}
}

// receive reads incoming live-caption state events and forwards them to the corresponding
// bounded delivery path.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - lease: Current lease identity used to fence stale workers.
//   - track: Media-track metadata or negotiated track instance.
//   - speaker: Verified speaker identity or public speaker label.
//   - runID: Identifier of the current live-caption processing run.
//   - origin: Media timeline origin in milliseconds.
//   - events: Destination for committed lifecycle or chat events.
func (w *Worker) receive(ctx context.Context, lease Lease, track media.EgressTrack, speaker, runID string, origin int64, events <-chan domain.Event) {
	latest := map[string]domain.Event{}
	for event := range events {
		if !ValidEvent(event, w.Config.MaxCaptionRunes, w.Config.LiveMaxDuration.Milliseconds()) {
			operations.Event("live_invalid_event")
			continue
		}
		old, exists := latest[event.UtteranceID]
		if exists && !Newer(old, event) {
			continue
		}
		if !exists && len(latest) >= w.Config.MaxFinalCaptions {
			operations.Event("live_caption_limit")
			return
		}
		latest[event.UtteranceID] = domain.Event{Revision: event.Revision, Sequence: event.Sequence, Final: event.Final}
		event.StartMS += origin
		event.EndMS += origin
		caption := domain.Caption{ID: uuid.NewSHA1(uuid.MustParse(runID), []byte(event.UtteranceID)).String(), ConferenceID: lease.ConferenceID, SessionID: lease.SessionID, Generation: lease.Generation, ParticipantID: track.ParticipantID, Speaker: speaker, TrackInstanceID: track.ID, Event: event}
		op, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		kind := "caption.partial"
		if event.Final {
			kind = "caption.final"
			saved, err := w.Repo.SaveFinal(op, lease, caption, w.Config.MaxFinalCaptions)
			if err != nil {
				cancel()
				w.status(context.WithoutCancel(ctx), lease, "degraded")
				operations.Event("live_persist_failed")
				return
			}
			caption = saved
		}
		w.publish(op, lease.ConferenceID, kind, caption)
		cancel()
		operations.Observe("caption_latency", max(0, time.Since(lease.Origin.Add(time.Duration(caption.EndMS)*time.Millisecond)).Seconds()))
		metric := "caption_partial"
		if event.Final {
			metric = "caption_final"
		}
		operations.Event(metric)
		operations.Observe(metric, max(0, time.Since(lease.Origin.Add(time.Duration(caption.EndMS)*time.Millisecond)).Seconds()))
	}
}

// ValidEvent checks the live-caption provider event before it is published or saved.
//
// Parameters:
//   - e: The event state processed by this operation.
//   - maximum: Maximum permitted size or number of characters.
//   - duration: Duration or validity window used by the operation.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func ValidEvent(e domain.Event, maximum int, duration int64) bool {
	return e.UtteranceID != "" && len(e.UtteranceID) <= 128 && utf8.ValidString(e.UtteranceID) && !strings.ContainsRune(e.UtteranceID, 0) && utf8.ValidString(e.Text) && !strings.ContainsRune(e.Text, 0) && len(strings.TrimSpace(e.Text)) > 0 && utf8.RuneCountInString(e.Text) <= maximum && e.StartMS >= 0 && e.EndMS >= e.StartMS && e.EndMS <= duration && e.Revision >= 0 && e.Sequence >= 0 && (e.Language == "ru" || e.Language == "en" || e.Language == "auto")
}

// Newer compares caption revisions so stale updates cannot replace newer speech results.
//
// Parameters:
//   - old: Previously accepted event revision.
//   - next: Next HTTP handler or newer event, as defined by its type.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func Newer(old, next domain.Event) bool {
	return (!old.Final || next.Final) && (next.Revision > old.Revision || next.Revision == old.Revision && next.Final && !old.Final) && next.Sequence >= old.Sequence
}

// status persists and publishes a bounded live-caption status without logging the original
// provider failure.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - lease: Current lease identity used to fence stale workers.
//   - status: Lifecycle state or response status to record.
func (w *Worker) status(ctx context.Context, lease Lease, status string) {
	w.statusMu.Lock()
	previous := w.statuses[lease.Token]
	if previous.value == status && time.Since(previous.at) < 5*time.Second {
		w.statusMu.Unlock()
		return
	}
	if w.statuses == nil {
		w.statuses = map[string]statusStamp{}
	}
	w.statuses[lease.Token] = statusStamp{status, time.Now()}
	w.statusMu.Unlock()
	op, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = w.Repo.Status(op, lease, status)
	w.publish(op, lease.ConferenceID, "caption.status", map[string]any{"sessionId": lease.SessionID, "generation": lease.Generation, "status": status, "enabled": lease.Enabled})
}

// publish publishes the saved live-caption state change through its configured event or command
// transport.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - cid: Conference identifier limiting the operation scope.
//   - kind: Validated event, job, media or resource category.
//   - payload: Structured event or command payload.
func (w *Worker) publish(ctx context.Context, cid, kind string, payload any) {
	if w.Events == nil {
		return
	}
	event := realtime.Event(kind, cid, payload)
	if w.Events.Publish(ctx, realtime.Bus{Kind: "event", ConferenceID: cid, Event: &event}) != nil {
		operations.Event("live_delivery_failed")
	}
}
