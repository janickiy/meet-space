package notificationsapp

import "sync"

// syncCounts ограничивает количество SSE-соединений для каждого аккаунта. eventStreams.streams отдельно обеспечивает соблюдение общего лимита соединений.
type syncCounts struct {
	mu     sync.Mutex
	values map[string]int
}

// acquire резервирует выделенный для каждого пользователя слот подключения до открытия потока.
//
// Параметры:
//   - userID: действующий идентификатор учетной записи, используемый для ограничения доступа.
//
// Возвращаемые значения:
//   - result 1: выполняется ли задокументированное условие операции или ее условное состояние
//     изменено успешно.
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
