// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package httpmiddleware

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

const authenticatedUserIDKey = "authenticated_user_id"

type SessionVerifier interface {
	// VerifySession проверяет JWT и возвращает утверждения о принадлежности к учётной записи,
	// гостевой конференции и сроке действия.
	//
	// @Parameters:
	//   - arg1: Недоверенное сериализованное значение для декодирования или проверки.
	//
	// @returns:
	//   - result1: Подтвержденный идентификатор учетной записи.
	//   - result2: Область действия гостевой конференции или пустая строка для
	//   неограниченной зарегистрированной учетной записи.
	//   - result3: Временная метка или граница допустимого срока действия, связанная с
	//   результатом.
	//   - result4: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	VerifySession(string) (string, string, time.Time, error)
}

// AuthorizationVerifier проверяет постоянный сеанс с использованием срока действия запроса.
type AuthorizationVerifier interface {
	// VerifyAuthorization проверяет токен авторизации и определяет его область действия, а
	// также срок действия и принадлежность пользователя, сеанса и гостя.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Недоверенное сериализованное значение для декодирования или проверки.
	//
	// @returns:
	//   - result1: Подтвержденный идентификатор учетной записи.
	//   - result2: Область действия гостевой конференции или пустая строка для
	//   зарегистрированной учетной записи.
	//   - result3: Постоянный идентификатор сеанса на сервере или пустая строка для
	//   гостевого или устаревшего токена.
	//   - result4: Временная метка или граница допустимого срока действия, связанная с
	//   результатом.
	//   - result5: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	VerifyAuthorization(context.Context, string) (string, string, string, time.Time, error)
}

// GuestRequestAllowed хранит учетные данные гостя в рамках одной конференции, включая WS.
// После проверки области действия по-прежнему выполняются стандартные проверки на допуск к
// конференции и определение роли.
//
// @Parameters:
//   - method: Метод HTTP запрошенного маршрута.
//   - path: Проверенный путь к локальной файловой системе или объекту.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func GuestRequestAllowed(method, path, conferenceID string) bool {
	if conferenceID == "" {
		return true
	}
	if method == "GET" && (path == "/api/v1/auth/me" || path == "/api/v1/capabilities" || path == "/api/v1/webrtc/config") {
		return true
	}
	if method == "POST" && path == "/api/v1/auth/logout" {
		return true
	}
	base := "/api/v1/conferences/" + conferenceID
	return path == base || strings.HasPrefix(path, base+"/")
}

// TokenVerifier определяет заменяемый контракт зависимости для HTTP.
type TokenVerifier interface {
	// Verify проверяет предоставленные учётные данные или подписанную привязку, прежде чем
	// подтвердить личность вызывающего абонента.
	//
	// @Parameters:
	//   - arg1: Недоверенное сериализованное значение для декодирования или проверки.
	//
	// @returns:
	//   - result1: Подтвержденный идентификатор учетной записи.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Verify(string) (string, error)
}

// Authenticate проверяет токен Bearer JWT и сохраняет его подтвержденные идентификационные
// данные в контексте Gin.
//
// @Parameters:
//   - tokens: Эмитент или верификатор токена, используемый для проверки подлинности.
//
// @returns:
//   - result1: Обработчик HTTP для регистрации этой операции.
func Authenticate(tokens TokenVerifier) gin.HandlerFunc {

	return func(c *gin.Context) {
		header := strings.Fields(c.GetHeader("Authorization"))
		if len(header) != 2 || !strings.EqualFold(header[0], "Bearer") || tokens == nil {
			httpresponse.Fail(c, apperrors.ErrUnauthorized)
			return
		}
		var id, conferenceID string
		var err error
		if durable, ok := tokens.(AuthorizationVerifier); ok {
			id, conferenceID, _, _, err = durable.VerifyAuthorization(c.Request.Context(), header[1])
		} else if scoped, ok := tokens.(SessionVerifier); ok {
			id, conferenceID, _, err = scoped.VerifySession(header[1])
		} else {
			id, err = tokens.Verify(header[1])
		}
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		if id == "" {
			httpresponse.Fail(c, apperrors.ErrUnauthorized)
			return
		}
		if !GuestRequestAllowed(c.Request.Method, c.Request.URL.Path, conferenceID) {
			httpresponse.Fail(c, apperrors.ErrForbidden)
			return
		}
		c.Set(authenticatedUserIDKey, id)
		c.Next()
	}
}

// UserID считывает идентификатор аутентифицированной учетной записи, сохраненный промежуточным
// программным обеспечением HTTP.
//
// @Parameters:
//   - c: Контекст запроса и ответа Gin, включая аутентифицированные идентификационные данные.
//
// @returns:
//   - result1: Идентификатор аутентифицированной учетной записи из контекста запроса.
func UserID(c *gin.Context) string { return c.GetString(authenticatedUserIDKey) }
