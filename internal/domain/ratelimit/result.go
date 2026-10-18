package ratelimit

import (
	"time"
)

// Result содержит решение ограничителя и метаданные бюджета HTTP.  Limit — это бюджет запроса
// на одно окно; Remaining — его неиспользованная часть.  RetryAfter ограничивает время ожидания
// повторной попытки.  При нулевом значении ResetAt не формируется заголовок X-RateLimit-Reset.
type Result struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
	ResetAt    time.Time
}
