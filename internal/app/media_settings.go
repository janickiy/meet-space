package app

import (
	"context"
	"github.com/janickiy/meet-space/internal/domain/settings"
	"github.com/janickiy/meet-space/internal/infrastructure/sfu"
	"time"
)

// Bounded polling avoids a database query on each peer admission. Any failed
// refresh closes new admissions until the next successful read; existing calls survive.
func startMediaSettings(ctx context.Context, reader settings.Reader, engine *sfu.Manager) <-chan struct{} {
	refresh := func() {
		check, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		values, err := reader.Snapshot(check)
		engine.SetAdmissionLimits(values.MediaMaxPeers, values.MediaMaxRooms, err == nil)
	}
	refresh()
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()
	return done
}
