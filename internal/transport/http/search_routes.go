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
	searchapp "github.com/janickiy/meet-space/internal/app/search"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	"time"
)

// RegisterSearchRoutes preserves private, rate-limited conference-title search.
func RegisterSearchRoutes(router gin.IRouter, handler *searchapp.Handler, authentication gin.HandlerFunc, limiters ...middleware.Limiter) {
	var limiter middleware.Limiter
	if len(limiters) > 0 {
		limiter = limiters[0]
	}
	searchLimit := middleware.RateLimit(limiter, middleware.RateLimitConfig{Enabled: limiter != nil, Rules: []middleware.Rule{{Method: "GET", Path: APIV1Prefix + "/search", Scope: "conference_search", Limit: 30, Window: time.Minute, Key: func(c *gin.Context) string { return middleware.UserID(c) }}}})
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("Pragma", "no-cache")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	router.Group(APIV1Prefix, private, authentication).GET("/search", searchLimit, handler.Search)
}
