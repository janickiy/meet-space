// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package personalapp

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/personal"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

type InfoRepository interface {
	ConversationProfile(context.Context, string, string, string) (personal.Profile, error)
	ConversationMaterials(context.Context, string, string, string, string, int) (chat.MaterialPage, error)
}

// Profile returns the requested member's profile after checking the shared chat.
func (h *Handler) Profile(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	userID, ok := uuidParameter(c, "userId")
	if !ok {
		return
	}
	item, err := h.Repo.ConversationProfile(c.Request.Context(), middleware.UserID(c), id, userID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// Materials pages through the caller's visible media, files or links, without
// fetching the entire history or changing read markers.
func (h *Handler) Materials(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	limit := 50
	if raw, present := c.GetQuery("limit"); present {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		limit = value
	}
	page, err := h.Repo.ConversationMaterials(c.Request.Context(), middleware.UserID(c), id, c.Query("kind"), c.Query("before"), limit)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "nextCursor": page.NextCursor})
}
