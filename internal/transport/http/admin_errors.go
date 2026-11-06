package httptransport

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/errorlogs"
)

// ErrorResources is optional so the technical log does not change the boundary
// used by the existing administrative resource endpoints or their consumers.
type ErrorResources interface {
	ListErrors(context.Context, errorlogs.Query) (errorlogs.Page, error)
	ErrorDetail(context.Context, string) (errorlogs.Event, error)
}

func errorQuery(c *gin.Context) (errorlogs.Query, error) {
	invalid := func() (errorlogs.Query, error) {
		return errorlogs.Query{}, apperrors.New(apperrors.ErrInvalidInput, "Некорректные параметры журнала ошибок")
	}
	if len(c.Request.URL.RawQuery) > 2048 {
		return invalid()
	}
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		return invalid()
	}
	allowed := map[string]bool{"service": true, "level": true, "code": true, "requestId": true, "from": true, "to": true, "cursor": true, "pageSize": true}
	for key, entries := range values {
		if !allowed[key] || len(entries) != 1 {
			return invalid()
		}
	}
	query := errorlogs.Query{
		Service: values.Get("service"), Level: values.Get("level"), Code: values.Get("code"),
		RequestID: values.Get("requestId"), From: values.Get("from"), To: values.Get("to"), Before: values.Get("cursor"),
	}
	switch strings.ToLower(query.Level) {
	case "error":
		query.Level = "ERROR"
	case "warn", "warning":
		query.Level = "WARN"
	}
	if size := values.Get("pageSize"); size != "" {
		query.PageSize, err = strconv.Atoi(size)
		if err != nil || query.PageSize < 1 || query.PageSize > 100 {
			return invalid()
		}
	}
	return errorlogs.ValidateQuery(query)
}

func (h *adminHandler) errorList(c *gin.Context) {
	query, err := errorQuery(c)
	if err != nil {
		adminFailure(c, err)
		return
	}
	resources, ok := h.resources.(ErrorResources)
	if !ok {
		adminFailure(c, apperrors.ErrUnavailable)
		return
	}
	page, err := resources.ListErrors(c.Request.Context(), query)
	adminResult(c, page, err)
}

func (h *adminHandler) errorDetail(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id.String() != c.Param("id") {
		adminFailure(c, apperrors.New(apperrors.ErrInvalidInput, "Некорректный идентификатор ошибки"))
		return
	}
	if len(c.Request.URL.RawQuery) != 0 {
		adminFailure(c, apperrors.New(apperrors.ErrInvalidInput, "Параметры подробностей ошибки не поддерживаются"))
		return
	}
	resources, ok := h.resources.(ErrorResources)
	if !ok {
		adminFailure(c, apperrors.ErrUnavailable)
		return
	}
	event, err := resources.ErrorDetail(c.Request.Context(), id.String())
	adminResult(c, event, err)
}
