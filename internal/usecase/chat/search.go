// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package chat

import (
	"context"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/chat"
)

type SearchRepository interface {
	// SearchMessagePage возвращает страницу с ограниченным количеством результатов поиска по
	// истории сообщений в рамках авторизованной области чата.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции или диалога, выбранный в зависимости от режима
	//   репозитория.
	//   - arg4: Текст для поиска или валидированные фильтры запросов.
	//   - arg5: Непрозрачная граница продолжения с предыдущей страницы.
	//   - arg6: Максимальное количество результатов или операций в этом пакете.
	//
	// @returns:
	//   - result1: Полученная страница.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	SearchMessagePage(context.Context, string, string, string, string, int) (domain.Page, error)
	// ChatMessageContext возвращает видимую историю сообщений вокруг запрошенного сообщения в
	// пределах авторизованной области.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции или диалога, выбранный в зависимости от режима
	//   репозитория.
	//   - arg4: Идентификатор сообщения в рамках авторизованного диалога.
	//
	// @returns:
	//   - result1: Полученная страница.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ChatMessageContext(context.Context, string, string, string) (domain.Page, error)
}

// SearchService использует обычную политику видимости в чате, не помечая результаты как
// прочитанные.
type SearchService struct{ Repo SearchRepository }

// Search — поиск выполняет поиск в постоянных сообщениях чата и личных вложениях с использованием
// ограниченных входных данных и фильтров видимости, заданных вызывающей стороной.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conversation: Идентификатор чата, ограничивающий область действия операции.
//   - query: Текст для поиска или валидированные фильтры запросов.
//   - cursor: Непрозрачная граница продолжения с предыдущей страницы.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//
// @returns:
//   - result1: Полученная страница.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *SearchService) Search(ctx context.Context, user, conversation, query, cursor string, limit int) (domain.Page, error) {
	query, err := domain.NormalizeSearchQuery(query)
	if err != nil {
		return domain.Page{}, err
	}
	if limit < 1 || limit > 50 {
		return domain.Page{}, apperrors.ErrInvalidInput
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.Repo.SearchMessagePage(bounded, user, conversation, query, cursor, limit)
}

// Context возвращает видимые сообщения, окружающие выбранное сообщение, чтобы пользователь мог
// перейти к результату поиска.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conversation: Идентификатор чата, ограничивающий область действия операции.
//   - messageID: Идентификатор сообщения в рамках авторизованного диалога.
//
// @returns:
//   - result1: Полученная страница.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *SearchService) Context(ctx context.Context, user, conversation, messageID string) (domain.Page, error) {
	id, err := domain.UUID(messageID)
	if err != nil {
		return domain.Page{}, err
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.Repo.ChatMessageContext(bounded, user, conversation, id)
}
