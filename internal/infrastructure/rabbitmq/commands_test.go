// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package rabbitmq

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/records"
	"golang.org/x/sync/semaphore"
)

func TestPublisherWaitHonorsContext(t *testing.T) {
	p := &Publisher{gate: semaphore.NewWeighted(1)}
	if err := p.gate.Acquire(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- p.StartRecord(ctx, uuid.NewString(), 5) }()
	select {
	case err := <-done:
		p.gate.Release(1)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("waiting publication error = %v", err)
		}
	case <-time.After(time.Second):
		p.gate.Release(1)
		<-done
		t.Fatal("publication remained blocked after its deadline")
	}
}

func TestPublisherReconnectAndConsumerShutdown(t *testing.T) {
	url := os.Getenv("MEETSPACE_TEST_RABBITMQ_URL")
	if url == "" {
		t.Skip("MEETSPACE_TEST_RABBITMQ_URL must point to an isolated disposable broker")
	}
	name := "audit-" + uuid.NewString()
	options := Options{URL: url, Exchange: name, Queue: name, RoutingKey: name, ConsumerTag: name}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p, err := NewPublisher(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	c, err := NewConsumer(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	received := make(chan records.Command, 2)
	done := make(chan error, 1)
	go func() {
		done <- c.Consume(ctx, func(_ context.Context, command records.Command) error {
			received <- command
			return nil
		})
	}()
	id := uuid.NewString()
	if err := p.StartRecord(ctx, id, 5); err != nil {
		t.Fatal(err)
	}
	// Обрыв принадлежащего тесту соединения имитирует потерю транспорта.
	closeAMQP(p.conn)
	if err := p.StopRecord(ctx, id, "user_request"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"record.start", "record.stop"} {
		select {
		case command := <-received:
			if command.Type != kind || command.RecordID != id {
				t.Fatalf("received %#v, want %s for %s", command, kind, id)
			}
		case <-ctx.Done():
			t.Fatal("recording command was not delivered")
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("consumer did not stop after cancellation")
	}
}

func TestPublisherCanceledRequestKeepsConnection(t *testing.T) {
	url := os.Getenv("MEETSPACE_TEST_RABBITMQ_URL")
	if url == "" {
		t.Skip("MEETSPACE_TEST_RABBITMQ_URL must point to an isolated disposable broker")
	}
	name := "audit-" + uuid.NewString()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p, err := NewPublisher(ctx, Options{URL: url, Exchange: name, Queue: name, RoutingKey: name})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	conn := p.conn
	canceled, stop := context.WithCancel(ctx)
	stop()
	if err := p.StartRecord(canceled, uuid.NewString(), 5); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled publication error = %v", err)
	}
	if conn.IsClosed() {
		t.Fatal("a canceled request closed the healthy shared broker connection")
	}
	if err := p.StartRecord(ctx, uuid.NewString(), 5); err != nil {
		t.Fatalf("next publication failed: %v", err)
	}
	if p.conn != conn {
		t.Fatal("next publication unnecessarily reconnected")
	}
}
