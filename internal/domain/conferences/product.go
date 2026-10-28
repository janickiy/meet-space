// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package conferences

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// IsRejectedOrKicked определяет, было ли членство отозвано до повторного вступления в
// конференцию или получения приглашения. Состояния ожидания, отсутствия и неизвестности не
// являются основанием для отзыва членства; проверка на допуск проводится отдельно.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (p Participant) IsRejectedOrKicked() bool {
	return p.Status == Kicked || p.Status == Rejected || p.AdmissionState == AdmissionKicked || p.AdmissionState == AdmissionRejected
}

// IsAdmitted сообщает, позволяет ли сохраненное членство участвовать в конференции.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (p Participant) IsAdmitted() bool {
	return (p.AdmissionState == AdmissionAdmitted || p.AdmissionState == "") && p.Status != Waiting && p.Status != Rejected && p.Status != Kicked
}

// CanParticipate проверяет, позволяет ли членство и статус участника в конференции принимать
// активное участие.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (p Participant) CanParticipate() bool { return p.IsAdmitted() && p.Status == Joined }

// CanReadHistory проверяет, позволяет ли членство получать доступ к истории конференции.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (p Participant) CanReadHistory() bool {
	return p.IsAdmitted() && (p.Status == Joined || p.Status == Left)
}

// CanAdmit проверяет, есть ли у участника право принимать решения о допуске в зал ожидания.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (p Participant) CanAdmit() bool {
	return p.CanParticipate() && (p.Role == Owner || p.Role == CoHost)
}

// AdmissionRequest содержит решение организатора о допуске или отказе в допуске участника,
// ожидающего своей очереди.
type AdmissionRequest struct {
	Decision string `json:"decision"`
}

// Validate отклоняет несогласованные или выходящие за пределы допустимых значений поля
// конференции до того, как значение будет использовано.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r AdmissionRequest) Validate() error {
	if r.Decision != "admit" && r.Decision != "reject" {
		return apperrors.New(apperrors.ErrInvalidInput, "decision must be admit or reject")
	}
	return nil
}

// ScheduleRequest содержит однозначное время начала и, при необходимости, запланированную
// продолжительность.
type ScheduleRequest struct {
	ScheduledAt        time.Time `json:"scheduledAt"`
	PlannedDurationMin *int      `json:"plannedDurationMin"`
}

// ValidateSchedule проверяет запрошенное время начала и окончания перед тем, как конференция
// будет запланирована.
//
// @Parameters:
//   - at: Метка времени события или границы состояния.
//   - duration: Продолжительность или период действия, используемый в ходе операции.
//   - now: Время, используемое для расчета срока действия или продолжительности.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func ValidateSchedule(at *time.Time, duration *int, now time.Time) error {
	if duration != nil && (*duration < 1 || *duration > 1440) {
		return apperrors.New(apperrors.ErrInvalidInput, "plannedDurationMin must be between 1 and 1440")
	}
	if at == nil {
		if duration != nil {
			return apperrors.New(apperrors.ErrInvalidInput, "planned duration requires scheduledAt")
		}
		return nil
	}
	if at.IsZero() || !at.After(now) || at.Year() > 9999 {
		return apperrors.New(apperrors.ErrInvalidInput, "scheduledAt must be a future RFC3339 instant with an explicit timezone")
	}
	return nil
}

// TimelineQuery содержит серверные фильтры конференции для учетной записи и курсор пагинации.
type TimelineQuery struct {
	View   string
	Scope  string
	Sort   string
	Status Status
	From   *time.Time
	To     *time.Time
	Cursor string
	Limit  int
}

// TimelineCursor содержит дату и UUID границы страницы со списком конференций.
type TimelineCursor struct {
	At   time.Time `json:"at"`
	ID   string    `json:"id"`
	Sort string    `json:"sort,omitempty"`
}

