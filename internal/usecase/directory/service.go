// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package directory

import (
	"context"
	"errors"
	"time"

	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/directory"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"github.com/janickiy/meet-space/internal/domain/users"
)

type Settings interface {
	LDAP(context.Context) (domain.Config, error)
}

type Accounts interface {
	UpsertLDAP(context.Context, domain.Entry, int64) (users.User, error)
	ApplyLDAPSnapshot(context.Context, []domain.Entry, int64, time.Time) (domain.LDAPApplyResult, error)
}

type Queue interface {
	Enqueue(context.Context, int64, time.Duration, bool, admin.AuditInput) (bool, error)
	Begin(context.Context, jobs.Job) (time.Time, error)
	Complete(context.Context, jobs.Job, domain.LDAPApplyResult, int, string) error
	Status(context.Context) (domain.SyncStatus, error)
}

// Service использует LDAP только при входе и синхронизации; обычные запросы проверяют локальную сессию.
type Service struct {
	settings Settings
	client   domain.Client
	accounts Accounts
	queue    Queue
}

func New(settings Settings, client domain.Client, accounts Accounts, queue Queue) *Service {
	return &Service{settings: settings, client: client, accounts: accounts, queue: queue}
}

func (s *Service) Enabled(ctx context.Context) (bool, error) {
	if reader, ok := s.settings.(interface {
		LDAPEnabled(context.Context) (bool, error)
	}); ok {
		return reader.LDAPEnabled(ctx)
	}
	cfg, err := s.settings.LDAP(ctx)
	return cfg.Enabled, err
}

func (s *Service) Authenticate(ctx context.Context, login, password string) (users.User, error) {
	return s.authenticate(ctx, login, password, false)
}

// AuthenticateEmail использует почтовый атрибут для входа, сохраняя отдельный LDAP логин профиля.
func (s *Service) AuthenticateEmail(ctx context.Context, email, password string) (users.User, error) {
	return s.authenticate(ctx, email, password, true)
}

