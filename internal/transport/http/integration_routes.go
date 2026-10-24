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
	app "github.com/janickiy/meet-space/internal/app/integrations"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	"time"
)

// RegisterIntegrationRoutes регистрирует конечные точки для интеграции с предоставленными
// промежуточными модулями аутентификации и обработки запросов.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - handler: Обратный вызов, отвечающий за обработку операции с ограниченной областью
//     видимости.
//   - auth: Промежуточное ПО для аутентификации или обработчик учётных записей.
//   - limiters: Общие реализации ограничителей для зарегистрированных конечных точек.
func RegisterIntegrationRoutes(router gin.IRouter, handler *app.Handler, auth gin.HandlerFunc, limiters ...middleware.Limiter) {
	var limiter middleware.Limiter
	if len(limiters) > 0 {
		limiter = limiters[0]
	}
	// Отключить кэширование для всех ответов интеграции, включая сообщения об ошибках. c —
	// текущий контекст запроса и ответа промежуточного программного обеспечения.
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	// Формировать ключ сегмента ограничения частоты только на основе подтверждённой личности
	// пользователя с токеном Bearer, а не на основе полей, предоставленных пользователем. c —
	// аутентифицированный запрос; результат — идентификатор владельца сегмента.
	key := func(c *gin.Context) string { return middleware.UserID(c) }
	rules := []middleware.Rule{}
	for _, rule := range []struct {
		method, path, scope string
		limit               int
	}{
		{"GET", "/integrations/calendars/:provider/connect", "calendar_oauth_connect", 5},
		{"POST", "/integrations/calendars/:provider/callback", "calendar_oauth_callback", 10},
		{"POST", "/notifications/devices", "push_device_register", 10},
		{"POST", "/integrations/calendars/mock", "calendar_mock_connect", 5},
		{"PUT", "/notifications/preferences", "notification_preferences", 30},
		{"DELETE", "/integrations/calendars/:provider", "calendar_disconnect", 10},
		{"DELETE", "/notifications/devices/:deviceId", "push_device_revoke", 30},
	} {
		rules = append(rules, middleware.Rule{Method: rule.method, Path: APIV1Prefix + rule.path, Scope: rule.scope, Limit: rule.limit, Window: time.Minute, Key: key})
	}
	limited := middleware.RateLimit(limiter, middleware.RateLimitConfig{Enabled: limiter != nil, Rules: rules})
	notifications := router.Group(APIV1Prefix+"/notifications", private, auth, limited)
	notifications.GET("/preferences", handler.Preferences)
	notifications.PUT("/preferences", handler.SavePreferences)
	notifications.GET("/devices", handler.Devices)
	notifications.POST("/devices", handler.RegisterDevice)
	notifications.DELETE("/devices/:deviceId", handler.RevokeDevice)
	integration := router.Group(APIV1Prefix+"/integrations", private, auth, limited)
	integration.GET("/capabilities", handler.Capabilities)
	integration.GET("/calendars", handler.Calendars)
	integration.POST("/calendars/mock", handler.MockConnect)
	integration.GET("/calendars/:provider/connect", handler.Connect)
	integration.POST("/calendars/:provider/callback", handler.Callback)
	integration.DELETE("/calendars/:provider", handler.Disconnect)
	// Gin использует одно имя шаблона маршрута для подключения к провайдеру, обратного вызова и
	// удаления UUID; обработчик отдельно проверяет UUIDs.
	router.GET(APIV1Prefix+"/conferences/:id/calendar", private, auth, handler.CalendarMappings)
}
