// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package redis

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/realtime"
	goredis "github.com/redis/go-redis/v9"
)

func isolatedRedis(t *testing.T) (*goredis.Client, context.Context, string) {
	t.Helper()
	addr := os.Getenv("MEETSPACE_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("MEETSPACE_TEST_REDIS_ADDR must point to an isolated disposable Redis")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	client, err := NewClient(ctx, addr, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, ctx, "audit:" + uuid.NewString()
}

func TestRealtimePresenceRepeatedJoinLeaveCleansIndexes(t *testing.T) {
	client, ctx, prefix := isolatedRedis(t)
	store := NewRealtimeStore(client, prefix)
	for range 50 {
		now := time.Now().UTC()
		session := realtime.Session{ID: uuid.NewString(), ConnectionID: uuid.NewString(), ConferenceID: uuid.NewString(), UserID: uuid.NewString(), LastSeenAt: now}
		if err := store.Register(ctx, session, time.Minute); err != nil {
			t.Fatal(err)
		}
		if err := store.Register(ctx, session, time.Minute); !errors.Is(err, apperrors.ErrConflict) {
			t.Fatalf("duplicate join = %v", err)
		}
		if err := store.Touch(ctx, session.ConnectionID, time.Now().UTC(), time.Minute); err != nil {
			t.Fatal(err)
		}
		active, err := store.Active(ctx, session.ConferenceID)
		if err != nil || len(active) != 1 {
			t.Fatalf("active=%v err=%v", active, err)
		}
		if err := store.Unregister(ctx, session.ConnectionID); err != nil {
			t.Fatal(err)
		}
		if err := store.Unregister(ctx, session.ConnectionID); err != nil {
			t.Fatal(err)
		}
		if err := store.Touch(ctx, session.ConnectionID, time.Now().UTC(), time.Minute); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("touch after leave=%v", err)
		}
	}
	keys, _, err := client.Scan(ctx, 0, prefix+":*", 1000).Result()
	if err != nil || len(keys) != 0 {
		t.Fatalf("presence retained keys=%v err=%v", keys, err)
	}
}

func TestRealtimeSubscriptionCloseUnblocksReceiver(t *testing.T) {
	client, ctx, prefix := isolatedRedis(t)
	store := NewRealtimeStore(client, prefix)
	for range 25 {
		sub, err := store.Subscribe(ctx)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { _, err := sub.Receive(ctx); done <- err }()
		if err := sub.Close(); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("receiver completed without close error")
			}
		case <-ctx.Done():
			t.Fatal("subscription receiver did not stop")
		}
	}
}

func TestRealtimeTicketConsumedOnceConcurrently(t *testing.T) {
	client, ctx, prefix := isolatedRedis(t)
	store := NewRealtimeStore(client, prefix)
	ticket, conferenceID := strings.Repeat("x", 43), uuid.NewString()
	identity := realtime.Identity{UserID: uuid.NewString(), ExpiresAt: time.Now().Add(time.Minute)}
	if err := store.SaveTicket(ctx, ticket, conferenceID, identity, time.Minute); err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() {
			got, err := store.ConsumeTicket(ctx, ticket, conferenceID)
			if err == nil {
				if got.UserID != identity.UserID {
					t.Error("wrong ticket owner")
				}
				successes.Add(1)
			} else if !errors.Is(err, apperrors.ErrUnauthorized) {
				t.Errorf("consume=%v", err)
			}
		})
	}
	workers.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successful consumers=%d", successes.Load())
	}
}

func TestUserPresenceKeepsOtherDeviceOnline(t *testing.T) {
	client, ctx, prefix := isolatedRedis(t)
	presence := NewUserPresence(client, prefix, time.Minute)
	user := uuid.NewString()
	for _, connection := range []string{"device-one", "device-two"} {
		if _, err := presence.Touch(ctx, user, connection); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := presence.Remove(ctx, user, "device-one"); err != nil || n != 1 {
		t.Fatalf("remaining=%d err=%v", n, err)
	}
	online, err := presence.Online(ctx, []string{user})
	if err != nil || !online[user] {
		t.Fatalf("second device offline: %v %v", online, err)
	}
	if _, err := presence.Remove(ctx, user, "device-two"); err != nil {
		t.Fatal(err)
	}
	online, err = presence.Online(ctx, []string{user})
	if err != nil || online[user] {
		t.Fatalf("last device still online: %v %v", online, err)
	}
}
