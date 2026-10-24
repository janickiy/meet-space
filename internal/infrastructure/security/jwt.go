// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package security

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
)

const AccessTokenTTL = time.Hour
const tokenIssuer = "go-recorder"
const tokenAudience = "go-recorder-api"

type sessionClaims struct {
	jwt.RegisteredClaims
	GuestConferenceID string `json:"guestConferenceId,omitempty"`
	SessionID         string `json:"sid,omitempty"`
}

// TokenService создает и проверяет учетную запись JWTs с заданным ограниченным сроком действия.
type TokenService struct {
	secret []byte
	now    func() time.Time
}

// NewTokenService создает службу токенов с зависимостями и ограничениями, необходимыми для
// аутентификации и криптографических привязок.
//
// @Parameters:
//   - secret: Материал для создания закрытого ключа или необработанный секретный ключ; его
//     нельзя регистрировать или раскрывать.
//
// @returns:
//   - result1: Созданная или загруженная служба токенов.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NewTokenService(secret string) (*TokenService, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must contain at least 32 bytes")
	}
	return &TokenService{secret: []byte(secret), now: time.Now}, nil
}

// Issue создает подписанный токен, привязанный к предоставленным идентификационным данным и
// сроку действия.
//
// @Parameters:
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: Подписанные краткосрочные учетные данные для подтвержденной личности.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *TokenService) Issue(userID string) (string, error) {
	return s.issue(userID, "", "")
}

// IssueSession привязывает краткосрочный токен доступа к отзываемой сессии учетной записи.
//
// @Parameters:
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - sessionID: Постоянный или физический идентификатор сессии, определяемый операцией.
//
// @returns:
//   - result1: Подписанный доступ JWT привязан к постоянному сеансу.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *TokenService) IssueSession(userID, sessionID string) (string, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil || id == uuid.Nil {
		return "", fmt.Errorf("invalid account session")
	}
	return s.issue(userID, "", id.String())
}

// IssueGuest предоставляет доступ только к приглашённым участникам конференции, но не к учётной
// записи.
//
// @Parameters:
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Подписанный гостевой доступ JWT ограничен приглашёнными участниками
//     конференции.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *TokenService) IssueGuest(userID, conferenceID string) (string, error) {
	id, err := uuid.Parse(conferenceID)
	if err != nil || id == uuid.Nil {
		return "", fmt.Errorf("invalid guest conference")
	}
	return s.issue(userID, id.String(), "")
}

// issue создаёт подписанный токен, привязанный к предоставленным идентификационным данным и
// сроку действия.
//
// @Parameters:
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - sessionID: Постоянный или физический идентификатор сессии, определяемый операцией.
//
// @returns:
//   - result1: Подписанный доступ JWT содержит привязку к предоставленным идентификационным
//     данным.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *TokenService) issue(userID, conferenceID, sessionID string) (string, error) {
	id, err := uuid.Parse(userID)
	if err != nil || id == uuid.Nil {
		return "", fmt.Errorf("invalid token subject")
	}
	ttl := AccessTokenTTL
	if conferenceID != "" {
		ttl = users.GuestSessionTTL
	}
	now := s.now().UTC()
	claims := sessionClaims{GuestConferenceID: conferenceID, SessionID: sessionID, RegisteredClaims: jwt.RegisteredClaims{Subject: id.String(), Issuer: tokenIssuer,
		Audience: jwt.ClaimStrings{tokenAudience}, IssuedAt: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl))}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
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
func (s *TokenService) Verify(raw string) (string, error) {
	id, _, err := s.VerifyWithExpiry(raw)
	return id, err
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
func (s *TokenService) VerifyWithExpiry(raw string) (string, time.Time, error) {
	id, _, expiry, err := s.VerifySession(raw)
	return id, expiry, err
}

// VerifySession проверяет подпись и возвращает необязательную область действия встречи.
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
func (s *TokenService) VerifySession(raw string) (string, string, time.Time, error) {
	id, conferenceID, _, expiry, err := s.VerifyAuthorization(raw)
	return id, conferenceID, expiry, err
}

// VerifyAuthorization проверяет утверждения JWT; приложение также проверяет возможность отзыва
// сеанса.
//
// @Parameters:
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
func (s *TokenService) VerifyAuthorization(raw string) (string, string, string, time.Time, error) {
	if len(raw) == 0 || len(raw) > 4096 {
		return "", "", "", time.Time{}, apperrors.ErrUnauthorized
	}
	claims := &sessionClaims{}
	token, err := jwt.ParseWithClaims(raw, claims,

		func(_ *jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(), jwt.WithIssuer(tokenIssuer), jwt.WithAudience(tokenAudience), jwt.WithTimeFunc(s.now))
	if err != nil || !token.Valid || claims.IssuedAt == nil {
		return "", "", "", time.Time{}, apperrors.ErrUnauthorized
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil || id == uuid.Nil {
		return "", "", "", time.Time{}, apperrors.ErrUnauthorized
	}
	if claims.GuestConferenceID != "" {
		conferenceID, err := uuid.Parse(claims.GuestConferenceID)
		if err != nil || conferenceID == uuid.Nil || conferenceID.String() != claims.GuestConferenceID {
			return "", "", "", time.Time{}, apperrors.ErrUnauthorized
		}
	}
	if claims.SessionID != "" {
		sessionID, err := uuid.Parse(claims.SessionID)
		if err != nil || sessionID == uuid.Nil || sessionID.String() != claims.SessionID || claims.GuestConferenceID != "" {
			return "", "", "", time.Time{}, apperrors.ErrUnauthorized
		}
	}
	return id.String(), claims.GuestConferenceID, claims.SessionID, claims.ExpiresAt.Time, nil
}
