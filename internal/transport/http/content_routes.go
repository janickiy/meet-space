package httptransport

import (
	"github.com/gin-gonic/gin"
	contentapp "github.com/janickiy/meet-space/internal/app/content"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	"time"
)

// RegisterContentRoutes регистрирует конечные точки для контента с предоставленными
// промежуточными модулями аутентификации и обработки запросов.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - handler: Обратный вызов, отвечающий за обработку операции с ограниченной областью
//     видимости.
//   - authentication: Промежуточный модуль, который определяет подлинность аккаунта, прошедшего
//     аутентификацию.
//   - limiters: Общие реализации ограничителей для зарегистрированных конечных точек.
func RegisterContentRoutes(router gin.IRouter, handler *contentapp.Handler, authentication gin.HandlerFunc, limiters ...middleware.Limiter) {
	var limiter middleware.Limiter
	if len(limiters) > 0 {
		limiter = limiters[0]
	}
	searchLimit := middleware.RateLimit(limiter, middleware.RateLimitConfig{Enabled: limiter != nil, Rules: []middleware.Rule{{Method: "GET", Path: APIV1Prefix + "/search", Scope: "content_search", Limit: 30, Window: time.Minute, Key: func(c *gin.Context) string { return middleware.UserID(c) }}}})
	// Отключить кэширование приватного текста в браузере и прокси-сервере после выхода из
	// системы или отзыва доступа.  c — текущий контекст запроса и ответа HTTP.
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("Pragma", "no-cache")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	routes := router.Group(APIV1Prefix, private, authentication)
	base := "/conferences/:id/recordings/:recordingId"
	routes.GET(base+"/transcript", handler.Transcript)
	routes.GET(base+"/transcript/segments", handler.Segments)
	routes.POST(base+"/transcript/retry", handler.RetryTranscript)
	routes.GET(base+"/summary", handler.Summary)
	routes.POST(base+"/summary/regenerate", handler.RegenerateSummary)
	routes.GET("/search", searchLimit, handler.Search)
}
