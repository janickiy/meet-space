// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package users

import (
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// User сохраняет данные для входа в учетную запись и хэш пароля; публичные представления
// формируются отдельно.
type User struct {
	ID                string    `gorm:"type:uuid;primaryKey" json:"-"`
	Email             string    `json:"-"`
	PasswordHash      string    `gorm:"column:password_hash" json:"-"`
	DisplayName       *string   `gorm:"column:display_name" json:"-"`
	IsAdmin           bool      `gorm:"column:is_admin" json:"-"`
	GuestConferenceID *string   `gorm:"column:guest_conference_id" json:"-"`
	AvatarVersion     *string   `gorm:"column:avatar_version" json:"-"`
	CreatedAt         time.Time `json:"-"`
	UpdatedAt         time.Time `json:"-"`
}

const (
	MinPasswordCharacters = 8
	MaxPasswordCharacters = 128
)

// GuestSessionTTL keeps the guest JWT expiry and join response lifetime aligned.
const GuestSessionTTL = 3 * time.Hour

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (User) TableName() string { return "users" }

// View представляет собой безопасную публичную проекцию API учетной записи.
type View struct {
	ID                string    `json:"id"`
	Email             string    `json:"email"`
	DisplayName       *string   `json:"displayName"`
	IsAdmin           bool      `json:"isAdmin"`
	GuestConferenceID *string   `json:"guestConferenceId,omitempty"`
	AvatarVersion     *string   `json:"avatarVersion,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// View создает публичное представление API, не раскрывая приватные поля персистентности.
//
// @returns:
//   - result1: Результирующее представление.
func (u User) View() View {
	email := u.Email
	if u.GuestConferenceID != nil {
		email = ""
	}
	return View{ID: u.ID, Email: email, DisplayName: u.DisplayName, IsAdmin: u.IsAdmin, GuestConferenceID: u.GuestConferenceID, AvatarVersion: u.AvatarVersion, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt}
}

// ParticipantName выбирает публичное имя участника, которое используется в представлениях о
// членстве в конференции.
//
// @returns:
//   - result1: Безопасное публичное отображаемое имя участника.
func (u User) ParticipantName() string {
	if u.DisplayName != nil && *u.DisplayName != "" {
		return *u.DisplayName
	}
	return "Participant"
}

// RegisterRequest содержит адрес электронной почты, пароль и необязательное публичное
// регистрационное имя.
type RegisterRequest struct {
	Email       string  `json:"email"`
	Password    string  `json:"password"`
	DisplayName *string `json:"displayName"`
}

// UpdateProfileRequest позволяет изменять только отображаемое имя текущего аккаунта.
type UpdateProfileRequest struct {
	DisplayName string `json:"displayName"`
}

// NormalizeDisplayName обрезает и проверяет отображаемое имя перед сохранением.
//
// @Parameters:
//   - value: Значение для проверки, нормализации или преобразования.
//
// @returns:
//   - result1: Проверенное публичное отображаемое имя.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NormalizeDisplayName(value string) (string, error) {
	name := strings.TrimSpace(value)
	length := utf8.RuneCountInString(name)
	if !utf8.ValidString(name) || length < 1 || length > 100 {
		return "", apperrors.New(apperrors.ErrInvalidInput, "displayName must contain 1 to 100 characters")
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return "", apperrors.New(apperrors.ErrInvalidInput, "displayName must not contain control characters")
		}
	}
	return name, nil
}

// LoginRequest содержит введенные учетные данные для входа.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse содержит результат входа, токен доступа и безопасную публичную информацию об
// аккаунте.
type LoginResponse struct {
	Status      string `json:"status"`
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	ExpiresIn   int    `json:"expiresIn"`
	User        View   `json:"user"`
	// SessionToken передается только в виде файла cookie HttpOnly, а не JSON.
	SessionToken string `json:"-"`
}

// NormalizeEmail нормализует адрес электронной почты для единообразного поиска аккаунта.
//
// @Parameters:
//   - value: Значение для проверки, нормализации или преобразования.
//
// @returns:
//   - result1: Нормализованный адрес электронной почты, используемый для поиска аккаунта.
func NormalizeEmail(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

// ValidateEmail проверяет, соответствует ли адрес электронной почты правилам регистрации
// аккаунта.
//
// @Parameters:
//   - value: Значение для проверки, нормализации или преобразования.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func ValidateEmail(value string) error {
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value || len(value) > 254 || !strings.Contains(value, "@") {
		return apperrors.New(apperrors.ErrInvalidInput, "email must be a valid address, up to 254 bytes")
	}
	return nil
}

// NormalizeRegister нормализует поля регистрации и отклоняет неверные учетные данные перед
// созданием аккаунта.
//
// @Parameters:
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Результирующий запрос на регистрацию.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NormalizeRegister(request RegisterRequest) (RegisterRequest, error) {
	request.Email = NormalizeEmail(request.Email)
	if err := ValidateEmail(request.Email); err != nil {
		return request, err
	}
	length := utf8.RuneCountInString(request.Password)
	if !utf8.ValidString(request.Password) || length < MinPasswordCharacters || length > MaxPasswordCharacters {
		return request, apperrors.New(apperrors.ErrInvalidInput, "password must contain 8 to 128 characters")
	}
	if request.DisplayName != nil {
		name := strings.TrimSpace(*request.DisplayName)
		if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 {
			return request, apperrors.New(apperrors.ErrInvalidInput, "displayName must be at most 100 characters")
		}
		request.DisplayName = nil
		if name != "" {
			request.DisplayName = &name
		}
	}
	return request, nil
}
