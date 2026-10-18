package recorder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/janickiy/meet-space/internal/domain/records"
	"github.com/janickiy/meet-space/internal/infrastructure/composite"
	"github.com/janickiy/meet-space/internal/infrastructure/ffmpeg"
	s3storage "github.com/janickiy/meet-space/internal/infrastructure/storage/s3"
	"github.com/janickiy/meet-space/internal/operations"
)

// CompositeRepository defines the replaceable dependency contract for recording jobs and their
// artifacts.
type CompositeRepository interface {
	// FindByUUID loads a recording by its public UUID.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: External recording UUID.
	//
	// Returns:
	//   - result 1: The resulting record.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	FindByUUID(context.Context, string) (records.Record, error)
	// ListActiveComposite returns active composite recordings so the worker can reconcile or recover
	// processing.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: The collected record values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ListActiveComposite(context.Context) ([]records.Record, error)
	// ClaimComposite claims a versioned composite-recording lease for one worker.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Signed credential or operation-ownership token to verify.
	//   - argument 4: Identity of the worker owning this operation.
	//   - argument 5: Lifetime assigned to the temporary value or ownership record.
	//
	// Returns:
	//   - result 1: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ClaimComposite(context.Context, string, string, string, time.Duration) (bool, error)
	// RenewComposite renews a composite-recording lease only when its worker and lease version still
	// match.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Signed credential or operation-ownership token to verify.
	//   - argument 4: Lifetime assigned to the temporary value or ownership record.
	//
	// Returns:
	//   - result 1: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	RenewComposite(context.Context, string, string, time.Duration) (bool, error)
	// ReleaseComposite releases only the currently owned composite-recording lease.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Signed credential or operation-ownership token to verify.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ReleaseComposite(context.Context, string, string) error
	// TransitionComposite changes composite-recording state only for the current fenced lease.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Signed credential or operation-ownership token to verify.
	//   - argument 4: Lifecycle state or response status to record.
	//   - argument 5: Underlying error that caused the operation to fail.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	TransitionComposite(context.Context, string, string, string, error) error
	// SaveCompositeArtifacts commits composite artifacts only for the recording generation still
	// owned by the worker.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Signed credential or operation-ownership token to verify.
	//   - argument 4: Whether this is the final chunk and the archive must be closed.
	//   - argument 5: Optional preview-file metadata associated with the recording.
	//   - argument 6: Ordered transcript or media segments.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SaveCompositeArtifacts(context.Context, string, string, records.RecordFile, *records.RecordFile, []records.RecordSegment) error
	// MarkStopping conditionally moves a recording to the stopping state without overwriting a newer
	// lifecycle transition.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: External recording UUID.
	//   - argument 3: Bounded lifecycle or failure reason.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	MarkStopping(context.Context, string, string) error
	// AddEvent appends a persistent recording lifecycle diagnostic event.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: External recording UUID.
	//   - argument 3: Bounded recording lifecycle event category.
	//   - argument 4: Media or content source belonging to the current operation.
	//   - argument 5: Bounded diagnostic severity.
	//   - argument 6: Persisted or outgoing chat message.
	//   - argument 7: Identity of the worker owning this operation.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	AddEvent(context.Context, string, string, string, string, string, string) error
}

// CompositeRegistry defines the replaceable dependency contract for recording jobs and their
// artifacts.
type CompositeRegistry interface {
	// GetOwner loads the current distributed owner and fencing version of the media room.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//
	// Returns:
	//   - result 1: The resulting route.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	GetOwner(context.Context, string) (media.Route, error)
}

// CompositeOptions contains the dependencies and settings of conference composite recording.
type CompositeOptions struct {
	Repository     CompositeRepository
	Registry       CompositeRegistry
	S3             *s3storage.Client
	StoragePath    string
	FFmpegPath     string
	WorkerID       string
	InternalSecret string
	Config         config.CompositeConfig
	ConferenceLock conferenceReleaser
	// Invoke Publish after committing the lifecycle transition. It must bound I/O and must not
	// reenter this service.
	Publish func(context.Context, records.Record, string) error
	Logger  *log.Logger
}

// CompositeService manages recording independently of AMQP delivery time. A database lease
// protects ingestion, FFmpeg processing and artifact publication.
type CompositeService struct {
	o         CompositeOptions
	composer  *composite.Composer
	processor *ffmpeg.PostProcessor
	artifacts compositeArtifactPublisher
	client    *http.Client
	mu        sync.Mutex
	ctx       context.Context
	running   map[string]struct{}
	wg        sync.WaitGroup
	started   bool
	draining  bool
}

