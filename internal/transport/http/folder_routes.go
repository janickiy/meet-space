// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package httptransport

import (
	"github.com/gin-gonic/gin"
	foldersapp "github.com/janickiy/meet-space/internal/app/folders"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	"time"
)

// RegisterFolderRoutes регистрирует конечные точки для работы с папками с предоставленными
// промежуточными модулями аутентификации и обработки запросов.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - h: Адаптер обработчика HTTP для регистрации или привязки к клиенту.
//   - auth: Промежуточное ПО для аутентификации или обработчик учётных записей.
//   - limiter: Общий ограничитель частоты запросов.
func RegisterFolderRoutes(router gin.IRouter, h *foldersapp.Handler, auth gin.HandlerFunc, limiter httpmiddleware.Limiter) {
	rules := []httpmiddleware.Rule{}
	for _, route := range []struct {
		Method, Path, Scope string
		Limit               int
	}{
		{"GET", "/folders", "folder_list", 120}, {"POST", "/folders", "folder_create", 30},
		{"PUT", "/folders/order", "folder_order", 30}, {"GET", "/folders/:id", "folder_get", 120},
		{"PATCH", "/folders/:id", "folder_rename", 30}, {"DELETE", "/folders/:id", "folder_delete", 30},
		{"GET", "/folders/:id/items", "folder_items", 120}, {"GET", "/folder-items", "folder_candidates", 120},
		{"PUT", "/folders/:id/items/:kind/:itemId", "folder_add", 60}, {"DELETE", "/folders/:id/items/:kind/:itemId", "folder_remove", 60},
	} {
		rules = append(rules, httpmiddleware.Rule{Method: route.Method, Path: APIV1Prefix + route.Path, Scope: route.Scope, Limit: route.Limit, Window: time.Minute, Key: httpmiddleware.UserID})
	}
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	g := router.Group(APIV1Prefix, private, auth, h.AccountOnly, httpmiddleware.RateLimit(limiter, httpmiddleware.RateLimitConfig{Enabled: limiter != nil, Rules: rules}))
	g.GET("/folders", h.List)
	g.POST("/folders", h.Create)
	g.PUT("/folders/order", h.Order)
	g.GET("/folders/:id", h.Get)
	g.PATCH("/folders/:id", h.Rename)
	g.DELETE("/folders/:id", h.Delete)
	g.GET("/folders/:id/items", h.Items)
	g.GET("/folder-items", h.Candidates)
	g.PUT("/folders/:id/items/:kind/:itemId", h.AddItem)
	g.DELETE("/folders/:id/items/:kind/:itemId", h.RemoveItem)
}
