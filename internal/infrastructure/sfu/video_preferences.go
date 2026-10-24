// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package sfu

import (
	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/pion/sdp/v3"
)

// receiveVideoPreference управляет только входящим видео от этого однорангового узла.
// Сохранение согласованных отправителей позволяет возобновить прием видео с камеры и экрана, не
// прерывая локальные публикации и аудио, а также не выделяя новых получателей.
//
// @Parameters:
//   - raw: Недоверенное сериализованное значение для декодирования или проверки.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func receiveVideoPreference(raw string) (bool, error) {
	var description sdp.SessionDescription
	if err := description.UnmarshalString(raw); err != nil {
		return false, media.ErrInvalid
	}
	enabled, seen := true, false
	for _, attr := range description.Attributes {
		if attr.Key != "x-meet-receive-video" {
			continue
		}
		if seen || (attr.Value != "0" && attr.Value != "1") {
			return false, media.ErrInvalid
		}
		seen = true
		enabled = attr.Value == "1"
	}
	return enabled, nil
}