// NewCompositeService constructs the composite service with the dependencies and limits required
// for recording jobs and their artifacts.
//
// Parameters:
//   - o: Component dependencies and construction options.
//
// Returns:
//   - result 1: The constructed or loaded composite service.
func NewCompositeService(o CompositeOptions) *CompositeService {
	if o.Logger == nil {
		o.Logger = log.Default()
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = 5 * time.Second
	artifacts := compositeArtifactPublisher{repository: o.Repository, maxBytes: o.Config.MaxBytes}
	if o.S3 != nil {
		artifacts.storage = o.S3
	}
	return &CompositeService{o: o, composer: composite.NewComposer(o.FFmpegPath, o.Config.Width, o.Config.Height, o.Config.FPS, o.Config.Concurrency), processor: ffmpeg.NewPostProcessor(o.FFmpegPath), artifacts: artifacts, client: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, ctx: context.Background(), running: map[string]struct{}{}}
}

// Start starts recording jobs and their artifacts and prepares the resources owned by that
// operation.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
func (s *CompositeService) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.ctx = ctx
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.o.Config.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.reconcile(ctx)
			}
		}
	}()
}

// Wait waits for component-owned background tasks and releases their remaining transport
// resources.
func (s *CompositeService) Wait() { s.wg.Wait(); s.client.CloseIdleConnections() }

// Active returns the count of component-owned operations still requiring completion before
// shutdown.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
func (s *CompositeService) Active() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.running) }

// BeginDrain stops accepting new work while existing operations finish.
func (s *CompositeService) BeginDrain() { s.mu.Lock(); s.draining = true; s.mu.Unlock() }

// WaitContext bounds the wait for previously stopped tasks by the supplied deadline; the owner
// must request cancellation before calling it.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *CompositeService) WaitContext(ctx context.Context) error {
	done := make(chan struct{})
	go func() { s.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// HandleCommand idempotently handles an internal recording-start or recording-stop command.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - command: Validated internal recording or media command.
//   - record: Recording and its persisted lifecycle metadata.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *CompositeService) HandleCommand(ctx context.Context, command records.Command, record records.Record) error {
	switch command.Type {
	case "record.start":
		if records.IsTerminalStatus(record.Status) {
			return nil
		}
		return s.launch(ctx, record)
	case "record.stop":
		if records.IsTerminalStatus(record.Status) {
			return nil
		}
		if record.Status == records.StatusStarting || record.Status == records.StatusRecording {
			if err := s.o.Repository.MarkStopping(ctx, record.UUID, command.Reason); err != nil && !errors.Is(err, records.ErrRecordStateChanged) {
				return err
			}
			record.Status = records.StatusStopping
			s.publish(ctx, record, "recording.stopping")
		}
		// A command may reach another replica. The owner observes stopping by polling the database;
		// start is permitted only when no current owner exists.
		return s.launch(ctx, record)
	default:
		return fmt.Errorf("unknown composite command")
	}
}

// reconcile reconciles persisted recording state with currently owned resources after failures
// or restart.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
func (s *CompositeService) reconcile(ctx context.Context) {
	queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	records, err := s.o.Repository.ListActiveComposite(queryCtx)
	if err != nil {
		return
	}
	for _, record := range records {
		if err := s.launch(queryCtx, record); err != nil {
			s.o.Logger.Printf("composite reconciliation record=%s failed", record.UUID)
		}
	}
}

// launch starts composite-recording processing only after its fenced lease has been acquired.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - record: Recording and its persisted lifecycle metadata.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *CompositeService) launch(ctx context.Context, record records.Record) error {
	s.mu.Lock()
	if _, ok := s.running[record.UUID]; ok || s.draining || len(s.running) >= s.o.Config.MaxActive || s.ctx.Err() != nil {
		s.mu.Unlock()
		return nil
	}
	s.running[record.UUID] = struct{}{}
	workParent := s.ctx
	s.wg.Add(1)
	s.mu.Unlock()
	token := uuid.NewString()
	claimed, err := s.o.Repository.ClaimComposite(ctx, record.UUID, token, s.o.WorkerID, s.o.Config.LeaseTTL)
	if err != nil || !claimed {
		s.mu.Lock()
		delete(s.running, record.UUID)
		s.mu.Unlock()
		s.wg.Done()
		return err
	}
	go func() {
		defer s.wg.Done()
		defer func() { s.mu.Lock(); delete(s.running, record.UUID); s.mu.Unlock() }()
		workCtx, cancel := context.WithCancel(workParent)
		defer cancel()
		renewed := make(chan struct{})
		go func() {
			defer close(renewed)
			ticker := time.NewTicker(s.o.Config.LeaseTTL / 4)
			defer ticker.Stop()
			for {
				select {
				case <-workCtx.Done():
					return
				case <-ticker.C:
					checkCtx, done := context.WithTimeout(workCtx, s.o.Config.LeaseTTL/8)
					ok, err := s.o.Repository.RenewComposite(checkCtx, record.UUID, token, s.o.Config.LeaseTTL)
					done()
					if err != nil || !ok {
						cancel()
						return
					}
				}
			}
		}()
		err := s.run(workCtx, record, token)
		if err != nil {
			s.fail(record, token, err)
		}
		cancel()
		<-renewed
		releaseCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		_ = s.o.Repository.ReleaseComposite(releaseCtx, record.UUID, token)
	}()
	return nil
}

