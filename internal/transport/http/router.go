// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package httptransport

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/operations"
)

// APIV1Prefix — это основная версия префикса REST API.
const APIV1Prefix = "/api/v1"

// NewRouter создает маршрутизатор с зависимостями и ограничениями, необходимыми для HTTP.
//
// @Parameters:
//   - middleware: Дополнительное промежуточное ПО для обработки запросов, применяемое в порядке
//     регистрации.
//
// @returns:
//   - result1: Созданный или загруженный движок.
func NewRouter(middleware ...gin.HandlerFunc) *gin.Engine {
	router := gin.New()
	// Administrative routes are never registered on the public API listener.
	// Reject the namespace here too, regardless of cookies or forwarding headers.
	router.Use(func(c *gin.Context) {
		if strings.HasPrefix(strings.ToLower(c.Request.URL.Path), APIV1Prefix+"/admin") {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.Next()
	})
	router.Use(

		func(c *gin.Context) {
			defer func() {
				if recover() != nil {
					slog.Error("http request panic", "event_type", "server.panic", "method", c.Request.Method, "route", c.FullPath(), "request_id", operations.ID(c.Request.Context()))
					c.AbortWithStatusJSON(500, gin.H{"status": "failed", "message": "internal server error"})
				}
			}()
			c.Next()
		},

		func(c *gin.Context) {
			start := time.Now()
			c.Next()
			// Никогда не логируйте строки запроса: одноразовые билеты WebSocket — это тоже
			// учетные данные.
			if c.Writer.Status() >= 500 {
				slog.Error("http request failed", "event_type", "http.5xx", "route", c.FullPath(), "request_id", operations.ID(c.Request.Context()))
			}
			slog.Info("http request", "method", c.Request.Method, "route", c.FullPath(), "status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds(), "request_id", operations.ID(c.Request.Context()))
		})
	if len(middleware) > 0 {
		router.Use(middleware...)
	}
	router.GET("/health",

		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
	return router
}
