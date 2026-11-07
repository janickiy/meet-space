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
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	platformsettings "github.com/janickiy/meet-space/internal/domain/settings"
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

// WithAuthenticationPolicy подключает единый серверный выбор локального или LDAP входа.
func (s *Service) WithAuthenticationPolicy(reader platformsettings.Reader) *Service {
	s.authPolicy = reader
	return s
}

func (s *Service) ldapOnly(ctx context.Context) (bool, error) {
	if s.authPolicy != nil {
		values, err := s.authPolicy.Snapshot(ctx)
		return values.LDAPEnabled, err
	}
	if provider, ok := s.ldap.(interface {
		Enabled(context.Context) (bool, error)
	}); ok {
		return provider.Enabled(ctx)
	}
	return false, nil
}

func (s *Service) requireLocalAuthentication(ctx context.Context) error {
	enabled, err := s.ldapOnly(ctx)
	if err != nil {
		return err
	}
	if enabled {
		return apperrors.New(apperrors.ErrForbidden, "Локальные аккаунты отключены. Используйте корпоративный email и пароль.")
	}
	return nil
}

func (s *Service) requireAccountMode(ctx context.Context, user users.User) error {
	if user.GuestConferenceID != nil {
		return nil
	}
	enabled, err := s.ldapOnly(ctx)
	if err != nil {
		return err
	}
	if enabled != user.IsLDAP() {
		return apperrors.ErrUnauthorized
	}
	return nil
}

func (s *Service) ldapEmailLogin(ctx context.Context, request users.LoginRequest) (users.LoginResponse, error) {
	if users.ValidateEmail(request.Email) != nil || !validLDAPPassword(request.Password) {
		return users.LoginResponse{}, invalidCredentials()
	}
	provider, ok := s.ldap.(interface {
		AuthenticateEmail(context.Context, string, string) (users.User, error)
	})
	if !ok || s.sessions == nil {
		return users.LoginResponse{}, apperrors.ErrUnavailable
	}
	user, err := provider.AuthenticateEmail(ctx, request.Email, request.Password)
	if errors.Is(err, apperrors.ErrUnauthorized) {
		return users.LoginResponse{}, invalidCredentials()
	}
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

func validLDAPPassword(password string) bool {
	return password != "" && len(password) <= 4096 && utf8.ValidString(password) && !strings.ContainsRune(password, 0)
}

// LDAPLogin использует существующие SID и cookie после успешной корпоративной аутентификации.
func (s *Service) LDAPLogin(ctx context.Context, request users.LDAPLoginRequest) (users.LoginResponse, error) {
	enabled, err := s.ldapOnly(ctx)
	if err != nil {
		return users.LoginResponse{}, err
	}
	if !enabled {
		return users.LoginResponse{}, apperrors.New(apperrors.ErrForbidden, "Корпоративный вход отключён")
	}
	request.Login = strings.TrimSpace(request.Login)
	if request.Login == "" || len(request.Login) > 254 || !utf8.ValidString(request.Login) || !validLDAPPassword(request.Password) || strings.ContainsRune(request.Login, 0) {
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
