// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package httptransport

import (
	"time"

	"github.com/gin-gonic/gin"
	personalapp "github.com/janickiy/meet-space/internal/app/personal"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// RegisterPersonalAssetRoutes регистрирует персональные конечные точки доступа к активам с
// использованием предоставленного промежуточного программного обеспечения для аутентификации и
// обработки запросов.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - h: Адаптер обработчика HTTP для регистрации или привязки к клиенту.
//   - auth: Промежуточное ПО для аутентификации или обработчик учётных записей.
//   - accountOnly: Промежуточное ПО, отклоняющее гостевые идентификаторы для операций,
//     доступных только для зарегистрированных учётных записей.
//   - limiter: Общий ограничитель частоты запросов.
func RegisterPersonalAssetRoutes(router gin.IRouter, h *personalapp.AssetHandler, auth, accountOnly gin.HandlerFunc, limiter middleware.Limiter) {
	rules := []middleware.Rule{}
	for _, rule := range []struct {
		method, path, scope string
		limit               int
	}{
		{"PUT", "/conversations/:id/avatar", "group_avatar_put", 10},
		{"DELETE", "/conversations/:id/avatar", "group_avatar_delete", 30},
		{"GET", "/conversations/:id/avatar/content", "group_avatar_content", 120},
		{"GET", "/conversations/:id/attachments/:attachmentId/content", "group_attachment_content", 60},
	} {
		rules = append(rules, middleware.Rule{Method: rule.method, Path: APIV1Prefix + rule.path, Scope: rule.scope, Limit: rule.limit, Window: time.Minute, Key: middleware.UserID})
	}
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	g := router.Group(APIV1Prefix, private, auth, accountOnly, middleware.RateLimit(limiter, middleware.RateLimitConfig{Enabled: limiter != nil, Rules: rules}))
	g.PUT("/conversations/:id/avatar", h.PutAvatar)
	g.DELETE("/conversations/:id/avatar", h.DeleteAvatar)
	g.GET("/conversations/:id/avatar/content", h.AvatarContent)
	g.GET("/conversations/:id/attachments/:attachmentId/content", h.AttachmentContent)
}
