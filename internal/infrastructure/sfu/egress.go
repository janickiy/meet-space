package sfu

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/pion/rtp"
)

var errEgressOverflow = errors.New("recording_egress_overflow")

// egress passively observes encoded packets. Queue overflow ends the recording subscription
// without delaying SFU RTP forwarding.
type egress struct {
	audioOnly bool
	manager   *Manager
	room      *room
	id        string
	mu        sync.Mutex
	frames    chan media.EgressFrame
	done      chan struct{}
	err       error
	closed    bool
	sequence  uint64
	tracks    map[string]bool
	closeOnce sync.Once
}

// SubscribeRecording opens a passive bounded subscription to the SFU's encoded media for
// recording.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - recordingID: External recording UUID.
//
// Returns:
//   - result 1: The resulting egress subscription.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) SubscribeRecording(ctx context.Context, conferenceID, recordingID string) (media.EgressSubscription, error) {
	return m.subscribeEgress(ctx, conferenceID, recordingID, false)
}

// SubscribeAudio opens a separate bounded audio-only subscription without consuming the
// recording subscription slot.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - sessionID: Persistent or physical session identifier, as defined by the operation.
//
// Returns:
//   - result 1: The resulting egress subscription.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) SubscribeAudio(ctx context.Context, conferenceID, sessionID string) (media.EgressSubscription, error) {
	return m.subscribeEgress(ctx, conferenceID, sessionID, true)
}

// subscribeEgress reserves at most one room subscription per purpose and binds it to the current
// recording identity.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - recordingID: External recording UUID.
//   - audioOnly: Whether the subscription is restricted to auxiliary audio frames.
//
// Returns:
//   - result 1: The resulting egress subscription.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) subscribeEgress(ctx context.Context, conferenceID, recordingID string, audioOnly bool) (media.EgressSubscription, error) {
	for _, id := range []string{conferenceID, recordingID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil {
			return nil, media.ErrInvalid
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing || m.roomClosures[conferenceID] != nil {
		return nil, media.ErrUnavailable
	}
	r := m.rooms[conferenceID]
	if r == nil {
		if len(m.rooms) >= m.opts.MaxRooms {
			return nil, media.ErrLimit
		}
		r = newRoom(conferenceID)
		m.rooms[conferenceID] = r
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, existing := range r.egresses {
		if id == recordingID || existing.audioOnly == audioOnly {
			return nil, media.ErrNegotiation
		}
	}
	queueSize := m.opts.EgressQueueSize
	if audioOnly {
		queueSize = min(queueSize, 256)
	}
	e := &egress{audioOnly: audioOnly, manager: m, room: r, id: recordingID, frames: make(chan media.EgressFrame, queueSize), done: make(chan struct{}), tracks: map[string]bool{}}
	e.mu.Lock()
	e.enqueueLocked(media.EgressFrame{Type: "hello"})
	e.mu.Unlock()
	for _, t := range r.tracks {
		e.addTrack(t)
		if !audioOnly {
			t.requestPLI()
		}
	}
	r.egresses[recordingID] = e
	return e, nil
}

// Frames returns the subscription channel carrying encoded media frames.
//
// Returns:
//   - result 1: The bounded event or media channel owned by this operation.
func (e *egress) Frames() <-chan media.EgressFrame { return e.frames }

// Done returns the channel closed when the owned process or subscription terminates.
//
// Returns:
//   - result 1: The bounded event or media channel owned by this operation.
func (e *egress) Done() <-chan struct{} { return e.done }

// Err returns the terminal subscription error, if one was recorded.
//
// Returns:
//   - result 1: The owned process or subscription's terminal error, or nil after normal
//     completion.
func (e *egress) Err() error { e.mu.Lock(); defer e.mu.Unlock(); return e.err }

// enqueueLocked enqueues a media frame while holding subscription state and enforcing its queue
// bound.
//
// Parameters:
//   - frame: Encoded media frame with its timing metadata.
func (e *egress) enqueueLocked(frame media.EgressFrame) {
	if e.closed {
		return
	}
	e.sequence++
	frame.Sequence = e.sequence
	if frame.CapturedAt == 0 {
		frame.CapturedAt = time.Now().UnixNano()
	}
	select {
	case e.frames <- frame:
	default:
		if e.audioOnly {
			e.manager.audioTapDropped.Add(1)
		} else {
			e.manager.egressDropped.Add(1)
		}
		e.closed = true
		e.err = errEgressOverflow
		close(e.done)
	}
}

// addTrack adds the encoded source-track description to the recording subscription under its
// state lock.
//
// Parameters:
//   - t: Test handle used for assertions, cleanup and failure reporting.
func (e *egress) addTrack(t *publishedTrack) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.tracks[t.metadata.ID] || !t.permitted.Load() || (e.audioOnly && t.metadata.Kind != media.KindAudio) {
		return
	}
	e.tracks[t.metadata.ID] = true
	codec := t.remote.Codec()
	descriptor := media.EgressTrack{Track: t.metadata, MimeType: codec.MimeType, ClockRate: codec.ClockRate, Channels: codec.Channels, PayloadType: uint8(t.remote.PayloadType()), SSRC: uint32(t.remote.SSRC())}
	e.enqueueLocked(media.EgressFrame{Type: "track", Track: &descriptor, TrackID: t.metadata.ID})
}

// endTrack marks the recording source as ended and releases its active track state.
//
// Parameters:
//   - t: Test handle used for assertions, cleanup and failure reporting.
func (e *egress) endTrack(t *publishedTrack) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.tracks[t.metadata.ID] {
		return
	}
	delete(e.tracks, t.metadata.ID)
	e.enqueueLocked(media.EgressFrame{Type: "track.end", TrackID: t.metadata.ID})
}