// run runs recording jobs and their artifacts until the context is canceled or processing fails.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - record: Recording and its persisted lifecycle metadata.
//   - token: Signed credential or operation-ownership token to verify.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *CompositeService) run(ctx context.Context, record records.Record, token string) error {
	started := time.Now()
	defer func() { operations.Observe("recording", time.Since(started).Seconds()) }()
	dir := filepath.Join(s.o.StoragePath, "records", record.UUID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if record.Status == records.StatusRecording || (record.Status == records.StatusStarting && record.RecorderToken != nil) {
		return fmt.Errorf("recorder interrupted; completed chunks retained for recovery")
	}
	if record.Status == records.StatusStarting {
		if err := s.capture(ctx, record, token, dir); err != nil {
			return err
		}
	}
	current, err := s.o.Repository.FindByUUID(ctx, record.UUID)
	if err != nil {
		return err
	}
	if current.Status != records.StatusStopping && current.Status != records.StatusFinalizing && current.Status != records.StatusUploading {
		return fmt.Errorf("recording input ended without a stop transition")
	}
	if err := s.o.Repository.TransitionComposite(ctx, record.UUID, token, records.StatusFinalizing, nil); err != nil {
		return err
	}
	current.Status = records.StatusFinalizing
	s.publish(ctx, current, "recording.processing")
	if err := s.composer.Recover(ctx, dir); err != nil {
		return err
	}
	finalizationStart := time.Now()
	var result ffmpeg.Result
	audioOnly := record.Mode == records.ModeAudioOnly || record.Mode == records.ModeIndividualTracks
	if audioOnly {
		result, err = s.processor.FinalizeAudio(ctx, dir)
	} else {
		result, err = s.processor.FinalizeComposite(ctx, dir)
	}
	operations.Observe("finalization", time.Since(finalizationStart).Seconds())
	if err != nil {
		return err
	}
	if err := s.o.Repository.TransitionComposite(ctx, record.UUID, token, records.StatusUploading, nil); err != nil {
		return err
	}
	publication := s.artifacts.begin(compositeArtifactCommand{record: record, token: token, dir: dir, result: result})
	defer publication.Close()
	if err := publication.Publish(ctx); err != nil {
		return err
	}
	current, err = s.o.Repository.FindByUUID(ctx, record.UUID)
	operations.Event("recording_ready")
	operations.Event("recording_mode_" + record.Mode)
	if err == nil {
		s.publish(ctx, current, "recording.ready")
	}
	if s.o.ConferenceLock != nil {
		_ = s.o.ConferenceLock.Release(ctx, record.ConferenceID, record.UUID)
	}
	_ = s.o.Repository.AddEvent(ctx, record.UUID, "recording.ready", "recorder", "info", "Validated composite MP4 and preview uploaded", s.o.WorkerID)
	// Retain segments from failed or interrupted recordings. After the lease-protected ready commit,
	// private MinIO is authoritative and successful temporary files may be removed. KEEP_LOCAL is
	// for diagnostics and acceptance checks.
	if !s.o.Config.KeepLocal {
		if err := os.RemoveAll(dir); err != nil {
			s.o.Logger.Printf("composite local cleanup failed record=%s", record.UUID)
		}
	}
	return nil
}

