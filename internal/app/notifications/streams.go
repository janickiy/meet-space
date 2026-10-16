package notificationsapp

import "sync"

// syncCounts ограничивает число SSE-подключений каждого пользователя.
// Общий лимит подключений отдельно контролирует eventStreams.streams.
type syncCounts struct {
	mu     sync.Mutex
	values map[string]int
}

// acquire резервирует слот, если у пользователя меньше четырёх подключений.
func (s *syncCounts) acquire(userID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values[userID] >= 4 {
		return false
	}
	s.values[userID]++
	return true
}

// release освобождает слот, полученный успешным вызовом acquire.
func (s *syncCounts) release(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[userID]--
	if s.values[userID] <= 0 {
		delete(s.values, userID)
	}
}
