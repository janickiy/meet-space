// Package liveproviders implements the streaming gateway and an explicitly test-only caption
// source.
package liveproviders

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/janickiy/meet-space/internal/domain/captions"
)

// Provider contains operator-controlled settings and never exposes its endpoint or token to
// clients.
type Provider struct {
	Mode, Endpoint, Token string
	Timeout               time.Duration
}

// StartSession opens one bounded live-speech provider session for the authorized audio stream.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - config: Operator-controlled configuration and processing limits.
//
// Returns:
//   - result 1: The resulting session.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p Provider) StartSession(ctx context.Context, config captions.SessionConfig) (captions.Session, error) {
	if config.SampleRate != 16000 || config.Channels != 1 || config.Format != "pcm_s16le" {
		return nil, captions.ErrUnavailable
	}
	if p.Mode == "mock" {
		c, cancel := context.WithCancel(ctx)
		return &mockSession{ctx: c, cancel: cancel, events: make(chan captions.Event, 8), language: config.Language}, nil
	}
	if p.Mode != "websocket" {
		return nil, captions.ErrUnavailable
	}
	dialer := websocket.Dialer{HandshakeTimeout: p.Timeout, ReadBufferSize: 4096, WriteBufferSize: 4096}
	conn, response, err := dialer.DialContext(ctx, p.Endpoint, http.Header{"Authorization": []string{"Bearer " + p.Token}, "Idempotency-Key": []string{config.SessionID}})
	if response != nil && response.Body != nil {
		response.Body.Close()
	}
	if err != nil {
		return nil, captions.ErrUnavailable
	}
	child, cancel := context.WithCancel(ctx)
	s := &wsSession{ctx: child, cancel: cancel, conn: conn, events: make(chan captions.Event, 32), done: make(chan struct{}), timeout: p.Timeout}
	conn.SetReadLimit(24 << 10)
	_ = conn.SetWriteDeadline(time.Now().Add(p.Timeout))
	if err = conn.WriteJSON(config); err != nil {
		cancel()
		conn.Close()
		return nil, captions.ErrUnavailable
	}
	s.stop = context.AfterFunc(child, func() { conn.Close() })
	go s.read()
	return s, nil
}

// wsSession bounds response size, queue capacity, write time and network goroutine count.
type wsSession struct {
	ctx     context.Context
	cancel  context.CancelFunc
	conn    *websocket.Conn
	events  chan captions.Event
	done    chan struct{}
	timeout time.Duration
	mu      sync.Mutex
	stop    func() bool
}

// read reads incoming stream events until cancellation or terminal failure and enforces the
// destination queue bound.
func (s *wsSession) read() {
	defer close(s.done)
	defer close(s.events)
	defer s.cancel()
	for s.ctx.Err() == nil {
		var event captions.Event
		if s.conn.ReadJSON(&event) != nil {
			return
		}
		timer := time.NewTimer(s.timeout)
		select {
		case s.events <- event:
			timer.Stop()
		case <-timer.C:
			return
		case <-s.ctx.Done():
			timer.Stop()
			return
		}
	}
}

// WriteAudio forwards one bounded audio frame to the active speech session.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - pcm: Input bytes consumed by this operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *wsSession) WriteAudio(ctx context.Context, pcm []byte) error {
	if len(pcm) == 0 || len(pcm) > 32768 || len(pcm)%2 != 0 {
		return captions.ErrUnavailable
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if s.ctx.Err() != nil {
		return captions.ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	deadline := time.Now().Add(s.timeout)
	if at, ok := ctx.Deadline(); ok && at.Before(deadline) {
		deadline = at
	}
	_ = s.conn.SetWriteDeadline(deadline)
	if s.conn.WriteMessage(websocket.BinaryMessage, pcm) != nil {
		return captions.ErrUnavailable
	}
	return nil
}

// Events returns the live event stream or serves its authorized transport adapter.
//
// Returns:
//   - result 1: The bounded event or media channel owned by this operation.
func (s *wsSession) Events() <-chan captions.Event { return s.events }

// Close closes the component-owned resources and ends their lifecycle.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *wsSession) Close() error {
	select {
	case <-s.done:
	default:
		// Ending PCM input lets the gateway flush a final utterance; this wait is always bounded.
		s.mu.Lock()
		_ = s.conn.SetWriteDeadline(time.Now().Add(min(s.timeout, 2*time.Second)))
		err := s.conn.WriteJSON(map[string]string{"type": "end"})
		s.mu.Unlock()
		if err == nil {
			timer := time.NewTimer(min(s.timeout, 2*time.Second))
			select {
			case <-s.done:
			case <-s.ctx.Done():
			case <-timer.C:
			}
			timer.Stop()
		}
	}
	s.cancel()
	s.conn.Close()
	<-s.done
	s.stop()
	return nil
}

// mockSession generates revisable test utterances from the duration of PCM actually received.
type mockSession struct {
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	events   chan captions.Event
	bytes    int64
	language string
	closed   bool
}

// WriteAudio forwards one bounded audio frame to the active speech session.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - pcm: Input bytes consumed by this operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *mockSession) WriteAudio(ctx context.Context, pcm []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || ctx.Err() != nil || s.ctx.Err() != nil {
		return captions.ErrUnavailable
	}
	before := s.bytes / 16000
	s.bytes += int64(len(pcm))
	after := s.bytes / 16000
	for n := before + 1; n <= after; n++ {
		final := n%2 == 0
		id := (n - 1) / 2
		lang := s.language
		if lang == "auto" {
			lang = "ru"
		}
		text := "ТЕСТ: обсуждение"
		if final {
			text = "ТЕСТОВЫЕ СУБТИТРЫ: обсуждение проекта. Это демонстрация."
		}
		if lang == "en" {
			text = "TEST CAPTIONS: discussing the project. Demo only."
		}
		event := captions.Event{UtteranceID: fmt.Sprint(id), Sequence: n, Revision: 1, StartMS: id * 1000, EndMS: n * 500, Text: text, Language: lang, Final: final}
		if final {
			event.Revision = 2
		}
		select {
		case s.events <- event:
		default:
			return captions.ErrUnavailable
		}
	}
	return nil
}

// Events returns the live event stream or serves its authorized transport adapter.
//
// Returns:
//   - result 1: The bounded event or media channel owned by this operation.
func (s *mockSession) Events() <-chan captions.Event { return s.events }

// Close closes the component-owned resources and ends their lifecycle.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *mockSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		if s.bytes%32000 > 0 {
			lang := s.language
			if lang == "auto" {
				lang = "ru"
			}
			text := "ТЕСТОВЫЕ СУБТИТРЫ: конец демонстрационной реплики."
			if lang == "en" {
				text = "TEST CAPTIONS: end of demo utterance."
			}
			e := captions.Event{UtteranceID: fmt.Sprint(s.bytes / 32000), Sequence: (s.bytes / 16000) + 1, Revision: 2, StartMS: (s.bytes / 32000) * 1000, EndMS: s.bytes * 1000 / 32000, Text: text, Language: lang, Final: true}
			select {
			case s.events <- e:
			default:
				s.closed = true
				s.cancel()
				close(s.events)
				return captions.ErrUnavailable
			}
		}
		s.closed = true
		s.cancel()
		close(s.events)
	}
	return nil
}
