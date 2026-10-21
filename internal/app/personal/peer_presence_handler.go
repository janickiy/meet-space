// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package personalapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	personalusecase "github.com/janickiy/meet-space/internal/usecase/personal"
)

// PeerPresence возвращает текущее онлайн-состояние пирингового узла только в рамках авторизованного прямого диалога.
//
// @Parameters:
// - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) PeerPresence(c *gin.Context) {
	id, err := chat.UUID(c.Param("id"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"status": "failed", "message": "invalid conversationId"})
		return
	}
	actor, err := chat.UUID(middleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, apperrors.ErrUnauthorized)
		return
	}
	item, err := personalusecase.ReadPeerPresence(c.Request.Context(), h.Repo, h.Presence, actor, id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}
