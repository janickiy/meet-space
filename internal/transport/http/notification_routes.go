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
	notificationsapp "github.com/janickiy/meet-space/internal/app/notifications"
)

// RegisterNotificationRoutes регистрирует конечные точки уведомлений с предоставленными
// средствами аутентификации и промежуточным программным обеспечением для обработки запросов.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - handler: Обратный вызов, отвечающий за обработку операции с ограниченной областью
//     видимости.
//   - auth: Промежуточное ПО для аутентификации или обработчик учётных записей.
func RegisterNotificationRoutes(router gin.IRouter, handler *notificationsapp.Handler, auth gin.HandlerFunc) {
	group := router.Group(APIV1Prefix+"/notifications", auth)
	group.GET("", handler.List)
	group.GET("/events", handler.Events)
	group.POST("/:notificationId/read", handler.Read)
}
