// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/directory"
	ldapinfra "github.com/janickiy/meet-space/internal/infrastructure/ldap"
	pg "github.com/janickiy/meet-space/internal/infrastructure/postgres"
	directoryusecase "github.com/janickiy/meet-space/internal/usecase/directory"
	settingsusecase "github.com/janickiy/meet-space/internal/usecase/settings"
	"gorm.io/gorm"
)

func newDirectoryService(db *gorm.DB, settings *settingsusecase.Service) *directoryusecase.Service {
	return directoryusecase.New(settings, ldapinfra.NewClient(), pg.NewDirectoryRepository(db), pg.NewDirectorySyncRepository(db))
}

func (r *adminResources) DirectoryStatus(ctx context.Context) (directory.SyncStatus, error) {
	if r.directory == nil {
		return directory.SyncStatus{}, apperrors.ErrUnavailable
	}
	return r.directory.Status(ctx)
}

func (r *adminResources) DirectoryTest(ctx context.Context, revision int64, audit admin.AuditInput) (directory.TestResult, error) {
	if r.directory == nil {
		return directory.TestResult{}, apperrors.ErrUnavailable
	}
	result, err := r.directory.Test(ctx, revision)
	audit.Action, audit.ObjectType, audit.ObjectID = "directory.test", "directory", "ldap"
	status := "succeeded"
	if err != nil {
		status = "failed"
	}
	audit.Metadata = map[string]any{"status": status}
	if auditErr := r.repository.Audit(ctx, audit); auditErr != nil {
		return directory.TestResult{}, auditErr
	}
	return result, err
}

func (r *adminResources) DirectorySync(ctx context.Context, revision int64, audit admin.AuditInput) error {
	if r.directory == nil {
		return apperrors.ErrUnavailable
	}
	return r.directory.Enqueue(ctx, true, audit, revision)
}

// Один таймер на процесс, а уникальное задание в БД объединяет все экземпляры воркера.
func runDirectoryTicks(ctx context.Context, service *directoryusecase.Service) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := service.Enqueue(bounded, false, admin.AuditInput{})
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.Warn("LDAP scheduler unavailable", "event_type", "directory.scheduler.failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
