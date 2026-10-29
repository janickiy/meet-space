// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package notifications

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// Payload содержит только ссылки на ресурсы и состояние. Уведомление не раскрывает содержимое
// чата или файлов и не предоставляет доступ к ресурсам.
type Payload struct {
	ConferenceID     string     `json:"conferenceId"`
	ConversationID   string     `json:"conversationId,omitempty"`
	ConversationType string     `json:"conversationType,omitempty"`
	IsReply          bool       `json:"isReply,omitempty"`
	InvitationID     string     `json:"invitationId,omitempty"`
	RecordingID      string     `json:"recordingId,omitempty"`
	MessageID        string     `json:"messageId,omitempty"`
	ScheduledAt      *time.Time `json:"scheduledAt,omitempty"`
	AdmissionState   string     `json:"admissionState,omitempty"`
}

// Notification сохраняет личное событие, ссылки на ресурсы, данные дедупликации, состояние
// чтения и состояние доставки.
type Notification struct {
	ID          string     `json:"id" gorm:"type:uuid;primaryKey"`
	UserID      string     `json:"userId" gorm:"type:uuid"`
	Type        string     `json:"type"`
	Version     int        `json:"version"`
	Payload     Payload    `json:"payload" gorm:"serializer:json;type:jsonb"`
	DedupKey    string     `json:"-"`
	CreatedAt   time.Time  `json:"createdAt"`
	ReadAt      *time.Time `json:"readAt"`
	PublishedAt *time.Time `json:"-"`
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (Notification) TableName() string { return "notifications" }

// Page содержит одну страницу с ограниченным количеством уведомлений и метаданные продолжения.
type Page struct {
	Items       []Notification `json:"items"`
	NextCursor  *string        `json:"nextCursor"`
	UnreadCount int64          `json:"unreadCount"`
}

// Cursor содержит границы CreatedAt и ID следующей страницы с уведомлениями об аккаунте.
type Cursor struct {
	UserID string    `json:"userId"`
	At     time.Time `json:"at"`
	ID     string    `json:"id"`
}

// DecodeCursor декодирует и проверяет непрозрачный курсор для разбивки на страницы.
//
// @Parameters:
//   - value: Значение для проверки, нормализации или преобразования.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: Сконструированный или загруженный курсор.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func DecodeCursor(value, userID string) (*Cursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > 512 {
		return nil, apperrors.ErrInvalidInput
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(value)
	var c Cursor
	if err != nil || json.Unmarshal(b, &c) != nil || c.UserID != userID || c.At.IsZero() {
		return nil, apperrors.ErrInvalidInput
	}
	id, err := uuid.Parse(c.ID)
	if err != nil || id == uuid.Nil {
		return nil, apperrors.ErrInvalidInput
	}
	c.ID = id.String()
	return &c, nil
}

// EncodeCursor кодирует стабильную границу для следующей страницы с результатами.
//
// @Parameters:
//   - n: Количество запрошенных элементов или операций.
//
// @returns:
//   - result1: Непрозрачный курсор для продолжения уведомлений.
func EncodeCursor(n Notification) string {
	b, _ := json.Marshal(Cursor{UserID: n.UserID, At: n.CreatedAt, ID: n.ID})
	return base64.RawURLEncoding.EncodeToString(b)
}
