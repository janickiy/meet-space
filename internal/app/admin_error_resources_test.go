package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/errorlogs"
)

type adminErrorRepoProbe struct {
	query    errorlogs.Query
	id       string
	observed context.Context
	result   error
}

func (p *adminErrorRepoProbe) List(ctx context.Context, q errorlogs.Query) (errorlogs.Page, error) {
	p.observed = ctx
	p.query = q
	return errorlogs.Page{Items: []errorlogs.Event{{ID: "00000000-0000-4000-8000-000000000001"}}}, p.result
}
func (p *adminErrorRepoProbe) Get(ctx context.Context, id string) (errorlogs.Event, error) {
	p.observed = ctx
	p.id = id
	return errorlogs.Event{ID: id}, p.result
}
func TestAdminErrorResourcesBoundRequestsAndDelegate(t *testing.T) {
	probe := &adminErrorRepoProbe{}
	r := &adminResources{errorLogs: probe}
	query := errorlogs.Query{Service: "api", PageSize: 25}
	before := time.Now()
	page, err := r.ListErrors(context.Background(), query)
	if err != nil || len(page.Items) != 1 || probe.query != query {
		t.Fatal("list delegation failed", err)
	}
	deadline, ok := probe.observed.Deadline()
	if !ok || deadline.After(before.Add(6*time.Second)) {
		t.Fatal("error query has no bounded deadline")
	}
	if !errors.Is(probe.observed.Err(), context.Canceled) {
		t.Fatal("list context retained after response")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	original, _ := ctx.Deadline()
	detail, err := r.ErrorDetail(ctx, "00000000-0000-4000-8000-000000000001")
	if err != nil || detail.ID != probe.id {
		t.Fatal("detail delegation failed", err)
	}
	deadline, ok = probe.observed.Deadline()
	if !ok || deadline.After(original) {
		t.Fatal("detail request extended parent deadline")
	}
	if !errors.Is(probe.observed.Err(), context.Canceled) {
		t.Fatal("detail context retained after response")
	}
	probe.result = apperrors.ErrNotFound
	if _, err = r.ErrorDetail(context.Background(), "missing"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("missing error detail swallowed", err)
	}
}
func TestAdminErrorResourcesUnavailable(t *testing.T) {
	r := &adminResources{}
	if _, err := r.ListErrors(context.Background(), errorlogs.Query{}); !errors.Is(err, apperrors.ErrUnavailable) {
		t.Fatal("missing storage accepted", err)
	}
	if _, err := r.ErrorDetail(context.Background(), "missing"); !errors.Is(err, apperrors.ErrUnavailable) {
		t.Fatal("missing storage detail accepted", err)
	}
}