func (s *Service) authenticate(ctx context.Context, identifier, password string, byEmail bool) (users.User, error) {
	cfg, err := s.settings.LDAP(ctx)
	if err != nil {
		return users.User{}, publicError(err)
	}
	if !cfg.Enabled {
		return users.User{}, apperrors.New(apperrors.ErrUnauthorized, "Корпоративный вход отключён")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	var entry domain.Entry
	if byEmail {
		client, ok := s.client.(interface {
			AuthenticateEmail(context.Context, domain.Config, string, string) (domain.Entry, error)
		})
		if !ok {
			return users.User{}, apperrors.ErrUnavailable
		}
		entry, err = client.AuthenticateEmail(ctx, cfg, identifier, password)
	} else {
		entry, err = s.client.Authenticate(ctx, cfg, identifier, password)
	}
	if err != nil {
		return users.User{}, publicError(err)
	}
	if entry.Disabled {
		return users.User{}, publicError(domain.ErrNotAllowed)
	}
	user, err := s.accounts.UpsertLDAP(ctx, entry, cfg.Revision)
	if err != nil {
		return users.User{}, publicError(err)
	}
	return user, nil
}

func (s *Service) Test(ctx context.Context, revision int64) (domain.TestResult, error) {
	cfg, err := s.settings.LDAP(ctx)
	if err != nil {
		return domain.TestResult{}, publicError(err)
	}
	if cfg.Revision != revision {
		return domain.TestResult{}, apperrors.New(apperrors.ErrConflict, "Настройки LDAP изменились. Обновите страницу и повторите проверку.")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	result, err := s.client.Test(ctx, cfg)
	if err != nil {
		return domain.TestResult{}, publicError(err)
	}
	current, err := s.settings.LDAP(ctx)
	if err != nil {
		return domain.TestResult{}, publicError(err)
	}
	if current.Revision != revision {
		return domain.TestResult{}, apperrors.New(apperrors.ErrConflict, "Настройки LDAP изменились во время проверки. Повторите проверку.")
	}
	return result, nil
}

func (s *Service) Status(ctx context.Context) (domain.SyncStatus, error) {
	return s.queue.Status(ctx)
}

// Enqueue синхронизирует запросы админки и таймеров всех экземпляров через одну строку БД.
func (s *Service) Enqueue(ctx context.Context, manual bool, audit admin.AuditInput, expectedRevision ...int64) error {
	cfg, err := s.settings.LDAP(ctx)
	if err != nil {
		return publicError(err)
	}
	if len(expectedRevision) > 0 && cfg.Revision != expectedRevision[0] {
		return apperrors.New(apperrors.ErrConflict, "Настройки LDAP изменились. Обновите страницу и повторите синхронизацию.")
	}
	if !cfg.Enabled {
		if manual {
			return apperrors.New(apperrors.ErrInvalidInput, "Сначала сохраните и включите LDAP")
		}
		return nil
	}
	_, err = s.queue.Enqueue(ctx, cfg.Revision, time.Duration(cfg.SyncIntervalMinutes)*time.Minute, manual, audit)
	return err
}

func (s *Service) Handle(ctx context.Context, job jobs.Job) error {
	if job.Kind != domain.SyncJobKind {
		return jobs.ErrSkip
	}
	startedAt, err := s.queue.Begin(ctx, job)
	if err != nil {
		return err
	}
	cfg, err := s.settings.LDAP(ctx)
	if err != nil {
		return s.failed(ctx, job, err)
	}
	if !cfg.Enabled || cfg.Revision != job.Version {
		finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := s.queue.Complete(finish, job, domain.LDAPApplyResult{}, 0, "directory_config_changed"); err != nil {
			return err
		}
		return jobs.ErrSkip
	}
	entries, err := s.client.Scan(ctx, cfg)
	if err != nil {
		return s.failed(ctx, job, err)
	}
	// Только полностью полученный и проверенный снимок попадает в атомарную транзакцию.
	result, err := s.accounts.ApplyLDAPSnapshot(domain.WithSyncLease(ctx, job), entries, cfg.Revision, startedAt)
	if err != nil {
		return s.failed(ctx, job, err)
	}
	return s.queue.Complete(ctx, job, result, len(entries), "")
}

func (s *Service) failed(ctx context.Context, job jobs.Job, cause error) error {
	code, retry := failureCode(cause)
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.queue.Complete(finish, job, domain.LDAPApplyResult{}, 0, code); err != nil {
		return err
	}
	return jobs.Error{Code: code, Retryable: retry}
}

// Fail завершает отображаемое состояние после исчерпания повторов или аварии обработчика.
func (s *Service) Fail(ctx context.Context, job jobs.Job, code string) error {
	return s.queue.Complete(ctx, job, domain.LDAPApplyResult{}, 0, code)
}

func failureCode(err error) (string, bool) {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "directory_timeout", true
	case errors.Is(err, domain.ErrInvalidConfig):
		return "directory_invalid_config", false
	case errors.Is(err, domain.ErrIncompleteScan):
		return "directory_incomplete_scan", true
	case errors.Is(err, domain.ErrUnavailable), errors.Is(err, apperrors.ErrUnavailable):
		return "directory_unavailable", true
	case errors.Is(err, apperrors.ErrConflict):
		return "directory_conflict", false
	default:
		return "directory_persistence", true
	}
}

func publicError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domain.ErrInvalidCredentials), errors.Is(err, domain.ErrNotAllowed):
		return apperrors.New(apperrors.ErrUnauthorized, "Не удалось войти. Проверьте корпоративный логин и пароль.")
	case errors.Is(err, apperrors.ErrConflict):
		return apperrors.New(apperrors.ErrConflict, "Не удалось связать учётную запись с каталогом. Обратитесь к администратору.")
	case errors.Is(err, domain.ErrInvalidConfig):
		return apperrors.New(apperrors.ErrInvalidInput, "Проверьте настройки подключения LDAP")
	default:
		return apperrors.New(apperrors.ErrUnavailable, "Каталог пользователей временно недоступен. Повторите попытку позже.")
	}
}
