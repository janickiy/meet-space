package notificationsapp

import "sync"

// syncCounts bounds each account's SSE connections. eventStreams.streams separately enforces the
// overall connection limit.
type syncCounts struct {
	mu     sync.Mutex
	values map[string]int
}

// acquire reserves a bounded per-user connection slot before the stream is opened.
//
// Parameters:
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (s *syncCounts) acquire(userID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values[userID] >= 4 {
		return false
	}
	s.values[userID]++
	return true
}

// release releases only the personal notification resource owned by the matching operation or
// token.
//
// Parameters:
//   - userID: Acting account identifier used to scope access.
func (s *syncCounts) release(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[userID]--
	if s.values[userID] <= 0 {
		delete(s.values, userID)
	}
}
