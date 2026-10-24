// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package records

import "time"

// ConferenceStartRequest содержит настройки записи для авторизованной конференции.
type ConferenceStartRequest struct {
	SegmentDurationSec int    `json:"segmentDurationSec,omitempty"`
	Mode               string `json:"mode,omitempty"`
}

// ValidConferenceMode проверяет, поддерживается ли запрошенный режим записи конференции.
//
// @Parameters:
//   - mode: Проверенная операция или режим записи.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func ValidConferenceMode(mode string) bool {
	return mode == ModeComposite || mode == ModeAudioOnly || mode == ModeIndividualTracks || mode == ModeScreenFocus
}

// OutboxCommand обеспечивает надежную доставку записей от PostgreSQL к RabbitMQ с
// использованием версионных повторных попыток.
type OutboxCommand struct {
	ID           int64
	RecordID     string
	CommandType  string
	Reason       string
	ClaimToken   *string
	ClaimedUntil *time.Time
	PublishedAt  *time.Time
	CreatedAt    time.Time
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (OutboxCommand) TableName() string { return "recording_outbox" }

// IsComposite сообщает, относится ли запись к составному конвейеру записи конференций.
//
// @Parameters:
//   - record: Запись и сохраненные метаданные ее жизненного цикла.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func IsComposite(record Record) bool {
	return ValidConferenceMode(record.Mode) || record.SourceType == "conference" || record.PlatformConferenceID != nil
}

// PublicStatus преобразует сохраненный статус записи в состояние жизненного цикла, видимое для
// клиента.
//
// @Parameters:
//   - status: Состояние жизненного цикла или статус ответа на запись.
//
// @returns:
//   - result1: Статус жизненного цикла записи, видимый для клиента.
func PublicStatus(status string) string {
	if status == StatusFinalizing || status == StatusUploading {
		return "processing"
	}
	return status
}
