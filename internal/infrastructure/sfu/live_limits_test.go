package sfu

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/media"
)

func TestLiveLimitsPreserveCallsAndGateAdmissions(t *testing.T) {
	m := newLifecycleManager(t)
	ctx := context.Background()
	room := uuid.NewString()
	firstBinding := lifecycleBinding(room)
	first, err := m.Join(ctx, firstBinding)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Join(ctx, lifecycleBinding(room))
	if err != nil {
		t.Fatal(err)
	}
	m.SetAdmissionLimits(1, 1, true)
	if m.Snapshot().Peers != 2 {
		t.Fatal("lowering policy disconnected peers")
	}
	if again, err := m.Join(ctx, firstBinding); err != nil || again.MediaPeerID != first.MediaPeerID {
		t.Fatal("idempotent join lost existing peer", err)
	}
	if _, err := m.Join(ctx, lifecycleBinding(room)); !errors.Is(err, media.ErrLimit) {
		t.Fatal("lower peer limit not enforced", err)
	}
	if _, err := m.Join(ctx, lifecycleBinding(uuid.NewString())); !errors.Is(err, media.ErrLimit) {
		t.Fatal("lower room limit not enforced", err)
	}
	if err := m.Leave(ctx, second.MediaPeerID); err != nil {
		t.Fatal(err)
	}
	m.SetAdmissionLimits(2, 1, true)
	if _, err := m.Join(ctx, lifecycleBinding(room)); err != nil {
		t.Fatal("raised limit did not admit peer", err)
	}
	m.SetAdmissionLimits(0, 0, false)
	if _, err := m.Join(ctx, lifecycleBinding(room)); !errors.Is(err, media.ErrUnavailable) {
		t.Fatal("unavailable settings admitted peer", err)
	}
	if m.Snapshot().Peers != 2 || m.NegotiationPeerLimit() < 2 {
		t.Fatal("failed refresh damaged existing calls")
	}
	m.SetAdmissionLimits(3, 2, true)
	if _, err := m.Join(ctx, lifecycleBinding(uuid.NewString())); err != nil {
		t.Fatal("recovery did not restore admission", err)
	}
}

func TestLiveLimitsConcurrentUpdatesAndJoins(t *testing.T) {
	m := newLifecycleManager(t)
	var wait sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			for n := 0; n < 30; n++ {
				if worker == 0 {
					m.SetAdmissionLimits(1+n%10, 2+n%4, true)
					continue
				}
				peer, err := m.Join(context.Background(), lifecycleBinding(uuid.NewString()))
				if err == nil {
					_ = m.Leave(context.Background(), peer.MediaPeerID)
				} else if !errors.Is(err, media.ErrLimit) {
					t.Errorf("join: %v", err)
				}
				_ = m.NegotiationPeerLimit()
				_ = m.SettingsReady()
			}
		}(worker)
	}
	wait.Wait()
	if m.Snapshot().Peers != 0 {
		t.Fatal("peer leaked after concurrent admission")
	}
}
