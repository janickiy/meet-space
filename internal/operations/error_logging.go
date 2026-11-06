package operations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/buildinfo"
	"github.com/janickiy/meet-space/internal/domain/errorlogs"
)

type ErrorRecorder interface{ Record(errorlogs.Event) bool }
type errorRecorderHolder struct{ recorder ErrorRecorder }

var errorRecorder atomic.Pointer[errorRecorderHolder]

// SetErrorRecorder installs one process-owned sink. Its reset is conditional, so
// shutdown of an old owner cannot detach a newer collector.
func SetErrorRecorder(recorder ErrorRecorder) func() {
	var next *errorRecorderHolder
	if recorder != nil {
		next = &errorRecorderHolder{recorder: recorder}
	}
	previous := errorRecorder.Swap(next)
	return func() { errorRecorder.CompareAndSwap(next, previous) }
}

type ErrorBatchWriter interface {
	WriteBatch(context.Context, errorlogs.Batch) error
}
type ErrorRecorderStats struct {
	Accepted, Written, Dropped uint64
	Queued                     int
}
type ErrorLogRecorder struct {
	gate                       sync.RWMutex
	context                    context.Context
	cancel                     context.CancelFunc
	writer                     ErrorBatchWriter
	queue                      chan errorlogs.Event
	stop                       chan struct{}
	done                       chan struct{}
	once                       sync.Once
	closed                     atomic.Bool
	accepted, written, dropped atomic.Uint64
}

// NewErrorRecorder never writes on the request or slog caller's goroutine.
// Capacity includes a 256-event queue plus at most one 256-event retry batch.
func NewErrorRecorder(writer ErrorBatchWriter) *ErrorLogRecorder {
	ctx, cancel := context.WithCancel(context.Background())
	r := &ErrorLogRecorder{context: ctx, cancel: cancel, writer: writer, queue: make(chan errorlogs.Event, errorlogs.MaxBatchSize), stop: make(chan struct{}), done: make(chan struct{})}
	go r.run()
	return r
}
func (r *ErrorLogRecorder) Record(e errorlogs.Event) bool {
	r.gate.RLock()
	defer r.gate.RUnlock()
	if r.closed.Load() {
		r.dropped.Add(1)
		return false
	}
	e = errorlogs.Sanitize(e)
	select {
	case r.queue <- e:
		r.accepted.Add(1)
		return true
	default:
		r.dropped.Add(1)
		return false
	}
}
func (r *ErrorLogRecorder) Stats() ErrorRecorderStats {
	return ErrorRecorderStats{Accepted: r.accepted.Load(), Written: r.written.Load(), Dropped: r.dropped.Load(), Queued: len(r.queue)}
}
func (r *ErrorLogRecorder) Close() {
	r.once.Do(func() { r.gate.Lock(); r.closed.Store(true); r.cancel(); close(r.stop); r.gate.Unlock() })
	<-r.done
}
func (r *ErrorLogRecorder) take() errorlogs.Batch {
	b := errorlogs.Batch{ID: uuid.NewString(), Events: make([]errorlogs.Event, 0, errorlogs.MaxBatchSize)}
	for len(b.Events) < errorlogs.MaxBatchSize {
		select {
		case e := <-r.queue:
			b.Events = append(b.Events, e)
		default:
			return b
		}
	}
	return b
}
func (r *ErrorLogRecorder) run() {
	defer close(r.done)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var pending errorlogs.Batch
	attempts := 0
	lastPrune := time.Time{}
	for {
		select {
		case <-r.stop:
			// One shared deadline also bounds flushing two full batches on shutdown.
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			for {
				if len(pending.Events) == 0 {
					pending = r.take()
				}
				if len(pending.Events) == 0 {
					break
				}
				if ctx.Err() != nil || r.writer.WriteBatch(ctx, pending) != nil {
					r.dropped.Add(uint64(len(pending.Events)))
					break
				}
				r.written.Add(uint64(len(pending.Events)))
				pending = errorlogs.Batch{}
			}
			cancel()
			for {
				select {
				case <-r.queue:
					r.dropped.Add(1)
				default:
					return
				}
			}
		case <-tick.C:
			if len(pending.Events) == 0 {
				pending = r.take()
				attempts = 0
			}
			if len(pending.Events) == 0 && time.Since(lastPrune) < time.Minute {
				continue
			}
			ctx, cancel := context.WithTimeout(r.context, 3*time.Second)
			err := r.writer.WriteBatch(ctx, pending)
			cancel()
			if err != nil {
				attempts++
				if attempts >= 5 {
					r.dropped.Add(uint64(len(pending.Events)))
					pending = errorlogs.Batch{}
					attempts = 0
				}
				continue
			}
			lastPrune = time.Now()
			r.written.Add(uint64(len(pending.Events)))
			pending = errorlogs.Batch{}
			attempts = 0
		}
	}
}

