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
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// Pins отображает общие пины с использованием видимой истории сообщений текущего участника.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Pins(c *gin.Context) {
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
	page, err := h.Repo.ConversationPins(c.Request.Context(), middleware.UserID(c), id, c.Query("before"), limit)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

// SetPin сохраняет общее состояние закрепления, прежде чем предложить текущим участникам обновить его.
// Событие не содержит идентификатора сообщения, текста или предварительного просмотра из истории другого участника.
//
// @Parameters:
//   - pinned: нужно ли закрепить выбранное сообщение.
//
// @returns:
//   - result1: обработчик HTTP для регистрации этой операции.
func (h *Handler) SetPin(pinned bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := uuidParameter(c, "id")
		if !ok {
			return
		}
		messageID, err := chat.UUID(c.Param("messageId"))
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		kind, err := h.Repo.SetConversationPin(c.Request.Context(), middleware.UserID(c), id, messageID, pinned)
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		if h.Events != nil {
			_ = h.Events.PublishConversation(c.Request.Context(), id, "conversation.pins.updated", gin.H{"conversationId": id, "type": kind})
		}
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	}
}
