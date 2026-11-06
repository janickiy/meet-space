package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/janickiy/meet-space/internal/domain/settings"
	"github.com/janickiy/meet-space/internal/infrastructure/sfu"
)

type changingMediaSettings struct{ calls atomic.Int64 }

func (r *changingMediaSettings) Snapshot(ctx context.Context) (settings.Values, error) {
	if _, ok := ctx.Deadline(); !ok {
		return settings.Values{}, errors.New("missing deadline")
	}
	if r.calls.Add(1) == 1 {
		return settings.Values{}, errors.New("temporary settings outage")
	}
	return settings.Values{MediaMaxPeers: 25, MediaMaxRooms: 3}, nil
}

func TestMediaSettingsPollRecoversAndStops(t *testing.T) {
	engine, err := sfu.NewManager(sfu.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Shutdown(context.Background())
	reader := &changingMediaSettings{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := startMediaSettings(ctx, reader, engine)
	if engine.SettingsReady() {
		t.Fatal("failed initial read left admissions open")
	}
	deadline := time.NewTimer(7 * time.Second)
	defer deadline.Stop()
	check := time.NewTicker(10 * time.Millisecond)
	defer check.Stop()
	for !engine.SettingsReady() {
		select {
		case <-deadline.C:
			t.Fatal("poll did not recover")
		case <-check.C:
		}
	}
	if engine.NegotiationPeerLimit() != 25 {
		t.Fatal("refreshed limit not applied")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("settings poll outlived context")
	}
	calls := reader.calls.Load()
	time.Sleep(20 * time.Millisecond)
	if reader.calls.Load() != calls {
		t.Fatal("read after poll shutdown")
	}
}
