// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package conferencesapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	d "github.com/janickiy/meet-space/internal/domain/conferences"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

type InvitationService interface {
	// Search Поиск по жизненному циклу конференции и членству участников с использованием ограниченных входных данных и фильтров видимости вызывающей стороны.
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем выполнения операции.
	//   - arg2: идентификатор аккаунта, выполняющего операцию.
	//   - arg3: идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: текст поиска или проверенные фильтры запроса.
	//
	// @returns:
	//   - result1: собранные значения пользователей приглашения.
	//   - result2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, передачи или сохранения.  //
	Search(context.Context, string, string, string) ([]d.InvitationUser, error)
	// Invite создает или обновляет приглашения на конференцию для проверенных получателей.
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем выполнения операции.
	//   - arg2: идентификатор учетной записи, выполняющей операцию.
	//   - arg3: идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: проверенный запрос приложения или входящие параметры операции.
	//
	// @returns:
	//   - result1: собранные значения результатов приглашения.
	//   - result2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, передачи или сохранения данных.
	//
	Invite(context.Context, string, string, d.InvitationRequest) ([]d.InvitationResult, error)
}
type InvitationHandler struct{ Service InvitationService }

// Search по жизненному циклу конференции и членству участников с использованием ограниченных входных данных и фильтров видимости вызывающей стороны.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *InvitationHandler) Search(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	items, err := h.Service.Search(c.Request.Context(), middleware.UserID(c), id, c.Query("query"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items})
}

// Invite создает или обновляет приглашения на конференцию для проверенных получателей.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *InvitationHandler) Invite(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	var request d.InvitationRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	items, err := h.Service.Invite(c.Request.Context(), middleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "success", "items": items})
}
