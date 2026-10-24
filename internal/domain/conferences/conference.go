// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package conferences

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

type Status string
type Role string
type ParticipantStatus string
type AdmissionState string

const (
	Created           Status            = "created"
	Scheduled         Status            = "scheduled"
	Active            Status            = "active"
	Finished          Status            = "finished"
	Cancelled         Status            = "cancelled"
	Owner             Role              = "owner"
	CoHost            Role              = "co_host"
	ParticipantRole   Role              = "participant"
	Guest             Role              = "guest"
	Joined            ParticipantStatus = "joined"
	Left              ParticipantStatus = "left"
	Waiting           ParticipantStatus = "waiting"
	Rejected          ParticipantStatus = "rejected"
	Kicked            ParticipantStatus = "kicked"
	AdmissionWaiting  AdmissionState    = "waiting"
	AdmissionAdmitted AdmissionState    = "admitted"
	AdmissionRejected AdmissionState    = "rejected"
	AdmissionKicked   AdmissionState    = "kicked"
)

var ErrInviteCollision = errors.New("invite code collision")

// CanTransition проверяет, разрешён ли запрошенный переход между состояниями жизненного цикла
// из текущего состояния.
//
// @Parameters:
//   - from: Исходное состояние предлагаемого перехода между состояниями жизненного цикла.
//   - to: Конечное состояние или получатель операции.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func CanTransition(from, to Status) bool {
	return ((from == Created || from == Scheduled) && (to == Active || to == Cancelled)) || (from == Active && to == Finished)
}

// Conference сохраняет данные о владельце, состоянии жизненного цикла и расписании.
type Conference struct {
	ID                 string `gorm:"type:uuid;primaryKey"`
	OwnerID            string `gorm:"column:owner_id;type:uuid"`
	Kind               string `gorm:"default:meeting"`
	Title              string
	InviteCode         string `gorm:"column:invite_code"`
	Status             Status
	CreatedAt          time.Time
	UpdatedAt          time.Time
	StartedAt          *time.Time
	FinishedAt         *time.Time
	WaitingRoomEnabled bool
	ScheduledAt        *time.Time
	PlannedDurationMin *int
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (Conference) TableName() string { return "conferences" }

// Participant сохраняет данные о членстве в учётных записях, ограничениях доступа и
// медиаконтенте независимо от физических подключений.
type Participant struct {
	ID                 string  `gorm:"type:uuid;primaryKey"`
	ConferenceID       string  `gorm:"column:conference_id;type:uuid"`
	UserID             *string `gorm:"column:user_id;type:uuid"`
	DisplayName        string
	Role               Role
	Status             ParticipantStatus
	JoinedAt           *time.Time
	LeftAt             *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	MicrophoneEnabled  bool
	CameraEnabled      bool
	ScreenSharing      bool
	MicrophoneBlocked  bool
	CameraBlocked      bool
	ScreenBlocked      bool
	MediaPolicyVersion int64          `gorm:"default:1"`
	AdmissionState     AdmissionState `gorm:"default:admitted"`
	AdmissionDecidedAt *time.Time
	AdmissionVersion   int64 `gorm:"default:1"`
	ChatLeft           bool  `json:"-" gorm:"column:chat_left;->;-:migration"`
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (Participant) TableName() string { return "conference_participants" }

// View — это безопасная публичная проекция API конференции.
type View struct {
	ID                 string     `json:"id"`
	OwnerID            string     `json:"ownerId"`
	Title              string     `json:"title"`
	InviteCode         string     `json:"inviteCode"`
	InviteURL          string     `json:"inviteUrl"`
	Status             Status     `json:"status"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	StartedAt          *time.Time `json:"startedAt"`
	FinishedAt         *time.Time `json:"finishedAt"`
	WaitingRoomEnabled bool       `json:"waitingRoomEnabled"`
	ScheduledAt        *time.Time `json:"scheduledAt"`
	PlannedDurationMin *int       `json:"plannedDurationMin"`
}

// View создает публичное представление API, не раскрывая приватные поля персистентности.
//
// @returns:
//   - result1: Результирующее представление.
func (c Conference) View() View {
	return View{ID: c.ID, OwnerID: c.OwnerID, Title: c.Title, InviteCode: c.InviteCode,
		InviteURL: "/api/v1/conference-invites/" + c.InviteCode, Status: c.Status,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, StartedAt: c.StartedAt, FinishedAt: c.FinishedAt,
		WaitingRoomEnabled: c.WaitingRoomEnabled, ScheduledAt: c.ScheduledAt, PlannedDurationMin: c.PlannedDurationMin}
}

// InviteView предоставляет ограниченную информацию о приглашениях, не предоставляя доступа к
// защищенным данным о комнатах.
type InviteView struct {
	ID                 string     `json:"id"`
	Title              string     `json:"title"`
	Status             Status     `json:"status"`
	WaitingRoomEnabled bool       `json:"waitingRoomEnabled,omitempty"`
	ScheduledAt        *time.Time `json:"scheduledAt,omitempty"`
}

// ParticipantView содержит информацию о членстве и участниках, доступную внешнему клиенту.
type ParticipantView struct {
	ID                 string            `json:"id"`
	ConferenceID       string            `json:"conferenceId"`
	UserID             *string           `json:"userId"`
	DisplayName        string            `json:"displayName"`
	Role               Role              `json:"role"`
	Status             ParticipantStatus `json:"status"`
	JoinedAt           *time.Time        `json:"joinedAt"`
	LeftAt             *time.Time        `json:"leftAt"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
	MicrophoneEnabled  bool              `json:"microphoneEnabled"`
	CameraEnabled      bool              `json:"cameraEnabled"`
	ScreenSharing      bool              `json:"screenSharing"`
	MicrophoneBlocked  bool              `json:"microphoneBlocked"`
	CameraBlocked      bool              `json:"cameraBlocked"`
	ScreenBlocked      bool              `json:"screenBlocked"`
	MediaPolicyVersion int64             `json:"mediaPolicyVersion"`
	AdmissionState     AdmissionState    `json:"admissionState"`
	AdmissionDecidedAt *time.Time        `json:"admissionDecidedAt"`
	AdmissionVersion   int64             `json:"admissionVersion"`
	ChatLeft           bool              `json:"-"`
}

// View создает публичное представление API, не раскрывая приватные поля персистентности.
//
// @returns:
//   - result1: Полученное представление участника.
func (p Participant) View() ParticipantView {
	return ParticipantView(p)
}

// CreateRequest содержит настройки создания конференции, включая расписание и конфигурацию
// комнаты ожидания.
type CreateRequest struct {
	Title              string     `json:"title"`
	WaitingRoomEnabled bool       `json:"waitingRoomEnabled"`
	ScheduledAt        *time.Time `json:"scheduledAt"`
	PlannedDurationMin *int       `json:"plannedDurationMin"`
}

// JoinRequest содержит информацию об участниках, указанную при присоединении к конференции.
type JoinRequest struct {
	InviteCode string `json:"inviteCode"`
}

// NormalizeTitle обрезает и проверяет название конференции перед сохранением.
//
// @Parameters:
//   - value: Значение для проверки, нормализации или преобразования.
//
// @returns:
//   - result1: Проверенное нормализованное название конференции.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NormalizeTitle(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !utf8.ValidString(value) || value == "" || utf8.RuneCountInString(value) > 200 {
		return "", apperrors.New(apperrors.ErrInvalidInput, "title must contain 1 to 200 characters")
	}
	return value, nil
}
