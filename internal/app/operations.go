package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/janickiy/meet-space/internal/infrastructure/sfu"
	"github.com/janickiy/meet-space/internal/operations"
)

// hostname returns the process host label without exposing configuration secrets.
//
// Returns:
//   - result 1: Bounded process host label.
func hostname() string {
	value, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return value
}

// runProfiling runs the restricted profiling listener when diagnostics are explicitly enabled.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - ops: Process operations and readiness runtime.
func runProfiling(ctx context.Context, ops *operations.Runtime) {
	if err := ops.Profiling(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("profiling unavailable", "event_type", "profiling.failed")
	}
}

// sampleDatabase updates bounded database connection-pool metrics.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - db: Database handle used for the operation or transaction.
func sampleDatabase(ctx context.Context, db *sql.DB) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		stats := db.Stats()
		operations.State("db_open", float64(stats.OpenConnections))
		operations.State("db_in_use", float64(stats.InUse))
		operations.State("db_wait_total", float64(stats.WaitCount))
		operations.State("db_wait_seconds", stats.WaitDuration.Seconds())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// sampleMedia updates bounded media-worker activity metrics.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - engine: Media engine that owns the room and peer resources.
func sampleMedia(ctx context.Context, engine *sfu.Manager) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		s := engine.Snapshot()
		operations.State("media_relay_total", float64(s.RelayConnections))
		operations.State("media_direct_total", float64(s.DirectConnections))
		operations.State("audio_taps", float64(s.AudioTaps))
		operations.State("audio_tap_drops_total", float64(s.AudioTapDrops))
		for k, v := range map[string]float64{"media_rooms": float64(s.Rooms), "media_peers": float64(s.Peers), "media_tracks": float64(s.Tracks), "media_subscriptions": float64(s.Subscriptions), "media_connections_total": float64(s.PeerConnections), "media_failures_total": float64(s.Failures), "rtp_packets_total": float64(s.Packets), "rtp_bytes_total": float64(s.Bytes), "rtp_dropped_total": float64(s.Dropped), "recording_outputs": float64(s.RecordingOutputs), "recording_drops_total": float64(s.RecordingDrops)} {
			operations.State(k, v)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
