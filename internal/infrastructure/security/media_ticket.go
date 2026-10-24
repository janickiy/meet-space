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
	"github.com/janickiy/meet-space/internal/domain/media"
)

// MediaTickets выдает и проверяет внутренние тикеты, привязанные к физическим
// медиаподключениям.
type MediaTickets struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// mediaClaims содержит подписанные утверждения об идентификаторе медиа-тикета и сроке его
// действия.
type mediaClaims struct {
	jwt.RegisteredClaims
	Purpose string        `json:"purpose"`
	Binding media.Binding `json:"binding"`
	Route   media.Route   `json:"route"`
}

// NewMediaTickets создает медиа-тикеты с зависимостями и ограничениями, необходимыми для
// аутентификации и криптографических привязок.
//
// @Parameters:
//   - secret: Материал для создания закрытого ключа или необработанный секретный ключ; его
//     нельзя регистрировать или раскрывать.
//   - ttl: Срок действия, назначенный для временного значения или записи о владельце.
//
// @returns:
//   - result1: Созданные или загруженные медиа-тикеты.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NewMediaTickets(secret string, ttl time.Duration) (*MediaTickets, error) {
	if len(secret) < 32 || ttl < 30*time.Second || ttl > 60*time.Second {
		return nil, fmt.Errorf("media ticket secret must contain >=32 bytes and TTL must be 30–60 seconds")
	}
	return &MediaTickets{secret: []byte(secret), ttl: ttl, now: time.Now}, nil
}

// validMediaBinding проверяет полную привязку идентификационных данных, используемых во
// внутренних медиа-тикетах.
//
// @Parameters:
//   - binding: Подтвержденная идентификация учетной записи, сеанса и медиа-подключения.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func validMediaBinding(binding media.Binding) bool {
	for _, id := range []string{binding.ConferenceID, binding.ParticipantID, binding.SessionID, binding.ConnectionID, binding.UserID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil || parsed.String() != id {
			return false
		}
	}
	return !binding.AuthorizationExpiresAt.IsZero()
}

// Issue создает подписанный токен, привязанный к предоставленным идентификационным данным и
// сроку действия.
//
// @Parameters:
//   - binding: Подтвержденная идентификация учетной записи, сеанса и медиа-подключения.
//   - route: Текущий владелец медиа-комнаты и версия ограждения.
//
// @returns:
//   - result1: Подписанные краткосрочные учетные данные для подтвержденной личности.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *MediaTickets) Issue(binding media.Binding, route media.Route) (string, error) {
	now := s.now().UTC()
	lease, err := uuid.Parse(route.LeaseID)
	if !validMediaBinding(binding) || !binding.AuthorizationExpiresAt.After(now) || route.WorkerID == "" || route.Endpoint == "" || err != nil || lease == uuid.Nil {
		return "", media.ErrUnauthorized
	}
	expires := now.Add(s.ttl)
	if binding.AuthorizationExpiresAt.Before(expires) {
		expires = binding.AuthorizationExpiresAt
	}
	claims := mediaClaims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "go-recorder-api", Audience: jwt.ClaimStrings{"go-recorder-media"}, Subject: binding.ConnectionID,
		ID: uuid.NewString(), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expires),
	}, Purpose: "media", Binding: binding, Route: route}
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
//   - result1: Полученная привязка.
//   - result2: Результирующий маршрут.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *MediaTickets) Verify(raw string) (media.Binding, media.Route, error) {
	if len(raw) == 0 || len(raw) > 8192 {
		return media.Binding{}, media.Route{}, media.ErrUnauthorized
	}
	claims := &mediaClaims{}
	token, err := jwt.ParseWithClaims(raw, claims,

		func(*jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(), jwt.WithIssuer("go-recorder-api"), jwt.WithAudience("go-recorder-media"), jwt.WithTimeFunc(s.now))
	if err != nil || !token.Valid || claims.Purpose != "media" || claims.IssuedAt == nil ||
		!validMediaBinding(claims.Binding) || claims.Subject != claims.Binding.ConnectionID ||
		!claims.Binding.AuthorizationExpiresAt.After(s.now()) ||
		claims.ExpiresAt.Time.After(claims.Binding.AuthorizationExpiresAt) ||
		claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time) > s.ttl+time.Second ||
		claims.Route.WorkerID == "" || claims.Route.Endpoint == "" {
		return media.Binding{}, media.Route{}, media.ErrUnauthorized
	}
	for _, id := range []string{claims.ID, claims.Route.LeaseID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil {
			return media.Binding{}, media.Route{}, media.ErrUnauthorized
		}
	}
	return claims.Binding, claims.Route, nil
}
