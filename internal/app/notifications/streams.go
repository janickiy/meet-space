// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package notificationsapp

import "sync"

// syncCounts ограничивает количество SSE-соединений для каждого аккаунта. eventStreams.streams отдельно обеспечивает соблюдение общего лимита соединений.
type syncCounts struct {
	mu     sync.Mutex
	values map[string]int
}

// acquire резервирует выделенный для каждого пользователя слот подключения до открытия потока.
//
// @Parameters:
//   - userID: действующий идентификатор учетной записи, используемый для ограничения доступа.
//
// @returns:
//   - result1: выполняется ли задокументированное условие операции или ее условное состояние
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

// release освобождает только ресурс персональных уведомлений, принадлежащий соответствующей
// операции или токену.
//
// @Parameters:
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
func (s *syncCounts) release(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[userID]--
	if s.values[userID] <= 0 {
		delete(s.values, userID)
	}
}
