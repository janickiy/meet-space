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
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/directory"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DirectorySyncRepository хранит очередь и результаты отдельно от содержимого каталога.
type DirectorySyncRepository struct{ db *gorm.DB }

func NewDirectorySyncRepository(db *gorm.DB) *DirectorySyncRepository {
	return &DirectorySyncRepository{db: db.Session(&gorm.Session{Logger: logger.Discard})}
}

func (r *DirectorySyncRepository) Enqueue(ctx context.Context, revision int64, interval time.Duration, manual bool, audit admin.AuditInput) (bool, error) {
	queued := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if manual {
			if err := adminMutationGuard(tx); err != nil {
				return err
			}
		}
		var current struct{ Revision int64 }
		result := tx.Raw(`SELECT revision FROM platform_settings WHERE singleton=TRUE
			AND COALESCE((values->>'ldapEnabled')::boolean,FALSE) AND revision=? FOR SHARE`, revision).Scan(&current)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return apperrors.New(apperrors.ErrConflict, "Настройки LDAP изменены. Обновите страницу.")
		}
		var state struct {
			RecentlyRequested bool
			RequestedRevision int64
		}
		if err := tx.Raw(`SELECT COALESCE(last_requested_at+(? * interval '1 millisecond')>clock_timestamp(),FALSE) AS recently_requested,
			requested_revision FROM ldap_sync_runs WHERE singleton=TRUE FOR UPDATE`, interval.Milliseconds()).Scan(&state).Error; err != nil {
			return err
		}
		var pending bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM background_jobs WHERE kind=? AND state IN ('queued','processing'))`, directory.SyncJobKind).Scan(&pending).Error; err != nil {
			return err
		}
		if pending {
			return nil
		}
		if !manual && state.RequestedRevision == revision && state.RecentlyRequested {
			return nil
		}
		id := uuid.NewString()
		if err := tx.Exec(`INSERT INTO background_jobs(id,kind,entity_id,conference_id,version,dedup_key,max_attempts)
			VALUES(?::uuid,?,?::uuid,?::uuid,?,?,3)`, id, directory.SyncJobKind, uuid.Nil.String(), uuid.Nil.String(), revision, "directory.sync:"+id).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE ldap_sync_runs SET last_requested_at=clock_timestamp(),requested_revision=?,
			job_id=?::uuid,lease_token=NULL,status='queued',error_code='' WHERE singleton=TRUE`, revision, id).Error; err != nil {
			return err
		}
		queued = true
		if manual {
			audit.Action, audit.ObjectType, audit.ObjectID = "directory.sync_requested", "directory", "ldap"
			audit.Metadata = map[string]any{"status": "queued"}
			return auditInsert(tx, audit)
		}
		return nil
	})
	return queued, err
}

func checkDirectoryJobLease(tx *gorm.DB, job jobs.Job) error {
	var id string
	result := tx.Raw(`SELECT id FROM background_jobs WHERE id=?::uuid AND kind=? AND state='processing'
		AND lease_token=?::uuid AND lease_until>clock_timestamp() FOR UPDATE`, job.ID, directory.SyncJobKind, job.LeaseToken).Scan(&id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return jobs.ErrLeaseLost
	}
	return nil
}

func (r *DirectorySyncRepository) Begin(ctx context.Context, job jobs.Job) (time.Time, error) {
	var started time.Time
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := checkDirectoryJobLease(tx, job); err != nil {
			return err
		}
		result := tx.Raw(`UPDATE ldap_sync_runs SET status='running',last_started_at=clock_timestamp(),
			lease_token=?::uuid,error_code='' WHERE singleton=TRUE AND job_id=?::uuid RETURNING last_started_at`, job.LeaseToken, job.ID).Scan(&started)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return jobs.ErrLeaseLost
		}
		return nil
	})
	return started, err
}

func (r *DirectorySyncRepository) Complete(ctx context.Context, job jobs.Job, counts directory.LDAPApplyResult, total int, code string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := checkDirectoryJobLease(tx, job); err != nil {
			return err
		}
		status := "succeeded"
		if code != "" {
			status = "failed"
		}
		result := tx.Exec(`UPDATE ldap_sync_runs SET status=?,error_code=?,last_completed_at=clock_timestamp(),
			last_succeeded_at=CASE WHEN ?='' THEN clock_timestamp() ELSE last_succeeded_at END,
			imported=?,updated=?,disabled=?,total=? WHERE singleton=TRUE AND job_id=?::uuid`,
			status, code, code, counts.Created, counts.Updated, counts.Disabled, total, job.ID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return jobs.ErrLeaseLost
		}
		return nil
	})
}

func (r *DirectorySyncRepository) Status(ctx context.Context) (directory.SyncStatus, error) {
	var status directory.SyncStatus
	err := r.db.WithContext(ctx).Raw(`SELECT status,last_started_at,last_completed_at,last_succeeded_at,error_code,
		imported,updated,disabled,total,EXISTS(SELECT 1 FROM background_jobs WHERE kind=? AND state IN ('queued','processing')) AS queued
		FROM ldap_sync_runs WHERE singleton=TRUE`, directory.SyncJobKind).Scan(&status).Error
	return status, err
}
