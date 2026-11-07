// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/directory"
	"github.com/janickiy/meet-space/internal/domain/jobs"
)

// Параллельные ручные и периодические запросы создают одно задание с проверяемым состоянием и интервалом.
func TestLDAPSyncQueueDeduplicationAndInterval(t *testing.T) {
	db, _, ctx, actor, _ := settingsFixture(t)
	if err := db.Exec(`UPDATE platform_settings SET values=jsonb_set(values,'{ldapEnabled}','true'::jsonb),revision=1`).Error; err != nil {
		t.Fatal(err)
	}
	queue := NewDirectorySyncRepository(db)
	if ok, err := queue.Enqueue(ctx, 1, time.Hour, true, actor); err != nil || !ok {
		t.Fatalf("manual enqueue: %v %v", ok, err)
	}
	var wait sync.WaitGroup
	result := make(chan error, 8)
	for i := range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ok, err := queue.Enqueue(ctx, 1, time.Hour, i%2 == 0, actor)
			if err == nil && ok {
				err = errors.New("duplicate sync queued")
			}
			result <- err
		}()
	}
	wait.Wait()
	close(result)
	for err := range result {
		if err != nil {
			t.Fatal(err)
		}
	}
	status, err := queue.Status(ctx)
	if err != nil || status.Status != "queued" || !status.Queued {
		t.Fatalf("queued status: %+v %v", status, err)
	}
	var total int64
	if err := db.Table("background_jobs").Where("kind=?", directory.SyncJobKind).Count(&total).Error; err != nil || total != 1 {
		t.Fatalf("dedup failed: %d %v", total, err)
	}
	jobsRepo := NewJobRepository(db)
	job, claimed, err := jobsRepo.Claim(ctx, directory.SyncJobKind, time.Minute)
	if err != nil || !claimed || job.Version != 1 {
		t.Fatalf("claim: %+v %v %v", job, claimed, err)
	}
	started, err := queue.Begin(ctx, job)
	if err != nil || started.IsZero() {
		t.Fatalf("begin: %v %v", started, err)
	}
	status, err = queue.Status(ctx)
	if err != nil || status.Status != "running" || status.LastStartedAt == nil {
		t.Fatalf("running status: %+v %v", status, err)
	}
	counts := directory.LDAPApplyResult{Created: 2, Updated: 3, Disabled: 1}
	if err := queue.Complete(ctx, job, counts, 6, ""); err != nil {
		t.Fatal(err)
	}
	if err := jobsRepo.Finish(ctx, job, "done", "", nil); err != nil {
		t.Fatal(err)
	}
	status, err = queue.Status(ctx)
	if err != nil || status.Status != "succeeded" || status.Queued || status.Imported != 2 || status.Updated != 3 || status.Disabled != 1 || status.Total != 6 || status.LastSucceededAt == nil {
		t.Fatalf("completed status: %+v %v", status, err)
	}
	if ok, err := queue.Enqueue(context.Background(), 1, time.Hour, false, admin.AuditInput{}); err != nil || ok {
		t.Fatalf("interval ignored: %v %v", ok, err)
	}
	if err := db.Exec("UPDATE ldap_sync_runs SET last_requested_at=clock_timestamp()-interval '2 hours'").Error; err != nil {
		t.Fatal(err)
	}
	if ok, err := queue.Enqueue(context.Background(), 1, time.Hour, false, admin.AuditInput{}); err != nil || !ok {
		t.Fatalf("periodic job not enqueued: %v %v", ok, err)
	}
}

