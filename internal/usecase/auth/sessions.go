// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
)

type SessionRepository interface {
	// Create сохраняет новую серверную сессию и дайджест секрета обновления, не сохраняя
	// исходный секрет обновления.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Постоянное или физическое состояние сессии, определяемое ее типом.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Create(context.Context, users.AuthSession) error
	// GetByHash загружает постоянную серверную сессию по дайджесту секрета обновления.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Ранее вычисленный дайджест, используемый для проверки подлинности или
	//   целостности.
	//
	// @returns:
	//   - result1: Результирующая сессия аутентификации.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	GetByHash(context.Context, string) (users.AuthSession, error)
	// GetByID загружает постоянный сеанс сервера по его идентификатору.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Результирующая сессия аутентификации.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	GetByID(context.Context, string) (users.AuthSession, error)
	// RevokeByHash отзывает постоянный сеанс, соответствующий дайджесту refresh-secret.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Ранее вычисленный дайджест, используемый для проверки подлинности или
	//   целостности.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	RevokeByHash(context.Context, string) error
	// RevokeByID отзывает только постоянный сеанс, принадлежащий указанной учетной записи и
	// имеющий указанный идентификатор.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	RevokeByID(context.Context, string, string) error
	// Bootstrap — при начальной загрузке атомарно создается или повторно используется постоянный
	// сеанс-кандидат с проверкой на отзыв устаревших токенов и предпочтительный сеанс браузера.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Сессия кандидата или запись ICE для проверки перед использованием.
	//   - arg3: Дайджест устаревшего токена доступа, используемый для проверки на
	//   возможность отзыва.
	//   - arg4: При повторном входе в систему через браузер предпочтительнее использовать
	//   существующий идентификатор сессии аккаунта.
	//
	// @returns:
	//   - result1: Результирующая сессия аутентификации.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Bootstrap(context.Context, users.AuthSession, string, string) (users.AuthSession, error)
	// RevokeLegacy регистрирует отзыв устаревшего токена доступа, чтобы не создавать новый
	// сеанс при начальной загрузке.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Ранее вычисленный дайджест, используемый для проверки подлинности или
	//   целостности.
	//   - arg3: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	RevokeLegacy(context.Context, string, string) error
	// AuthorizeLegacy отклоняет устаревшие учетные данные, которые были явно отозваны.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Ранее вычисленный дайджест, используемый для проверки подлинности или
	//   целостности.
	//   - arg3: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	AuthorizeLegacy(context.Context, string, string) error
}

type sessionTokens interface {
	// IssueSession выдает токен доступа JWT, привязанный к указанному постоянному сеансу на
	// сервере.
	//
	// @Parameters:
	//   - arg1: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg2: Постоянный или физический идентификатор сессии, определяемый операцией.
	//
	// @returns:
	//   - result1: Подписанный доступ JWT привязан к постоянному сеансу.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	IssueSession(string, string) (string, error)
	// VerifyAuthorization проверяет токен авторизации и определяет его область действия, а
	// также срок действия и принадлежность пользователя, сеанса и гостя.
	//
	// @Parameters:
	//   - arg1: Недоверенное сериализованное значение для декодирования или проверки.
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
	VerifyAuthorization(string) (string, string, string, time.Time, error)
}

// WithSessions обеспечивает надежную проверку входа в систему и отзыва токенов для API.
//
// @Parameters:
//   - repository: Зависимость персистентности, используемая операцией приложения.
//
// @returns:
//   - result1: Созданный или загруженный сервис.
func (s *Service) WithSessions(repository SessionRepository) *Service {
	s.sessions = repository
	return s
}

// signIn создает постоянный сеанс учетной записи, если таковой имеется, и выдает краткосрочный
// токен доступа.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Подтвержденная запись учетной записи, для которой обрабатывается публичная
//     идентификация или сеанс на сервере.
//
// @returns:
//   - result1: Итоговый ответ при входе в систему.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) signIn(ctx context.Context, user users.User) (users.LoginResponse, error) {
	if s.sessions == nil {
		token, err := s.tokens.Issue(user.ID)
		return loginResponse(user, token, ""), err
	}
	if user.GuestConferenceID != nil {
		return users.LoginResponse{}, apperrors.ErrForbidden
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return users.LoginResponse{}, err
	}
	raw := base64.RawURLEncoding.EncodeToString(secret[:])
	session := users.AuthSession{ID: uuid.NewString(), UserID: user.ID, TokenHash: sessionHash(raw), CreatedAt: time.Now().UTC()}
	response, err := s.sessionResponse(user, session, raw)
	if err != nil {
		return users.LoginResponse{}, err
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return users.LoginResponse{}, err
	}
	return response, nil
}

