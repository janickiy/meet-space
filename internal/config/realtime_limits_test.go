package config

import (
	"testing"

	"github.com/janickiy/meet-space/internal/domain/media"
)

// TestRealtimeDefaultsAllowBoundedFirefoxSDP verifies realtime defaults allow bounded firefox
// SDP to prevent regressions in realtime limits test.
//
// Parameters:
//   - t: Test handle used for assertions, cleanup and failure reporting.
func TestRealtimeDefaultsAllowBoundedFirefoxSDP(t *testing.T) {
	for _, name := range []string{"WS_MAX_MESSAGE_BYTES", "WS_MAX_OUTBOUND_BYTES", "WS_MAX_SDP_BYTES", "TURN_URLS", "TURN_FORCE_RELAY", "WEBRTC_ICE_SERVERS_JSON"} {
		t.Setenv(name, "")
	}
	cfg, err := LoadRealtime()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SDPBytes != media.MaxSDPBytes || cfg.MessageBytes != 270336 || cfg.OutboundBytes != media.MaxCommandBytes {
		t.Fatalf("inconsistent signal limits: SDP=%d message=%d outbound=%d", cfg.SDPBytes, cfg.MessageBytes, cfg.OutboundBytes)
	}
	if cfg.ICEBytes != 4096 || cfg.MessagesPerSecond != 20 || cfg.Burst != 40 {
		t.Fatal("non-SDP signal protections changed")
	}
}

// TestRealtimePreservesSmallerOperatorLimits verifies realtime preserves smaller operator limits
// to prevent regressions in realtime limits test.
//
// Parameters:
//   - t: Test handle used for assertions, cleanup and failure reporting.
func TestRealtimePreservesSmallerOperatorLimits(t *testing.T) {
	t.Setenv("TURN_URLS", "")
	t.Setenv("TURN_FORCE_RELAY", "")
	t.Setenv("WEBRTC_ICE_SERVERS_JSON", "")
	t.Setenv("WS_MAX_MESSAGE_BYTES", "65536")
	t.Setenv("WS_MAX_OUTBOUND_BYTES", "262144")
	t.Setenv("WS_MAX_SDP_BYTES", "49152")
	cfg, err := LoadRealtime()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SDPBytes != 49152 || cfg.MessageBytes != 65536 || cfg.OutboundBytes != 262144 {
		t.Fatal("operator's tighter caps were overridden")
	}
}
