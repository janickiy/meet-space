package httptransport

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/telemetry"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// RegisterClientErrorRoutes регистрирует конечные точки обработки ошибок клиента с
// предоставленными модулями аутентификации и промежуточным ПО для обработки запросов.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - handler: Обратный вызов, отвечающий за обработку операции с ограниченной областью
//     видимости.
//   - limiter: Общий ограничитель частоты запросов.
func RegisterClientErrorRoutes(router gin.IRouter, handler *telemetry.Handler, limiter middleware.Limiter) {
	path := APIV1Prefix + "/client-errors"
	router.POST(path, middleware.RateLimit(limiter, middleware.RateLimitConfig{
		Enabled: true,
		Rules: []middleware.Rule{
			{Method: "POST", Path: path, Scope: "client-errors-ip", Limit: 30, Window: time.Minute, Key: middleware.ClientIPKey},
			{Method: "POST", Path: path, Scope: "client-errors-global", Limit: 300, Window: time.Minute, Key: func(*gin.Context) string { return "all" }},
		},
	}), handler.Receive)
}
