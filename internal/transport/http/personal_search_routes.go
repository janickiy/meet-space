package httptransport

import (
	"time"

	"github.com/gin-gonic/gin"
	chatapp "github.com/janickiy/meet-space/internal/app/chat"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// RegisterPersonalSearchRoutes регистрирует персональные конечные точки доступа для поиска с
// аутентификацией.
//
// Для регистрации аккаунтов и текущего членства в чатах требуется отдельная процедура.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - handler: Обратный вызов, отвечающий за обработку операции с ограниченной областью
//     видимости.
//   - auth: Промежуточное ПО для аутентификации или обработчик учётных записей.
//   - accountOnly: Промежуточное ПО, отклоняющее гостевые идентификаторы для операций,
//     доступных только для зарегистрированных учётных записей.
//   - limiter: Общий ограничитель частоты запросов.
func RegisterPersonalSearchRoutes(router gin.IRouter, handler *chatapp.SearchHandler, auth, accountOnly gin.HandlerFunc, limiter httpmiddleware.Limiter) {
	base := APIV1Prefix + "/conversations/:id/messages"
	rules := []httpmiddleware.Rule{}
	for _, suffix := range []string{"/search", "/:messageId/context"} {
		rules = append(rules, httpmiddleware.Rule{Method: "GET", Path: base + suffix, Scope: "conversation_message_search", Limit: 60, Window: time.Minute, Key: httpmiddleware.UserID})
	}
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	routes := router.Group(base, private, auth, accountOnly, httpmiddleware.RateLimit(limiter, httpmiddleware.RateLimitConfig{Enabled: limiter != nil, Rules: rules}))
	routes.GET("/search", handler.Search)
	routes.GET("/:messageId/context", handler.Context)
}