// loginResponse формирует общедоступный ответ на запрос входа в систему и внутреннее значение
// refresh-secret, используемое для установки cookie.
//
// @Parameters:
//   - user: Подтвержденная запись учетной записи, для которой обрабатывается публичная
//     идентификация или сеанс на сервере.
//   - token: Подписанные учетные данные или токен владения операцией для проверки.
//   - raw: Сериализованный токен доступа или обновленные учетные данные для проверки; никогда
//     не сохраняйте и не раскрывайте их.
//
// @returns:
//   - result1: Итоговый ответ при входе в систему.
func loginResponse(user users.User, token, raw string) users.LoginResponse {
	return users.LoginResponse{Status: "success", AccessToken: token, TokenType: "Bearer", ExpiresIn: 3600, User: user.View(), SessionToken: raw}
}

// sessionResponse выдает краткосрочный токен доступа, привязанный к предоставленному
// постоянному сеансу.
//
// @Parameters:
//   - user: Подтвержденная запись учетной записи, для которой обрабатывается публичная
//     идентификация или сеанс на сервере.
//   - session: Постоянное или физическое состояние сессии, определяемое ее типом.
//   - raw: Сериализованный токен доступа или обновленные учетные данные для проверки; никогда
//     не сохраняйте и не раскрывайте их.
//
// @returns:
//   - result1: Итоговый ответ при входе в систему.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) sessionResponse(user users.User, session users.AuthSession, raw string) (users.LoginResponse, error) {
	tokens, ok := s.tokens.(sessionTokens)
	if !ok {
		return users.LoginResponse{}, apperrors.ErrUnavailable
	}
	token, err := tokens.IssueSession(user.ID, session.ID)
	if err != nil {
		return users.LoginResponse{}, err
	}
	return loginResponse(user, token, raw), nil
}

// sessionHash хеширует исходный секретный ключ обновления, сохраняя только его дайджест
// SHA-256.
//
// @Parameters:
//   - raw: Исходный секретный ключ обновления, считанный из cookie-файла HttpOnly.
//
// @returns:
//   - result1: Шестнадцатеричный код SHA-256 исходного секретного ключа обновления.
func sessionHash(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

// validSessionSecret проверяет, что секретный ключ обновления имеет каноническую 32-байтовую
// кодировку, безопасную для URL.
//
// @Parameters:
//   - raw: Исходный секретный ключ обновления, считанный из cookie-файла HttpOnly.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func validSessionSecret(raw string) bool {
	if len(raw) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	return err == nil && len(decoded) == 32 && base64.RawURLEncoding.EncodeToString(decoded) == raw
}

// sessionForCookie определяет, что постоянный сеанс не был отменен, по проверенному
// cookie-файлу обновления.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - raw: Исходный секретный ключ обновления, считанный из cookie-файла HttpOnly.
//
// @returns:
//   - result1: Результирующая сессия аутентификации.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) sessionForCookie(ctx context.Context, raw string) (users.AuthSession, error) {
	if s.sessions == nil || !validSessionSecret(raw) {
		return users.AuthSession{}, apperrors.ErrUnauthorized
	}
	session, err := s.sessions.GetByHash(ctx, sessionHash(raw))
	if errors.Is(err, apperrors.ErrNotFound) || err == nil && session.RevokedAt != nil {
		return users.AuthSession{}, apperrors.ErrUnauthorized
	}
	return session, err
}

// account загружает зарегистрированную учетную запись и отклоняет отсутствующие учетные записи
// или гостевые учетные записи для операций с постоянными сеансами.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Полученный пользователь.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) account(ctx context.Context, id string) (users.User, error) {
	user, err := s.repository.GetByID(ctx, id)
	if errors.Is(err, apperrors.ErrNotFound) {
		return users.User{}, apperrors.ErrUnauthorized
	}
	if err != nil {
		return users.User{}, err
	}
	if user.GuestConferenceID != nil {
		return users.User{}, apperrors.ErrForbidden
	}
	return user, nil
}

