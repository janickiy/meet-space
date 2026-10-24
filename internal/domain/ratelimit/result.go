// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

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
