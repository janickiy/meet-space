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
	recordingsapp "github.com/janickiy/meet-space/internal/app/recordings"
)

// RegisterConferenceRecordingRoutes регистрирует конечные точки для записи конференций с
// предоставленными промежуточными модулями аутентификации и обработки запросов.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - handler: Обратный вызов, отвечающий за обработку операции с ограниченной областью
//     видимости.
//   - authentication: Промежуточный модуль, который определяет подлинность аккаунта, прошедшего
//     аутентификацию.
func RegisterConferenceRecordingRoutes(router gin.IRouter, handler *recordingsapp.Handler, authentication gin.HandlerFunc) {
	routes := router.Group(APIV1Prefix+"/conferences", authentication)
	routes.POST("/:id/recordings", handler.Start)
	routes.GET("/:id/recordings", handler.List)
	routes.GET("/:id/recordings/:recordingId", handler.Read)
	routes.DELETE("/:id/recordings/:recordingId", handler.Delete)
	routes.POST("/:id/recordings/:recordingId/stop", handler.Stop)
}
