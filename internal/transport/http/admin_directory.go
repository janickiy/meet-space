// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package httptransport

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/directory"
)

type adminDirectoryResources interface {
	DirectoryStatus(context.Context) (directory.SyncStatus, error)
	DirectoryTest(context.Context, int64, admin.AuditInput) (directory.TestResult, error)
	DirectorySync(context.Context, int64, admin.AuditInput) error
}

func (h *adminHandler) registerDirectory(group *gin.RouterGroup) {
	resource := func(c *gin.Context) (adminDirectoryResources, bool) {
		r, ok := h.resources.(adminDirectoryResources)
		if !ok {
			adminFailure(c, apperrors.ErrUnavailable)
		}
		return r, ok
	}
	group.GET("/directory/status", func(c *gin.Context) {
		if r, ok := resource(c); ok {
			result, err := r.DirectoryStatus(c.Request.Context())
			adminResult(c, result, err)
		}
	})
	group.POST("/directory/test", func(c *gin.Context) {
		if r, ok := resource(c); ok {
			revision, valid := directoryRevision(c)
			if !valid {
				return
			}
			result, err := r.DirectoryTest(c.Request.Context(), revision, h.audit(c))
			adminResult(c, result, err)
		}
	})
	group.POST("/directory/sync", func(c *gin.Context) {
		if r, ok := resource(c); ok {
			revision, valid := directoryRevision(c)
			if !valid {
				return
			}
			err := r.DirectorySync(c.Request.Context(), revision, h.audit(c))
			adminResult(c, gin.H{"status": "queued"}, err)
		}
	})
}

func directoryRevision(c *gin.Context) (int64, bool) {
	var input struct {
		Revision *int64 `json:"revision"`
	}
	if !adminDecode(c, &input) {
		return 0, false
	}
	if input.Revision == nil || *input.Revision < 1 {
		adminFailure(c, apperrors.New(apperrors.ErrInvalidInput, "Укажите ревизию сохранённых настроек"))
		return 0, false
	}
	return *input.Revision, true
}
