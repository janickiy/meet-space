package websocket

import (
	"bytes"
	"testing"
	"time"

	"github.com/janickiy/meet-space/internal/config"
	domain "github.com/janickiy/meet-space/internal/domain/realtime"
)

// queueClient creates an unconnected client for deterministic queue ownership checks.
func queueClient() *client {
	return newClient(nil, &Handler{cfg: config.RealtimeConfig{QueueSize: 2, OutboundBytes: 1024}}, domain.Session{}, time.Time{})
}

// TestCriticalQueueOverflowStopsClient verifies the slow-consumer bound for signaling.
func TestCriticalQueueOverflowStopsClient(t *testing.T) {
	c := queueClient()
	event := domain.Event("media.answer", "", nil)
	for range 2 {
		if !c.Offer(event) {
			t.Fatal("critical queue rejected an event before reaching capacity")
		}
	}
	if c.Offer(event) {
		t.Fatal("critical queue did not reject the first overflowing event")
	}
	select {
	case <-c.done:
	default:
		t.Fatal("critical overflow did not close the client")
	}
	if c.reason != "slow_client" || len(c.out) != 2 {
		t.Fatalf("reason=%q queue=%d", c.reason, len(c.out))
	}
	if c.Offer(event) {
		t.Fatal("closed client accepted another event")
	}
}

// TestLowPriorityOverflowDoesNotStopMedia verifies recoverable chat events stay bounded.
func TestLowPriorityOverflowDoesNotStopMedia(t *testing.T) {
	c := queueClient()
	event := domain.Event("chat.message.created", "", nil)
	if !domain.LowPriorityEvent(event.Type) {
		t.Fatal("fixture is not a recoverable event")
	}
	for range 1000 {
		if !c.Offer(event) {
			t.Fatal("low-priority overflow stopped the client")
		}
	}
	select {
	case <-c.done:
		t.Fatal("recoverable overflow terminated media signaling")
	default:
	}
	if len(c.low) != cap(c.low) || len(c.out) != 0 {
		t.Fatalf("unexpected queue sizes low=%d critical=%d", len(c.low), len(c.out))
	}
}

// TestOversizedOutboundEventStopsClient verifies the server-side allocation bound.
func TestOversizedOutboundEventStopsClient(t *testing.T) {
	c := queueClient()
	if c.Offer(domain.Envelope{Type: "media.answer", Data: bytes.Repeat([]byte{'x'}, 1025)}) {
		t.Fatal("oversized payload accepted")
	}
	if c.reason != "outbound_too_large" || len(c.out) != 0 {
		t.Fatalf("reason=%q queue=%d", c.reason, len(c.out))
	}
}
