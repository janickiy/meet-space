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
	platformapp "github.com/janickiy/meet-space/internal/app/platform"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// RegisterPlatformStatusRoutes регистрирует платформенные конечные точки доступа для получения
// статуса с использованием предоставленного промежуточного программного обеспечения для
// аутентификации и обработки запросов.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - handler: Обратный вызов, отвечающий за обработку операции с ограниченной областью
//     видимости.
//   - auth: Промежуточное ПО для аутентификации или обработчик учётных записей.
//   - checker: Проверка сохранённого доступа администратора.
func RegisterPlatformStatusRoutes(router gin.IRouter, handler *platformapp.Handler, auth gin.HandlerFunc, checker middleware.AdminChecker) {
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	router.GET(APIV1Prefix+"/capabilities", private, auth, handler.Capabilities)
	admin := router.Group(APIV1Prefix+"/admin", private, auth, middleware.RequireAdmin(checker))
	admin.GET("/summary", handler.Summary)
}
