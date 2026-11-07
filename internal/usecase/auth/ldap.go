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
	"strings"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
)

// LDAPAuthenticator проверяет реквизиты в каталоге и возвращает привязанный локальный профиль.
type LDAPAuthenticator interface {
	Authenticate(context.Context, string, string) (users.User, error)
}

func (s *Service) WithLDAP(provider LDAPAuthenticator) *Service {
	s.ldap = provider
	return s
}

// LDAPLogin использует существующие SID и cookie после успешной корпоративной аутентификации.
func (s *Service) LDAPLogin(ctx context.Context, request users.LDAPLoginRequest) (users.LoginResponse, error) {
	request.Login = strings.TrimSpace(request.Login)
	if request.Login == "" || len(request.Login) > 254 || !utf8.ValidString(request.Login) || request.Password == "" || len(request.Password) > 4096 || strings.ContainsRune(request.Login, 0) || strings.ContainsRune(request.Password, 0) {
		return users.LoginResponse{}, apperrors.New(apperrors.ErrUnauthorized, "Неверный корпоративный логин или пароль")
	}
	if s.ldap == nil || s.sessions == nil {
		return users.LoginResponse{}, apperrors.ErrUnavailable
	}
	user, err := s.ldap.Authenticate(ctx, request.Login, request.Password)
	if err != nil {
		return users.LoginResponse{}, err
	}
	if !user.IsLDAP() || user.GuestConferenceID != nil {
		return users.LoginResponse{}, apperrors.ErrUnauthorized
	}
	if err := ctx.Err(); err != nil {
		return users.LoginResponse{}, err
	}
	return s.signIn(ctx, user)
}

func managedDirectoryAccount() error {
	return apperrors.New(apperrors.ErrForbidden, "Корпоративная учётная запись управляется в LDAP / Active Directory. Обратитесь к администратору.")
}
