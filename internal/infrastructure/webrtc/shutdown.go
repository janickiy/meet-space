package webrtc

import (
	"context"
	"github.com/janickiy/meet-space/internal/infrastructure/ffmpeg"
	"sync"
	"time"
)

// Shutdown stops new work and waits for the component-owned resources to finish closing.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	if done := m.shutdownDone; done != nil {
		m.mu.Unlock()
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	done := make(chan struct{})
	m.shutdownDone = done
	m.closed = true
	sessions := make([]*session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.sessions = map[string]*session{}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, s := range sessions {
		s.mu.Lock()
		s.failed = true
		s.stopping = true
		pc, process := s.pc, s.process
		startDone := s.startDone
		if s.waitTimer != nil {
			s.waitTimer.Stop()
			s.waitTimer = nil
		}
		s.mu.Unlock()
		if pc != nil {
			_ = pc.Close()
		}
		wg.Add(1)
		go func(s *session, p *ffmpeg.SegmentProcess) {
			defer wg.Done()
			if p != nil {
				_ = p.Stop(2 * time.Second)
			}
			s.cancel()
			if startDone != nil {
				<-startDone
			}
		}(s, process)
	}
	if m.iceConn != nil {
		_ = m.iceConn.Close()
	}
	if m.iceTCP != nil {
		_ = m.iceTCP.Close()
	}
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