// Validate отклоняет несогласованные или выходящие за пределы допустимых значений поля
// конференции до того, как значение будет использовано.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (q TimelineQuery) Validate() error {
	if q.View != "upcoming" && q.View != "active" && q.View != "past" {
		return apperrors.New(apperrors.ErrInvalidInput, "view must be upcoming, active or past")
	}
	if q.Scope != "all" && q.Scope != "owned" && q.Scope != "participating" {
		return apperrors.ErrInvalidInput
	}
	if q.Sort != "" && q.Sort != "newest" && q.Sort != "oldest" {
		return apperrors.New(apperrors.ErrInvalidInput, "sort must be newest or oldest")
	}
	if q.Limit < 1 || q.Limit > 100 || (q.From != nil && q.To != nil && q.From.After(*q.To)) {
		return apperrors.ErrInvalidInput
	}
	if q.Status != "" && q.Status != Created && q.Status != Scheduled && q.Status != Active && q.Status != Finished && q.Status != Cancelled {
		return apperrors.ErrInvalidInput
	}
	cursor, err := DecodeTimelineCursor(q.Cursor)
	if err == nil && cursor != nil && (cursor.Sort == "oldest") != (q.Sort == "oldest") {
		return apperrors.New(apperrors.ErrInvalidInput, "cursor does not match the requested sort order")
	}
	return err
}

// DecodeTimelineCursor декодирует и проверяет границу продолжения для временной шкалы
// конференций.
//
// @Parameters:
//   - value: Значение для проверки, нормализации или преобразования.
//
// @returns:
//   - result1: Курсор для построения или загрузки временной шкалы.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func DecodeTimelineCursor(value string) (*TimelineCursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > 256 {
		return nil, apperrors.ErrInvalidInput
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(value)
	var c TimelineCursor
	if err != nil || json.Unmarshal(data, &c) != nil || c.At.IsZero() || (c.Sort != "" && c.Sort != "newest" && c.Sort != "oldest") {
		return nil, apperrors.ErrInvalidInput
	}
	id, err := uuid.Parse(c.ID)
	if err != nil || id == uuid.Nil {
		return nil, apperrors.ErrInvalidInput
	}
	c.ID = id.String()
	return &c, nil
}

// EncodeTimelineCursor records the conference's effective date and stable ID.
// The sort argument binds oldest-first cursors to their direction; empty or
// newest order retains the legacy cursor format for existing clients.
func EncodeTimelineCursor(c Conference, sort string) string {
	at := c.CreatedAt
	if c.ScheduledAt != nil {
		at = *c.ScheduledAt
	}
	if c.FinishedAt != nil {
		at = *c.FinishedAt
	}
	cursor := TimelineCursor{At: at, ID: c.ID}
	if sort == "oldest" {
		cursor.Sort = sort
	}
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}

// TimelinePage содержит отфильтрованные конференции и курсор для продолжения.
type TimelinePage struct {
	Items      []View  `json:"items"`
	NextCursor *string `json:"nextCursor"`
}

// RecordingSummary объединяет состояния записи без дублирования метаданных файлов в
// конференции.
type RecordingSummary struct {
	Total      int64 `json:"total"`
	Ready      int64 `json:"ready"`
	Processing int64 `json:"processing"`
	Failed     int64 `json:"failed"`
}

// HistoryOwner хранит данные о владельцах истории, которые используются для отслеживания
// жизненного цикла конференции и членства участников.
type HistoryOwner struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// HistoryView объединяет информацию о завершённых конференциях, видимое членство и сводные
// данные о записях.
type HistoryView struct {
	Conference            View              `json:"conference"`
	Owner                 HistoryOwner      `json:"owner"`
	DurationSec           *int64            `json:"durationSec"`
	ParticipantCount      int64             `json:"participantCount"`
	Participants          []ParticipantView `json:"participants"`
	ParticipantsTruncated bool              `json:"participantsTruncated"`
	Recordings            RecordingSummary  `json:"recordings"`
	ChatAvailable         bool              `json:"chatAvailable"`
	ChatReadOnly          bool              `json:"chatReadOnly"`
}

// CanParticipate применяет ту же политику членства к общедоступному представлению в режиме
// реального времени.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (p ParticipantView) CanParticipate() bool {
	return Participant{Status: p.Status, AdmissionState: p.AdmissionState}.CanParticipate()
}

// CanReadHistory включает в себя бывших участников, допущенных к конференции, в отличие от
// участников в режиме реального времени.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (p ParticipantView) CanReadHistory() bool {
	return Participant{Status: p.Status, AdmissionState: p.AdmissionState}.CanReadHistory()
}
