// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

// Подписи к пакетам определяют контракт на расшифровку речи в режиме реального времени, не
// зависящий от медиадвижка и провайдера SDK.
package captions

import (
	"context"
	"errors"
	"time"
)

var ErrUnavailable = errors.New("live_provider_unavailable")

// SessionConfig описывает дорожку одного соединения без биометрической идентификации.
// Аудиосигнал подписан PCM16LE, моно, 16000 Гц; время событий отсчитывается от первого сэмпла
// PCM.
type SessionConfig struct {
	SessionID       string `json:"sessionId"`
	TrackInstanceID string `json:"trackInstanceId"`
	Language        string `json:"language"`
	SampleRate      int    `json:"sampleRate"`
	Channels        int    `json:"channels"`
	Format          string `json:"format"`
}

// Event — событие содержит информацию о ревизии высказывания провайдера; окончательный результат не
// может быть заменен частичным результатом.
type Event struct {
	UtteranceID string `json:"utteranceId"`
	Sequence    int64  `json:"sequence"`
	Revision    int64  `json:"revision"`
	StartMS     int64  `json:"startMs"`
	EndMS       int64  `json:"endMs"`
	Text        string `json:"text"`
	Language    string `json:"language"`
	Final       bool   `json:"final"`
}

// LiveTranscriptionProvider открывает сеанс потоковой передачи, при этом адаптер скрывает
// информацию о провайдере SDK.
type LiveTranscriptionProvider interface {
	// StartSession открывает один ограниченный сеанс работы с провайдером для передачи речи в
	// режиме реального времени для авторизованного аудиопотока.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Ограничения конфигурации и обработки, контролируемые оператором.
	//
	// @returns:
	//   - result1: Результирующий сеанс.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	StartSession(context.Context, SessionConfig) (Session, error)
}

// Session — сеанс включает в себя одно внешнее соединение и ограниченный поток результатов.
type Session interface {
	// WriteAudio пересылает один ограниченный аудиокадр в активный речевой сеанс.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Значение в байтах, требуемое по контракту.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	WriteAudio(context.Context, []byte) error
	// Events возвращает поток событий в реальном времени или обслуживает авторизованный
	// транспортный адаптер.
	//
	// @returns:
	//   - result1: Ограниченное событие или медиаканал, принадлежащий этой операции.
	Events() <-chan Event
	// Close закрывает ресурсы, принадлежащие компоненту, и завершает их жизненный цикл.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Close() error
}

// Caption — подпись привязывает текст к подтверждённой сервером информации об участнике и хронологии
// конференции.
type Caption struct {
	Cursor          int64  `json:"cursor"`
	Generation      int64  `json:"generation"`
	ID              string `json:"id"`
	ConferenceID    string `json:"conferenceId"`
	SessionID       string `json:"sessionId"`
	ParticipantID   string `json:"participantId"`
	Speaker         string `json:"speaker"`
	TrackInstanceID string `json:"trackInstanceId"`
	Event
	CreatedAt time.Time `json:"createdAt"`
}

// State — состояние сообщает допущенным участникам, отправляется ли аудиозапись конференции на
// расшифровку.
type State struct {
	ConferenceID         string    `json:"conferenceId"`
	SessionID            string    `json:"sessionId"`
	Enabled              bool      `json:"enabled"`
	Available            bool      `json:"available"`
	CanManage            bool      `json:"canManage"`
	Language             string    `json:"language"`
	Status               string    `json:"status"`
	Generation           int64     `json:"generation"`
	Origin               time.Time `json:"origin"`
	CanonicalRecordingID *string   `json:"canonicalRecordingId,omitempty"`
}
