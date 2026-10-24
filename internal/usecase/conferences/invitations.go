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
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/users"
)

type InvitationRepository interface {
	// SearchInvitationUsers возвращает ограниченный список зарегистрированных кандидатов на
	// получение приглашения, доступных действующему организатору.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор учетной записи, выполняющей операцию.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Текст для поиска или валидированные фильтры запросов.
	//
	// @returns:
	//   - result1: Собранные значения пользователей, которым было отправлено приглашение.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	SearchInvitationUsers(context.Context, string, string, string) ([]domain.InvitationUser, error)
	// Invite создаёт или обновляет приглашения на конференцию для подтверждённых получателей.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор учетной записи, выполняющей операцию.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Валидированный запрос приложения или входящие параметры операции.
	//
	// @returns:
	//   - result1: Собранные значения результатов приглашения.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Invite(context.Context, string, string, domain.InvitationRequest) ([]domain.InvitationResult, error)
}

type InvitationService struct{ Repository InvitationRepository }

// Search — поиск по жизненному циклу конференции и членству участников с использованием ограниченных
// входных данных и фильтров видимости, заданных вызывающей стороной.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - actorID: Идентификатор учетной записи, выполняющей операцию.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - query: Текст для поиска или валидированные фильтры запросов.
//
// @returns:
//   - result1: Собранные значения пользователей, которым было отправлено приглашение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *InvitationService) Search(ctx context.Context, actorID, conferenceID, query string) ([]domain.InvitationUser, error) {
	query = strings.TrimSpace(query)
	queryLength := utf8.RuneCountInString(query)
	if !utf8.ValidString(query) || queryLength < 2 || queryLength > 100 {
		return nil, apperrors.New(apperrors.ErrInvalidInput, "query must contain 2 to 100 characters")
	}
	return s.Repository.SearchInvitationUsers(ctx, actorID, conferenceID, query)
}

// Invite создаёт или обновляет приглашения на конференцию для подтверждённых получателей.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - actorID: Идентификатор учетной записи, выполняющей операцию.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Собранные значения результатов приглашения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *InvitationService) Invite(ctx context.Context, actorID, conferenceID string, request domain.InvitationRequest) ([]domain.InvitationResult, error) {
	// ограничение применяется к отправленным получателям до дедупликации.
	recipientCount := len(request.Emails) + len(request.UserIDs)
	if recipientCount == 0 || recipientCount > 20 {
		return nil, apperrors.New(apperrors.ErrInvalidInput, "select 1 to 20 recipients")
	}
	normalized := domain.InvitationRequest{}
	seenEmails, seenUserIDs := map[string]bool{}, map[string]bool{}
	for _, email := range request.Emails {
		email = users.NormalizeEmail(email)
		if err := domain.ValidateInvitationEmail(email); err != nil {
			return nil, err
		}
		if !seenEmails[email] {
			normalized.Emails = append(normalized.Emails, email)
			seenEmails[email] = true
		}
	}
	for _, userID := range request.UserIDs {
		parsed, err := uuid.Parse(userID)
		if err != nil || parsed == uuid.Nil {
			return nil, apperrors.ErrInvalidInput
		}
		userID = parsed.String()
		if !seenUserIDs[userID] {
			normalized.UserIDs = append(normalized.UserIDs, userID)
			seenUserIDs[userID] = true
		}
	}
	return s.Repository.Invite(ctx, actorID, conferenceID, normalized)
}