// Новый владелец аренды блокирует старый Begin/Complete и применение снимка; сбой сохраняет последний успешный результат времени.
func TestLDAPSyncQueueLeaseAndRevision(t *testing.T) {
	db := ldapAuditDatabase(t)
	ctx := context.Background()
	queue := NewDirectorySyncRepository(db)
	jobsRepo := NewJobRepository(db)
	if ok, err := queue.Enqueue(ctx, 1, time.Hour, false, admin.AuditInput{}); err != nil || !ok {
		t.Fatalf("enqueue: %v %v", ok, err)
	}
	first, claimed, err := jobsRepo.Claim(ctx, directory.SyncJobKind, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	if _, err := queue.Begin(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE background_jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=?", first.ID).Error; err != nil {
		t.Fatal(err)
	}
	second, claimed, err := jobsRepo.Claim(ctx, directory.SyncJobKind, time.Minute)
	if err != nil || !claimed || second.ID != first.ID || second.LeaseToken == first.LeaseToken {
		t.Fatalf("reclaim: %v %v", claimed, err)
	}
	if _, err := queue.Begin(ctx, first); !errors.Is(err, jobs.ErrLeaseLost) {
		t.Fatalf("stale begin: %v", err)
	}
	if err := queue.Complete(ctx, first, directory.LDAPApplyResult{Created: 100}, 100, ""); !errors.Is(err, jobs.ErrLeaseLost) {
		t.Fatalf("stale completion: %v", err)
	}
	if _, err := NewDirectoryRepository(db).ApplyLDAPSnapshot(directory.WithSyncLease(ctx, first), []directory.Entry{ldapAuditEntry()}, 1, ldapAuditNow(t, db)); !errors.Is(err, jobs.ErrLeaseLost) {
		t.Fatalf("stale apply: %v", err)
	}
	if _, err := queue.Begin(ctx, second); err != nil {
		t.Fatal(err)
	}
	if err := queue.Complete(ctx, second, directory.LDAPApplyResult{}, 0, "ldap_unavailable"); err != nil {
		t.Fatal(err)
	}
	if err := jobsRepo.Finish(ctx, second, "failed", "ldap_unavailable", nil); err != nil {
		t.Fatal(err)
	}
	status, err := queue.Status(ctx)
	if err != nil || status.Status != "failed" || status.ErrorCode != "ldap_unavailable" || status.Imported != 0 || status.LastSucceededAt != nil {
		t.Fatalf("failed status: %+v %v", status, err)
	}
	if err := db.Exec("UPDATE platform_settings SET revision=2").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Enqueue(ctx, 1, time.Hour, false, admin.AuditInput{}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("stale revision queued: %v", err)
	}
	if ok, err := queue.Enqueue(ctx, 2, time.Hour, false, admin.AuditInput{}); err != nil || !ok {
		t.Fatalf("new configuration suppressed by interval: %v %v", ok, err)
	}
	invalid := admin.WithAuthorization(ctx, uuid.NewString(), "revoked")
	if _, err := queue.Enqueue(invalid, 2, time.Hour, true, admin.AuditInput{}); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatalf("revoked administrator queued sync: %v", err)
	}
}

// Истечение аренды в середине применения откатывает даже уже выполненные INSERT, а не только статистику задания.
func TestLDAPSnapshotLeaseExpiresBeforeCommit(t *testing.T) {
	db := ldapAuditDatabase(t)
	ctx := context.Background()
	queue := NewDirectorySyncRepository(db)
	if _, err := queue.Enqueue(ctx, 1, time.Hour, false, admin.AuditInput{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE FUNCTION audit_slow_ldap_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.auth_source='ldap' THEN PERFORM pg_sleep(0.6); END IF; RETURN NEW; END $$;
 CREATE TRIGGER audit_slow_ldap_insert BEFORE INSERT ON users FOR EACH ROW EXECUTE FUNCTION audit_slow_ldap_insert()`).Error; err != nil {
		t.Fatal(err)
	}
	job, ok, err := NewJobRepository(db).Claim(ctx, directory.SyncJobKind, 500*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	started, err := queue.Begin(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	entry := ldapAuditEntry()
	result, err := NewDirectoryRepository(db).ApplyLDAPSnapshot(directory.WithSyncLease(ctx, job), []directory.Entry{entry}, 1, started)
	if !errors.Is(err, jobs.ErrLeaseLost) || result.Created != 0 {
		t.Fatalf("expired lease committed result: %+v %v", result, err)
	}
	var count int64
	if err := db.Table("users").Where("email=?", entry.Email).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("expired transaction left a user: %d %v", count, err)
	}
	var advanced bool
	if err := db.Raw("SELECT last_snapshot_at IS NOT NULL FROM ldap_directory_state").Scan(&advanced).Error; err != nil || advanced {
		t.Fatalf("expired transaction advanced watermark: %v %v", advanced, err)
	}
}
