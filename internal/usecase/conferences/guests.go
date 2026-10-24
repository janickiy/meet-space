// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package conferences

import (
	"context"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/users"
)

type GuestRepository interface {
	// JoinGuest создает гостевое членство в рамках конференции на основе подтвержденного
	// приглашения.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Ограниченный публичный статус или код ошибки.
	//   - arg3: Имя параметра, файла cookie, ресурса или провайдера, к которому
	//   осуществляется обращение.
	//   - arg4: Идентификатор существующей гостевой учетной записи, с помощью которого
	//   можно возобновить действие приглашения.
	//
	// @returns:
	//   - result1: Полученный пользователь.
	//   - result2: Полученный участник.
	//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	JoinGuest(context.Context, string, string, string) (users.User, domain.Participant, error)
}

type GuestTokens interface {
	// IssueGuest создает краткосрочное гостевое членство JWT, доступное только в рамках одной
	// конференции.
	//
	// @Parameters:
	//   - arg1: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//
	// @returns:
	//   - result1: Подписанный гостевой доступ JWT ограничен приглашёнными участниками
	//   конференции.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	IssueGuest(string, string) (string, error)
}

type GuestSession struct {
	users.LoginResponse
	Item domain.ParticipantView `json:"item"`
}

type GuestService struct {
	Repository GuestRepository
	Tokens     GuestTokens
	Observer   interface {
		// ConferenceChanged уведомляет подключенные сеансы или наблюдателей об изменении
		// состояния конференции.
		//
		// @Parameters:
		//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
		//   операции.
		//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
		ConferenceChanged(context.Context, string)
	}
}

// Join создает или возобновляет гостевое членство только после явного приглашения к участию.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - code: Ограниченный публичный статус или код ошибки.
//   - name: Имя параметра, файла cookie, ресурса или провайдера, к которому осуществляется
//     обращение.
//   - resumeUserID: Идентификатор существующей гостевой учетной записи, с помощью которого
//     можно возобновить действие приглашения.
//
// @returns:
//   - result1: Результирующая гостевая сессия.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *GuestService) Join(ctx context.Context, code, name, resumeUserID string) (GuestSession, error) {
	if !validInvite(code) {
		return GuestSession{}, apperrors.ErrNotFound
	}
	name, err := users.NormalizeDisplayName(name)
	if err != nil {
		return GuestSession{}, err
	}
	user, participant, err := s.Repository.JoinGuest(ctx, code, name, resumeUserID)
	if err != nil {
		return GuestSession{}, err
	}
	token, err := s.Tokens.IssueGuest(user.ID, participant.ConferenceID)
	if err != nil {
		return GuestSession{}, err
	}
	if s.Observer != nil {
		s.Observer.ConferenceChanged(ctx, participant.ConferenceID)
	}
	return GuestSession{LoginResponse: users.LoginResponse{Status: "success", AccessToken: token, TokenType: "Bearer", ExpiresIn: int(users.GuestSessionTTL / time.Second), User: user.View()}, Item: participant.View()}, nil
}
