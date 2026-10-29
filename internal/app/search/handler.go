// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package search

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	apperrors "github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/search"
	"github.com/janickiy/meet-space/internal/operations"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

type Service interface {
	Search(context.Context, string, domain.Query) (domain.Page, error)
}
type Handler struct{ Service Service }

func (h *Handler) Search(c *gin.Context) {
	started := time.Now()
	defer func() { operations.Search(time.Since(started), c.Writer.Status() >= http.StatusBadRequest) }()
	q := domain.Query{Query: c.Query("q"), Source: c.Query("source"), ConferenceID: c.Query("conferenceId"), Mode: c.Query("mode"), ParticipantID: c.Query("participantId"), Membership: c.Query("membership"), Limit: 20}
	for key, target := range map[string]*int{"limit": &q.Limit, "offset": &q.Offset} {
		if value, exists := c.GetQuery(key); exists {
			n, err := strconv.Atoi(value)
			if err != nil {
				httpresponse.Fail(c, apperrors.ErrInvalidInput)
				return
			}
			*target = n
		}
	}
	for key, target := range map[string]**time.Time{"from": &q.From, "to": &q.To} {
		if value, exists := c.GetQuery(key); exists {
			at, err := time.Parse(time.RFC3339, value)
			if err != nil {
				httpresponse.Fail(c, apperrors.ErrInvalidInput)
				return
			}
			utc := at.UTC()
			*target = &utc
		}
	}
	page, err := h.Service.Search(c.Request.Context(), httpmiddleware.UserID(c), q)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "total": page.Total, "limit": page.Limit, "offset": page.Offset, "effectiveMode": page.EffectiveMode})
}