// Refresh обновляет краткосрочный токен доступа, не устанавливая срок действия простоя/сеанса.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - raw: Сериализованный токен доступа или обновленные учетные данные для проверки; никогда
//     не сохраняйте и не раскрывайте их.
//
// @returns:
//   - result1: Итоговый ответ при входе в систему.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Refresh(ctx context.Context, raw string) (users.LoginResponse, error) {
	session, err := s.sessionForCookie(ctx, raw)
	if err != nil {
		return users.LoginResponse{}, err
	}
	user, err := s.account(ctx, session.UserID)
	if err != nil {
		return users.LoginResponse{}, err
	}
	return s.sessionResponse(user, session, raw)
}

// Bootstrap обновляет существующий токен учетной записи.  Повторно открытые вкладки используют
// тот же cookie-файл.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - raw: Сериализованный токен доступа или обновленные учетные данные для проверки; никогда
//     не сохраняйте и не раскрывайте их.
//   - access: Текущий подписанный токен доступа, используемый только для проверки сеанса.
//
// @returns:
//   - result1: Итоговый ответ при входе в систему.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Bootstrap(ctx context.Context, userID, raw, access string) (users.LoginResponse, error) {
	user, err := s.account(ctx, userID)
	if err != nil {
		return users.LoginResponse{}, err
	}
	if s.sessions == nil {
		return users.LoginResponse{}, apperrors.ErrUnavailable
	}
	tokens, ok := s.tokens.(sessionTokens)
	if !ok {
		return users.LoginResponse{}, apperrors.ErrUnavailable
	}
	identity, scope, sessionID, _, err := tokens.VerifyAuthorization(access)
	if err != nil {
		return users.LoginResponse{}, err
	}
	if identity != userID || scope != "" {
		return users.LoginResponse{}, apperrors.ErrForbidden
	}
	legacyHash := ""
	if sessionID == "" {
		legacyHash = sessionHash(access)
	}
	var preferredID string
	var existingCookieID, existingCookie string
	if raw != "" {
		session, err := s.sessionForCookie(ctx, raw)
		if err == nil && session.UserID == userID {
			existingCookieID, existingCookie = session.ID, raw
			if sessionID != "" && sessionID == session.ID {
				return s.sessionResponse(user, session, raw)
			}
			if sessionID == "" {
				preferredID = session.ID
			}
		}
		if err != nil && !errors.Is(err, apperrors.ErrUnauthorized) {
			return users.LoginResponse{}, err
		}
	}
	if sessionID != "" {
		// Краткосрочный токен доступа, доступный для чтения JS, ни в коем случае не должен
		// генерировать постоянный секретный ключ.  Вышеуказанные файлы cookie используются
		// повторно; при их потере необходимо выполнить повторный вход в систему.
		return users.LoginResponse{}, apperrors.ErrUnauthorized
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return users.LoginResponse{}, err
	}
	raw = base64.RawURLEncoding.EncodeToString(secret[:])
	candidate := users.AuthSession{ID: uuid.NewString(), UserID: userID, TokenHash: sessionHash(raw), CreatedAt: time.Now().UTC()}
	session, err := s.sessions.Bootstrap(ctx, candidate, legacyHash, preferredID)
	if errors.Is(err, apperrors.ErrNotFound) {
		err = apperrors.ErrUnauthorized
	}
	if err != nil {
		return users.LoginResponse{}, err
	}
	if session.ID == existingCookieID {
		raw = existingCookie
	}
	return s.sessionResponse(user, session, raw)
}

// Logout — при выходе из системы удаляются как текущий файл cookie браузера, так и предоставленный
// действительный токен SID.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - raw: Сериализованный токен доступа или обновленные учетные данные для проверки; никогда
//     не сохраняйте и не раскрывайте их.
//   - access: Текущий подписанный токен доступа, используемый только для проверки сеанса.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Logout(ctx context.Context, raw, access string) error {
	if s.sessions == nil {
		return nil
	}
	if validSessionSecret(raw) {
		if err := s.sessions.RevokeByHash(ctx, sessionHash(raw)); err != nil {
			return err
		}
	}
	if tokens, ok := s.tokens.(sessionTokens); ok && access != "" {
		userID, scope, sessionID, _, err := tokens.VerifyAuthorization(access)
		if err == nil && sessionID != "" {
			return s.sessions.RevokeByID(ctx, sessionID, userID)
		}
		if err == nil && scope == "" {
			return s.sessions.RevokeLegacy(ctx, sessionHash(access), userID)
		}
	}
	return nil
}