// capture archives the encoded media stream for durable recovery and later composition.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - record: Recording and its persisted lifecycle metadata.
//   - token: Signed credential or operation-ownership token to verify.
//   - dir: Validated local directory.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *CompositeService) capture(ctx context.Context, record records.Record, token, dir string) error {
	captureCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	route, err := s.o.Registry.GetOwner(captureCtx, record.ConferenceID)
	if err != nil {
		return fmt.Errorf("conference media worker unavailable")
	}
	endpoint, err := url.Parse(route.Endpoint)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil {
		return fmt.Errorf("invalid media worker endpoint")
	}
	seconds := record.SegmentDurationSec
	if seconds < 1 || seconds > 30 {
		seconds = 5
	}
	request := media.EgressRequest{RequestID: uuid.NewString(), ConferenceID: record.ConferenceID, RecordingID: record.UUID, Route: route, SegmentDurationSec: seconds}
	data, _ := json.Marshal(request)
	httpRequest, err := http.NewRequestWithContext(captureCtx, http.MethodPost, strings.TrimRight(route.Endpoint, "/")+"/internal/media/egress", bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+s.o.InternalSecret)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("X-Request-ID", request.RequestID)
	response, err := s.client.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("connect recording egress failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("recording egress rejected (%d)", response.StatusCode)
	}
	monitored := make(chan struct{})
	input := &activityReader{reader: response.Body}
	input.lastRead.Store(time.Now().UnixNano())
	go func() {
		defer close(monitored)
		ticker := time.NewTicker(s.o.Config.PollInterval)
		defer ticker.Stop()
		deadline := time.NewTimer(s.o.Config.MaxDuration)
		defer deadline.Stop()
		for {
			select {
			case <-captureCtx.Done():
				return
			case <-deadline.C:
				queryCtx, done := context.WithTimeout(captureCtx, 3*time.Second)
				_ = s.o.Repository.MarkStopping(queryCtx, record.UUID, "duration_limit")
				done()
				cancel()
				return
			case <-ticker.C:
				if time.Since(time.Unix(0, input.lastRead.Load())) > 15*time.Second {
					cancel()
					return
				}
				queryCtx, done := context.WithTimeout(captureCtx, 3*time.Second)
				current, err := s.o.Repository.FindByUUID(queryCtx, record.UUID)
				done()
				if err != nil {
					cancel()
					return
				}
				if current.Status == records.StatusStopping || records.IsTerminalStatus(current.Status) {
					cancel()
					return
				}
			}
		}
	}()
	err = s.composer.Capture(captureCtx, ctx, input, dir, composite.CaptureOptions{Mode: record.Mode, SegmentDuration: time.Duration(seconds) * time.Second, MaxBytes: s.o.Config.MaxBytes, OnStarted: func() error {
		if err := s.o.Repository.TransitionComposite(ctx, record.UUID, token, records.StatusRecording, nil); err != nil {
			if errors.Is(err, records.ErrRecordStateChanged) {
				current, readErr := s.o.Repository.FindByUUID(ctx, record.UUID)
				if readErr == nil && current.Status == records.StatusStopping {
					cancel()
					return nil
				}
			}
			return err
		}
		current, err := s.o.Repository.FindByUUID(ctx, record.UUID)
		if err != nil {
			return err
		}
		s.publish(ctx, current, "recording.started")
		return nil
	}})
	cancel()
	<-monitored
	if composite.OnlyEgressEnded(err) {
		current, readErr := s.o.Repository.FindByUUID(ctx, record.UUID)
		if readErr == nil && current.Status == records.StatusStopping {
			return nil
		}
	}
	return err
}

// activityReader holds the activity reader data used by recording jobs and their artifacts.
type activityReader struct {
	reader   io.Reader
	lastRead atomic.Int64
}

// Read copies at most the destination buffer size from the owned stream while preserving its
// read and cancellation limits.
//
// Parameters:
//   - p: Destination byte buffer filled from the stream.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *activityReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.lastRead.Store(time.Now().UnixNano())
	}
	return n, err
}

// Close closes the component-owned resources and ends their lifecycle.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *activityReader) Close() error {
	if closer, ok := r.reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// fail records a safe recording job failure and starts the owned-resource cleanup path.
//
// Parameters:
//   - record: Recording and its persisted lifecycle metadata.
//   - token: Signed credential or operation-ownership token to verify.
//   - cause: Underlying error that caused the operation to fail.
func (s *CompositeService) fail(record records.Record, token string, cause error) {
	operations.Event("recording_failed")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Keep full FFmpeg diagnostics local; clients receive only a bounded safe message.
	s.o.Logger.Printf("composite record=%s failed: %v", record.UUID, cause)
	//lint:ignore ST1005 Сообщение клиенту сохраняет существующий текст ошибки.
	safe := errors.New("Conference recording failed; completed segments were retained")
	if err := s.o.Repository.TransitionComposite(ctx, record.UUID, token, records.StatusFailed, safe); err != nil {
		return
	}
	current, err := s.o.Repository.FindByUUID(ctx, record.UUID)
	if err == nil {
		s.publish(ctx, current, "recording.failed")
	}
	if s.o.ConferenceLock != nil {
		_ = s.o.ConferenceLock.Release(ctx, record.ConferenceID, record.UUID)
	}
	_ = s.o.Repository.AddEvent(ctx, record.UUID, "recording.failed", "recorder", "error", safe.Error(), s.o.WorkerID)
}

// publish publishes the saved recording job change through its configured event or command
// transport.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - record: Recording and its persisted lifecycle metadata.
//   - event: Trusted event to publish or process.
func (s *CompositeService) publish(ctx context.Context, record records.Record, event string) {
	if s.o.Publish == nil {
		return
	}
	publishCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := s.o.Publish(publishCtx, record, event); err != nil {
		s.o.Logger.Printf("recording event publish failed record=%s event=%s", record.UUID, event)
	}
}
