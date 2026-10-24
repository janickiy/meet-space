// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package media

import "errors"

// Code — это стабильная машиночитаемая ошибка медиапротокола, не связанная с человеческим
// фактором.
type Code string

const (
	ErrUnavailable    Code = "media_unavailable"
	ErrInvalid        Code = "invalid_media_signal"
	ErrUnauthorized   Code = "media_unauthorized"
	ErrOwnership      Code = "media_ownership_lost"
	ErrLimit          Code = "media_limit_exceeded"
	ErrPeerNotFound   Code = "media_peer_not_found"
	ErrNegotiation    Code = "media_negotiation_conflict"
	ErrScreenConflict Code = "screen_sharing_conflict"
	ErrPolicy         Code = "media_policy_blocked"
)

// Error возвращает текст ошибки, требуемый интерфейсом обработки ошибок.
//
// @returns:
//   - result1: Текст ошибки, предоставляемый данной реализацией обработки ошибок.
func (c Code) Error() string { return string(c) }

var protocolErrors = [...]Code{ErrUnavailable, ErrInvalid, ErrUnauthorized, ErrOwnership, ErrLimit, ErrPeerNotFound, ErrNegotiation, ErrScreenConflict, ErrPolicy}

// ErrorCode используется во внутренних и общедоступных пакетах ошибок HTTP и WS.
//
// @Parameters:
//   - err: Ошибка классификации, инкапсуляции или безопасного оповещения.
//
// @returns:
//   - result1: Ограниченный внутренний код сбоя медиа.
func ErrorCode(err error) string {
	for _, code := range protocolErrors {
		if errors.Is(err, code) {
			return string(code)
		}
	}
	return string(ErrUnavailable)
}

// ErrorFromCode принимает только коды протокола; неизвестные данные не передаются.
//
// @Parameters:
//   - value: Значение для проверки, нормализации или преобразования.
//
// @returns:
//   - result1: Соответствующая ошибка протокола или безопасная ошибка недоступности при
//     неизвестном коде.
func ErrorFromCode(value string) error {
	for _, code := range protocolErrors {
		if value == string(code) {
			return code
		}
	}
	return ErrUnavailable
}
