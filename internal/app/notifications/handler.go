// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package notificationsapp

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	usecase "github.com/janickiy/meet-space/internal/usecase/notifications"
)

// Handler обслуживает запросы на уведомления и делегирует право владения SSE-подключением.
type Handler struct {
	service *usecase.Service
	events  *eventStreams
}

// NewHandler сохраняет HTTP API, предоставляя SSE собственного владельца жизненного цикла.
//
// @Parameters:
//   - service: служба приложения, вызываемая этим адаптером.
//   - bus: транспортный протокол для публикации и подписки с ограниченной областью видимости.
//   - tokens: эмитент или верификатор токенов, используемый для проверки подлинности.
//   - limiter: общий ограничитель частоты запросов.
//   - namespace: пространство имен, изолирующее распределенные ключи и каналы этого компонента.
//
// @returns:
//   - result1: созданный или загруженный обработчик.
func NewHandler(service *usecase.Service, bus Subscriber, tokens Verifier, limiter Limiter, namespace string) *Handler {
	return &Handler{service: service, events: &eventStreams{
		service: service, bus: bus, tokens: tokens, limiter: limiter,
		namespace: namespace, counts: syncCounts{values: map[string]int{}},
		drain: make(chan struct{}),
	}}
}

// BeginDrain освобождает запросы SSE, чтобы можно было завершить корректное развертывание API.
func (h *Handler) BeginDrain() {
	if h.events != nil {
		h.events.beginDrain()
	}
}

// List возвращает ограниченную страницу записей о личных уведомлениях с использованием заданных параметров разбивки на страницы и фильтров.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) List(c *gin.Context) {
	limit := 30
	if raw := c.Query("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		limit = v
	}
	page, err := h.service.List(c.Request.Context(), httpmiddleware.UserID(c), c.Query("cursor"), limit)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, page)
}

// Read помечается как прочитанное и возвращается в сохраненное состояние.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Read(c *gin.Context) {
	id, err := uuid.Parse(c.Param("notificationId"))
	if err != nil || id == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	if !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	item, err := h.service.Read(c.Request.Context(), httpmiddleware.UserID(c), id.String())
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"item": item})
}

// Events открывает авторизованный поток уведомлений, принадлежащий компоненту SSE.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Events(c *gin.Context) { h.events.serve(c) }