type errorHandler struct {
	base   slog.Handler
	attrs  []slog.Attr
	groups []string
}

func NewErrorHandler(base slog.Handler) slog.Handler { return &errorHandler{base: base} }
func (h *errorHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}
func (h *errorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	all := append([]slog.Attr(nil), h.attrs...)
	if len(h.groups) == 0 {
		all = append(all, attrs...)
	}
	return &errorHandler{base: h.base.WithAttrs(attrs), attrs: all, groups: append([]string(nil), h.groups...)}
}
func (h *errorHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &errorHandler{base: h.base.WithGroup(name), attrs: append([]slog.Attr(nil), h.attrs...), groups: append(append([]string(nil), h.groups...), name)}
}
func (h *errorHandler) Handle(ctx context.Context, record slog.Record) error {
	// Keep the original stdout behavior independent of the persistent store.
	err := h.base.Handle(ctx, record)
	sink := errorRecorder.Load()
	if sink == nil {
		return err
	}
	attrs := map[string]slog.Value{}
	for _, a := range h.attrs {
		attrs[a.Key] = a.Value.Resolve()
	}
	if len(h.groups) == 0 {
		record.Attrs(func(a slog.Attr) bool { attrs[a.Key] = a.Value.Resolve(); return true })
	}
	str := func(key string) string {
		v := attrs[key]
		if v.Kind() == slog.KindString {
			return v.String()
		}
		return ""
	}
	code := str("code")
	eventType := str("event_type")
	client := record.Message == "client error" || eventType == "client.error"
	if record.Level < slog.LevelError && !(record.Level >= slog.LevelWarn && client) {
		return err
	}
	event := errorlogs.Event{ID: uuid.NewString(), Service: str("service"), Instance: str("instance_id"), Level: "ERROR", Code: code, Route: str("route"), RequestID: str("request_id"), Release: buildinfo.Current().Version, FirstSeen: record.Time, LastSeen: record.Time}
	if str("release") != "" {
		event.Release = str("release")
	}
	if event.Service == "" {
		event.Service = "api"
	}
	if client {
		event.Level = "WARN"
		if event.Service != "admin-frontend" {
			event.Service = "frontend"
		}
		if !strings.HasPrefix(code, "client.") {
			event.Code = "client." + code
		}
		if str("version") != "" {
			event.Release = str("version")
		}
		event.Browser = str("browser")
		// Type-erased input is decoded only into a fixed, validated asset-position
		// schema; arbitrary stack strings and function names are never retained.
		if v := attrs["stack"]; v.Kind() == slog.KindAny {
			raw, e := json.Marshal(v.Any())
			if e == nil && len(raw) <= 4096 {
				_ = json.Unmarshal(raw, &event.Stack)
			}
		}
	} else if event.Code == "" {
		switch eventType {
		case "server.panic", "http.5xx", "component.error", "dependency.failed", "worker.failed", "recording.failed", "storage.failed", "media.failed", "smtp.failed":
			event.Code = eventType
		default:
			switch record.Message {
			case "panic recovered", "request panic":
				event.Code = "server.panic"
			case "request failed", "administrative request failed":
				event.Code = "http.5xx"
			default:
				location := "unknown"
				if record.PC != 0 {
					frames := runtime.CallersFrames([]uintptr{record.PC})
					frame, _ := frames.Next()
					location = frame.Function + ":" + strconv.Itoa(frame.Line)
				}
				digest := sha256.Sum256([]byte(location))
				event.Code = "go.error." + hex.EncodeToString(digest[:6])
			}
		}
	}
	sink.recorder.Record(errorlogs.Sanitize(event))
	return err
}
