package httptransport

import (
	"github.com/gin-gonic/gin"
	conferencesapp "github.com/janickiy/meet-space/internal/app/conferences"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	"time"
)

// RegisterGuestRoutes регистрирует конечные точки для работы с гостевыми аккаунтами с
// предоставленными промежуточными модулями аутентификации и обработки запросов.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - handler: Обратный вызов, отвечающий за обработку операции с ограниченной областью
//     видимости.
//   - limiter: Общий ограничитель частоты запросов.
func RegisterGuestRoutes(router gin.IRouter, handler *conferencesapp.GuestHandler, limiter httpmiddleware.Limiter) {
	path := APIV1Prefix + "/conference-invites/:code/guest"
	limit := httpmiddleware.RateLimit(limiter, httpmiddleware.RateLimitConfig{Enabled: limiter != nil, Rules: []httpmiddleware.Rule{
		{Method: "POST", Path: path, Scope: "guest_join_ip", Limit: 10, Window: time.Minute, Key: httpmiddleware.ClientIPKey},
	}})
	router.POST(path, limit, handler.Join)
}
