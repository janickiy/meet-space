package sfu

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/media"
)

// newLifecycleManager creates an isolated SFU without external ICE servers or fixed ports.
// t owns shutdown so failed assertions cannot leave peer workers behind.
func newLifecycleManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), EgressQueueSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.Shutdown(ctx); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	return m
}

// lifecycleBinding supplies synthetic server-side identities for one local peer.
// conferenceID groups peers into the same isolated room.
func lifecycleBinding(conferenceID string) media.Binding {
	return media.Binding{ConferenceID: conferenceID, ParticipantID: uuid.NewString(), SessionID: uuid.NewString(), ConnectionID: uuid.NewString(), UserID: uuid.NewString()}
}

// TestRepeatedJoinLeaveReleasesResources checks registry ownership and worker completion.
// Heap samples are diagnostic only: runtime caches are not evidence of a leak by themselves.
func TestRepeatedJoinLeaveReleasesResources(t *testing.T) {
	m := newLifecycleManager(t)
	roomID := uuid.NewString()
	for batch := range 4 {
		for range 20 {
			view, err := m.Join(context.Background(), lifecycleBinding(roomID))
			if err != nil {
				t.Fatal(err)
			}
			p, err := m.get(view.MediaPeerID)
			if err != nil {
				t.Fatal(err)
			}
			if err := m.Leave(context.Background(), view.MediaPeerID); err != nil {
				t.Fatal(err)
			}
			select {
			case <-p.ctx.Done():
			default:
				t.Fatal("peer context survived Leave")
			}
			if p.pc.ConnectionState().String() != "closed" {
				t.Fatal("peer transport survived Leave")
			}
			stats := m.Snapshot()
			if stats.Rooms != 0 || stats.Peers != 0 || stats.Tracks != 0 || stats.Subscriptions != 0 {
				t.Fatalf("owned resources survived Leave: %+v", stats)
			}
		}
		runtime.GC()
		time.Sleep(25 * time.Millisecond)
		runtime.GC()
		var memory runtime.MemStats
		runtime.ReadMemStats(&memory)
		t.Logf("batch=%d cycles=%d heap_alloc_bytes=%d heap_objects=%d goroutines=%d", batch+1, (batch+1)*20, memory.HeapAlloc, memory.HeapObjects, runtime.NumGoroutine())
	}
}

// TestConcurrentJoinAndShutdown bounds in-flight registrations during shutdown.
func TestConcurrentJoinAndShutdown(t *testing.T) {
	m := newLifecycleManager(t)
	roomID := uuid.NewString()
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range 20 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			view, err := m.Join(context.Background(), lifecycleBinding(roomID))
			if err == nil {
				_ = m.Leave(context.Background(), view.MediaPeerID)
			} else if !errors.Is(err, media.ErrUnavailable) && !errors.Is(err, media.ErrLimit) {
				t.Errorf("unexpected Join error: %v", err)
			}
		}()
	}
	close(start)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	workers.Wait()
	if stats := m.Snapshot(); stats.Peers != 0 || stats.Rooms != 0 {
		t.Fatalf("shutdown retained registry entries: %+v", stats)
	}
	if _, err := m.Join(context.Background(), lifecycleBinding(roomID)); !errors.Is(err, media.ErrUnavailable) {
		t.Fatalf("Join after shutdown = %v", err)
	}
}

// TestEgressOverflowFailsClosed checks that a slow recorder cannot grow an unbounded queue.
func TestEgressOverflowFailsClosed(t *testing.T) {
	m := newLifecycleManager(t)
	for range 50 {
		sub, err := m.SubscribeRecording(context.Background(), uuid.NewString(), uuid.NewString())
		if err != nil {
			t.Fatal(err)
		}
		e := sub.(*egress)
		// The hello frame occupies one slot; the second ping exceeds the two-slot limit.
		e.Ping()
		e.Ping()
		select {
		case <-e.Done():
		default:
			t.Fatal("overflow did not terminate the egress")
		}
		if !errors.Is(e.Err(), errEgressOverflow) || len(e.frames) != 2 {
			t.Fatalf("overflow error=%v queue=%d", e.Err(), len(e.frames))
		}
		e.Close()
		e.Close()
		if stats := m.Snapshot(); stats.Rooms != 0 || stats.RecordingOutputs != 0 {
			t.Fatalf("closed egress retained ownership: %+v", stats)
		}
	}
	if drops := m.Snapshot().RecordingDrops; drops != 50 {
		t.Fatalf("recording drops=%d, want 50", drops)
	}
}

// TestRecordingEgressIsExclusive keeps the single-recorder lease independent of peers.
func TestRecordingEgressIsExclusive(t *testing.T) {
	m := newLifecycleManager(t)
	conferenceID, recordingID := uuid.NewString(), uuid.NewString()
	sub, err := m.SubscribeRecording(context.Background(), conferenceID, recordingID)
	if err != nil {
		t.Fatal(err)
	}
	frame := <-sub.Frames()
	if frame.Type != "hello" || frame.Sequence != 1 || frame.CapturedAt == 0 {
		t.Fatalf("initial frame = %+v", frame)
	}
	for _, id := range []string{recordingID, uuid.NewString()} {
		if _, err := m.SubscribeRecording(context.Background(), conferenceID, id); !errors.Is(err, media.ErrNegotiation) {
			t.Fatalf("second recording subscription = %v, want negotiation conflict", err)
		}
	}
	if stats := m.Snapshot(); stats.RecordingOutputs != 1 || stats.Rooms != 1 {
		t.Fatalf("active recording stats = %+v", stats)
	}
	sub.Close()
	replacement, err := m.SubscribeRecording(context.Background(), conferenceID, uuid.NewString())
	if err != nil {
		t.Fatalf("subscription after release: %v", err)
	}
	replacement.Close()
	if stats := m.Snapshot(); stats.RecordingOutputs != 0 || stats.Rooms != 0 {
		t.Fatalf("released recording stats = %+v", stats)
	}
}

// TestJoinRejectsRebinding checks that an existing connection cannot change its identity.
func TestJoinRejectsRebinding(t *testing.T) {
	m := newLifecycleManager(t)
	binding := lifecycleBinding(uuid.NewString())
	first, err := m.Join(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := m.Join(context.Background(), binding)
	if err != nil || duplicate.MediaPeerID != first.MediaPeerID {
		t.Fatalf("idempotent Join = %+v, %v", duplicate, err)
	}
	binding.ParticipantID = uuid.NewString()
	if _, err := m.Join(context.Background(), binding); !errors.Is(err, media.ErrUnauthorized) {
		t.Fatalf("identity rebinding accepted: %v", err)
	}
}
