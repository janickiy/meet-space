package operations

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/janickiy/meet-space/internal/domain/errorlogs"
)

type captureErrors struct {
	mu     sync.Mutex
	events []errorlogs.Event
}

func (c *captureErrors) Record(e errorlogs.Event) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
	return true
}
func TestErrorHandlerPrivacyLevelsAttributesAndStdout(t *testing.T) {
	capture := &captureErrors{}
	reset := SetErrorRecorder(capture)
	defer reset()
	var output bytes.Buffer
	logger := slog.New(NewErrorHandler(slog.NewJSONHandler(&output, nil))).With("service", "record-worker", "instance_id", "worker-1", "release", "test-release")
	logger.Info("normal info", "password", "raw_private_value")
	logger.Warn("expected refusal", "event_type", "access.denied")
	logger.Error("SQL private diagnostic raw_private_value", "code", "recording.failed", "err", "postgres://private_password")
	logger.WithGroup("internal").With("service", "private_password").Error("raw_private_value")
	logger.With("service", "frontend").Warn("client error", "code", "uncaught_error", "version", "frontend-release", "route", "/settings", "browser", "chromium", "stack", []errorlogs.Frame{{File: "index-abcdefgh123.js", Line: 3, Column: 1}})
	if !strings.Contains(output.String(), "raw_private_value") {
		t.Fatal("stdout output was unexpectedly removed")
	}
	if len(capture.events) != 3 {
		t.Fatalf("wrong captured levels: %d", len(capture.events))
	}
	first := capture.events[0]
	if first.Service != "record-worker" || first.Instance != "worker-1" || first.Code != "recording.failed" || first.Message != "Ошибка обработки записи" {
		t.Fatalf("structured attributes lost: %+v", first)
	}
	second := capture.events[1]
	if second.Service != "record-worker" || !strings.HasPrefix(second.Code, "go.error.") {
		t.Fatal("group shadowed service or caller digest lost")
	}
	client := capture.events[2]
	if client.Service != "frontend" || client.Level != "WARN" || client.Code != "client.uncaught_error" || client.Release != "frontend-release" || len(client.Stack) != 1 {
		t.Fatalf("client projection incorrect: %+v", client)
	}
	for _, e := range capture.events {
		if strings.Contains(e.Message, "private") || strings.Contains(e.Route, "private") {
			t.Fatal("private raw diagnostic retained")
		}
	}
}

type controlledWriter struct {
	mu      sync.Mutex
	calls   []errorlogs.Batch
	fail    bool
	entered chan struct{}
}

func (w *controlledWriter) WriteBatch(ctx context.Context, b errorlogs.Batch) error {
	w.mu.Lock()
	w.calls = append(w.calls, b)
	fail := w.fail
	entered := w.entered
	w.mu.Unlock()
	if entered != nil {
		select {
		case entered <- struct{}{}:
		default:
		}
	}
	if fail {
		return errors.New("synthetic unavailable")
	}
	return nil
}
func TestErrorRecorderBoundedNonBlockingAndShutdownFlush(t *testing.T) {
	writer := &controlledWriter{}
	r := NewErrorRecorder(writer)
	began := time.Now()
	accepted := 0
	for range errorlogs.MaxBatchSize + 100 {
		if r.Record(errorlogs.Event{Service: "api", Code: "http.5xx"}) {
			accepted++
		}
	}
	if time.Since(began) > 500*time.Millisecond {
		t.Fatal("logging blocked caller")
	}
	if accepted != errorlogs.MaxBatchSize || r.Stats().Dropped != 100 {
		t.Fatalf("queue not bounded: %+v", r.Stats())
	}
	r.Close()
	r.Close()
	stats := r.Stats()
	if stats.Written != errorlogs.MaxBatchSize || stats.Queued != 0 {
		t.Fatalf("shutdown did not flush queue: %+v", stats)
	}
	if r.Record(errorlogs.Event{}) {
		t.Fatal("closed recorder accepted an event")
	}
}
func TestErrorRecorderRetriesSameBatchIdentity(t *testing.T) {
	writer := &controlledWriter{fail: true, entered: make(chan struct{}, 8)}
	r := NewErrorRecorder(writer)
	r.Record(errorlogs.Event{Code: "http.5xx"})
	for range 2 {
		select {
		case <-writer.entered:
		case <-time.After(4 * time.Second):
			t.Fatal("retry not attempted")
		}
	}
	writer.mu.Lock()
	writer.fail = false
	first, second := writer.calls[0], writer.calls[1]
	writer.mu.Unlock()
	if first.ID == "" || first.ID != second.ID || len(first.Events) != 1 || first.Events[0].ID != second.Events[0].ID {
		t.Fatal("retry changed idempotency identity")
	}
	r.Close()
	if r.Stats().Written != 1 {
		t.Fatal("retry was not flushed")
	}
}
func TestErrorRecorderConcurrentCloseAndRecord(t *testing.T) {
	r := NewErrorRecorder(&controlledWriter{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				r.Record(errorlogs.Event{Code: "http.5xx"})
			}
		}()
	}
	go r.Close()
	wg.Wait()
	r.Close()
	if r.Stats().Queued != 0 {
		t.Fatal("events arrived after shutdown drain")
	}
}