// AuthorizeSession также обслуживает долгоживущие соединения WebSocket/SSE после истечения
// срока действия JWT.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - sessionID: Постоянный или физический идентификатор сессии, определяемый операцией.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) AuthorizeSession(ctx context.Context, userID, sessionID string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if s.sessions == nil || sessionID == "" {
		return apperrors.ErrUnauthorized
	}
	session, err := s.sessions.GetByID(ctx, sessionID)
	if errors.Is(err, apperrors.ErrNotFound) || err == nil && (session.UserID != userID || session.RevokedAt != nil) {
		return apperrors.ErrUnauthorized
	}
	return err
}

// VerifyAuthorization проверяет токен авторизации и определяет его область действия, а также
// срок действия и принадлежность пользователя, сеанса и гостя.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - raw: Сериализованный токен доступа или обновленные учетные данные для проверки; никогда
//     не сохраняйте и не раскрывайте их.
//
// @returns:
//   - result1: Подтвержденный идентификатор учетной записи.
//   - result2: Область действия гостевой конференции или пустая строка для
//     зарегистрированной учетной записи.
//   - result3: Постоянный идентификатор сеанса на сервере или пустая строка для гостевого
//     или устаревшего токена.
//   - result4: Временная метка или граница допустимого срока действия, связанная с
//     результатом.
//   - result5: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) VerifyAuthorization(ctx context.Context, raw string) (string, string, string, time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tokens, ok := s.tokens.(sessionTokens)
	if !ok {
		return "", "", "", time.Time{}, apperrors.ErrUnauthorized
	}
	userID, scope, sessionID, expiry, err := tokens.VerifyAuthorization(raw)
	if err == nil && sessionID != "" {
		err = s.AuthorizeSession(ctx, userID, sessionID)
	}
	if err == nil && sessionID == "" && scope == "" && s.sessions != nil {
		err = s.sessions.AuthorizeLegacy(ctx, sessionHash(raw), userID)
	}
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	return userID, scope, sessionID, expiry, nil
}

// VerifySession проверяет JWT и возвращает утверждения о принадлежности к учётной записи,
// гостевой конференции и сроке действия.
//
// @Parameters:
//   - raw: Сериализованный токен доступа или обновленные учетные данные для проверки; никогда
//     не сохраняйте и не раскрывайте их.
//
// @returns:
//   - result1: Подтвержденный идентификатор учетной записи.
//   - result2: Область действия гостевой конференции или пустая строка для неограниченной
//     зарегистрированной учетной записи.
//   - result3: Временная метка или граница допустимого срока действия, связанная с
//     результатом.
//   - result4: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) VerifySession(raw string) (string, string, time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	userID, scope, _, expiry, err := s.VerifyAuthorization(ctx, raw)
	return userID, scope, expiry, err
}

// VerifyWithExpiry проверяет токен доступа и возвращает идентификатор учетной записи и срок
// действия токена.
//
// @Parameters:
//   - raw: Сериализованный токен доступа или обновленные учетные данные для проверки; никогда
//     не сохраняйте и не раскрывайте их.
//
// @returns:
//   - result1: Подтвержденный идентификатор учетной записи.
//   - result2: Временная метка или граница допустимого срока действия, связанная с
//     результатом.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) VerifyWithExpiry(raw string) (string, time.Time, error) {
	userID, _, expiry, err := s.VerifySession(raw)
	return userID, expiry, err
}

// Verify проверяет предоставленные учётные данные или подписанную привязку, прежде чем
// подтвердить личность вызывающего абонента.
//
// @Parameters:
//   - raw: Сериализованный токен доступа или обновленные учетные данные для проверки; никогда
//     не сохраняйте и не раскрывайте их.
//
// @returns:
//   - result1: Подтвержденный идентификатор учетной записи.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Verify(raw string) (string, error) {
	userID, _, err := s.VerifyWithExpiry(raw)
	return userID, err
}
