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
	"time"

	"github.com/janickiy/meet-space/internal/domain/jobs"
)

const SyncJobKind = "directory.sync"

// SyncStatus содержит только безопасную сводку, без записей каталога и секретов.
type SyncStatus struct {
	Status          string     `json:"status"`
	LastStartedAt   *time.Time `json:"lastStartedAt"`
	LastCompletedAt *time.Time `json:"lastCompletedAt"`
	LastSucceededAt *time.Time `json:"lastSucceededAt"`
	ErrorCode       string     `json:"errorCode"`
	Imported        int        `json:"imported"`
	Updated         int        `json:"updated"`
	Disabled        int        `json:"disabled"`
	Total           int        `json:"total"`
	Queued          bool       `json:"queued"`
}

type syncLeaseKey struct{}

// WithSyncLease позволяет репозиторию проверить право воркера на фиксацию снимка.
func WithSyncLease(ctx context.Context, job jobs.Job) context.Context {
	return context.WithValue(ctx, syncLeaseKey{}, job)
}

func SyncLeaseFromContext(ctx context.Context) (jobs.Job, bool) {
	job, ok := ctx.Value(syncLeaseKey{}).(jobs.Job)
	return job, ok
}
