package app

import (
	"context"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/errorlogs"
)

type adminErrorLogRepository interface {
	List(context.Context, errorlogs.Query) (errorlogs.Page, error)
	Get(context.Context, string) (errorlogs.Event, error)
}

func (r *adminResources) ListErrors(ctx context.Context, query errorlogs.Query) (errorlogs.Page, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if r.errorLogs == nil {
		return errorlogs.Page{}, apperrors.ErrUnavailable
	}
	return r.errorLogs.List(ctx, query)
}

func (r *adminResources) ErrorDetail(ctx context.Context, id string) (errorlogs.Event, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if r.errorLogs == nil {
		return errorlogs.Event{}, apperrors.ErrUnavailable
	}
	return r.errorLogs.Get(ctx, id)
}
