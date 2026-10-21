// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package callsapp

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	usecase "github.com/janickiy/meet-space/internal/usecase/calls"
)

type Handler struct{ Service *usecase.Service }

// validID проверяет, является ли предоставленный идентификатор ресурса допустимым ненулевым UUID.
//
// @Parameters:
//   - value: значение для проверки, нормализации или преобразования.
//
// @returns:
//   - result1: выполняется ли задокументированное условие операции или ее условное состояние
//     изменено успешно.
func validID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

// Create проверяет запрос и возвращает только что созданный прямой вызов для действующего аккаунта.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Create(c *gin.Context) {
	var body struct {
		TargetUserID    string `json:"targetUserId"`
		ClientRequestID string `json:"clientRequestId"`
		ClientID        string `json:"clientId"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	actor := middleware.UserID(c)
	if !validID(body.TargetUserID) || !validID(body.ClientRequestID) || !validID(body.ClientID) || actor == body.TargetUserID {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	item, created, err := h.Service.Create(c.Request.Context(), actor, body.TargetUserID, body.ClientRequestID, body.ClientID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	code := 200
	if created {
		code = 201
	}
	c.JSON(code, gin.H{"status": "success", "item": item})
}

// Get обслуживает запрошенный прямой вызов в рамках полномочий вызывающей стороны.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Get(c *gin.Context) {
	id := c.Param("id")
	if !validID(id) {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	item, err := h.Service.Get(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success", "item": item})
}

// Active возвращает текущий нетерминальный прямой вызов аккаунта, если таковой имеется.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Active(c *gin.Context) {
	item, err := h.Service.Active(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success", "item": item})
}

// Change применяет запрошенное действие жизненного цикла прямого вызова и возвращает обновленное состояние.
//
// @Parameters:
//   - action: Запрошенное действие жизненного цикла или модерации.
//
// @returns:
//   - result1: HTTP-обработчик для регистрации этой операции.
func (h *Handler) Change(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if !validID(id) {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		var body struct {
			ClientID string `json:"clientId"`
		}
		if !httpresponse.BindJSON(c, &body, false) {
			return
		}
		if action != "decline" && !validID(body.ClientID) {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		item, err := h.Service.Change(c.Request.Context(), middleware.UserID(c), id, action, body.ClientID)
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		c.JSON(200, gin.H{"status": "success", "item": item})
	}
}
