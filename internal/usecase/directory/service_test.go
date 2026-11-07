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
	"testing"
	"time"

	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/directory"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"github.com/janickiy/meet-space/internal/domain/users"
)

type settingsStub struct{ config domain.Config }

func (s settingsStub) LDAP(context.Context) (domain.Config, error) { return s.config, nil }

type clientStub struct {
	err    error
	called bool
}

func (c *clientStub) Authenticate(context.Context, domain.Config, string, string) (domain.Entry, error) {
	c.called = true
	return domain.Entry{ExternalID: "stable-id", Email: "test@example.invalid"}, c.err
}
func (c *clientStub) Scan(context.Context, domain.Config) ([]domain.Entry, error) {
	c.called = true
	return []domain.Entry{{ExternalID: "stable-id", Email: "test@example.invalid"}}, c.err
}
func (c *clientStub) Test(context.Context, domain.Config) (domain.TestResult, error) {
	return domain.TestResult{}, c.err
}

type accountsStub struct {
	called bool
	lease  jobs.Job
}

func (a *accountsStub) UpsertLDAP(context.Context, domain.Entry, int64) (users.User, error) {
	a.called = true
	return users.User{}, nil
}
func (a *accountsStub) ApplyLDAPSnapshot(ctx context.Context, entries []domain.Entry, revision int64, started time.Time) (domain.LDAPApplyResult, error) {
	a.called = true
	a.lease, _ = domain.SyncLeaseFromContext(ctx)
	return domain.LDAPApplyResult{Created: len(entries)}, nil
}

type queueStub struct {
	code          string
	called        bool
	result        domain.LDAPApplyResult
	completeError error
}

func (q *queueStub) Enqueue(context.Context, int64, time.Duration, bool, admin.AuditInput) (bool, error) {
	q.called = true
	return true, nil
}
func (q *queueStub) Begin(context.Context, jobs.Job) (time.Time, error) { return time.Now(), nil }
func (q *queueStub) Complete(_ context.Context, _ jobs.Job, result domain.LDAPApplyResult, _ int, code string) error {
	q.code = code
	q.result = result
	return q.completeError
}
func (q *queueStub) Status(context.Context) (domain.SyncStatus, error) {
	return domain.SyncStatus{}, nil
}

func TestSupersededConfigPreservesStatusPersistenceFailure(t *testing.T) {
	want := errors.New("database unavailable")
	queue := &queueStub{completeError: want}
	service := New(settingsStub{domain.Config{Revision: 5}}, &clientStub{}, &accountsStub{}, queue)
	if err := service.Handle(context.Background(), jobs.Job{Kind: domain.SyncJobKind, Version: 4}); !errors.Is(err, want) {
		t.Fatalf("lost completion failure: %v", err)
	}
}

type changingSettings struct{ revision int64 }

func (s *changingSettings) LDAP(context.Context) (domain.Config, error) {
	previous := s.revision
	s.revision++
	return domain.Config{Revision: previous}, nil
}
func TestConnectionCheckRejectsConcurrentSettingsChange(t *testing.T) {
	service := New(&changingSettings{revision: 5}, &clientStub{}, &accountsStub{}, &queueStub{})
	if _, err := service.Test(context.Background(), 5); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("test confirmed a superseded configuration: %v", err)
	}
}

func TestFailedDirectoryReadCannotModifyAccounts(t *testing.T) {
	for _, err := range []error{domain.ErrIncompleteScan, domain.ErrUnavailable, context.DeadlineExceeded} {
		t.Run(err.Error(), func(t *testing.T) {
			client, accounts, queue := &clientStub{err: err}, &accountsStub{}, &queueStub{}
			service := New(settingsStub{domain.Config{Enabled: true, Revision: 4}}, client, accounts, queue)
			if err := service.Handle(context.Background(), jobs.Job{Kind: domain.SyncJobKind, Version: 4}); err == nil {
				t.Fatal("failed scan accepted")
			}
			if accounts.called {
				t.Fatal("partial scan reached account persistence")
			}
			if queue.code == "" {
				t.Fatal("failure not visible in admin status")
			}
		})
	}
}

func TestSupersededConfigDoesNotReadOrModifyDirectory(t *testing.T) {
	client, accounts, queue := &clientStub{}, &accountsStub{}, &queueStub{}
	service := New(settingsStub{domain.Config{Enabled: true, Revision: 5}}, client, accounts, queue)
	if err := service.Handle(context.Background(), jobs.Job{Kind: domain.SyncJobKind, Version: 4}); !errors.Is(err, jobs.ErrSkip) {
		t.Fatal(err)
	}
	if client.called || accounts.called {
		t.Fatal("stale job used current configuration")
	}
}

func TestSnapshotPersistenceReceivesWorkerLease(t *testing.T) {
	client, accounts, queue := &clientStub{}, &accountsStub{}, &queueStub{}
	service := New(settingsStub{domain.Config{Enabled: true, Revision: 4}}, client, accounts, queue)
	job := jobs.Job{ID: "job", LeaseToken: "lease", Kind: domain.SyncJobKind, Version: 4}
	if err := service.Handle(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if accounts.lease.ID != job.ID || accounts.lease.LeaseToken != job.LeaseToken || queue.result.Created != 1 {
		t.Fatal("lost lease or counters")
	}
}

func TestInvalidLDAPLoginNeverCreatesAccount(t *testing.T) {
	client, accounts, queue := &clientStub{err: domain.ErrInvalidCredentials}, &accountsStub{}, &queueStub{}
	service := New(settingsStub{domain.Config{Enabled: true, Revision: 4, TimeoutSeconds: 5}}, client, accounts, queue)
	if _, err := service.Authenticate(context.Background(), "employee", "wrong"); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal(err)
	}
	if accounts.called {
		t.Fatal("invalid login created an account")
	}
}

func TestDisabledDirectoryRetainsLocalOnlyOperation(t *testing.T) {
	client, accounts, queue := &clientStub{}, &accountsStub{}, &queueStub{}
	service := New(settingsStub{}, client, accounts, queue)
	if _, err := service.Authenticate(context.Background(), "employee", "password"); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal(err)
	}
	if err := service.Enqueue(context.Background(), false, admin.AuditInput{}); err != nil {
		t.Fatal(err)
	}
	if err := service.Enqueue(context.Background(), true, admin.AuditInput{}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal(err)
	}
	if client.called || accounts.called || queue.called {
		t.Fatal("disabled LDAP performed work")
	}
}