// packet copies an encoded packet into the recording subscription's bounded queue.
//
// Parameters:
//   - t: Test handle used for assertions, cleanup and failure reporting.
//   - raw: Untrusted serialized value to decode or validate.
//   - at: Timestamp of the event or state boundary.
func (e *egress) packet(t *publishedTrack, raw []byte, at int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.tracks[t.metadata.ID] || !t.permitted.Load() {
		return
	}
	e.enqueueLocked(media.EgressFrame{Type: "rtp", TrackID: t.metadata.ID, RTP: raw, CapturedAt: at})
}

// fail records a safe media peer failure and starts the owned-resource cleanup path.
//
// Parameters:
//   - err: Error to classify, wrap or report safely.
func (e *egress) fail(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.closed {
		e.err = err
		e.closed = true
		close(e.done)
	}
}

// Close closes the component-owned resources and ends their lifecycle.
func (e *egress) Close() {
	e.closeOnce.Do(func() {
		e.fail(nil)
		e.manager.mu.Lock()
		e.room.mu.Lock()
		if e.room.egresses[e.id] == e {
			delete(e.room.egresses, e.id)
		}
		if len(e.room.peers) == 0 && len(e.room.egresses) == 0 && e.manager.rooms[e.room.id] == e.room {
			delete(e.manager.rooms, e.room.id)
		}
		e.room.mu.Unlock()
		e.manager.mu.Unlock()
	})
}

// Keyframes requests a video keyframe from the recording subscription.
func (e *egress) Keyframes() {
	e.room.mu.Lock()
	defer e.room.mu.Unlock()
	for _, t := range e.room.tracks {
		t.requestPLI()
	}
}

// Ping refreshes the liveness of the current recording subscription.
func (e *egress) Ping() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enqueueLocked(media.EgressFrame{Type: "ping"})
}

// recordPacket copies an encoded packet to the recording egress subscriptions.
//
// Parameters:
//   - t: Test handle used for assertions, cleanup and failure reporting.
//   - packet: Encoded RTP or media packet.
func (m *Manager) recordPacket(t *publishedTrack, packet *rtp.Packet) {
	t.publisher.room.mu.Lock()
	egresses := make([]*egress, 0, len(t.publisher.room.egresses))
	for _, e := range t.publisher.room.egresses {
		if e.audioOnly && t.metadata.Kind != media.KindAudio {
			continue
		}
		egresses = append(egresses, e)
	}
	t.publisher.room.mu.Unlock()
	if len(egresses) == 0 {
		return
	}
	raw, err := packet.Marshal()
	if err != nil {
		return
	}
	at := time.Now().UnixNano()
	for _, e := range egresses {
		e.packet(t, raw, at)
	}
}
