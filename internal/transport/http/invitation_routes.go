package httptransport

import (
	"time"

	"github.com/gin-gonic/gin"
	app "github.com/janickiy/meet-space/internal/app/conferences"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// RegisterInvitationRoutes регистрирует конечные точки приглашения с помощью предоставленного
// промежуточного программного обеспечения для аутентификации и обработки запросов.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - handler: Обратный вызов, отвечающий за обработку операции с ограниченной областью
//     видимости.
//   - auth: Промежуточное ПО для аутентификации или обработчик учётных записей.
//   - limiter: Общий ограничитель частоты запросов.
func RegisterInvitationRoutes(router gin.IRouter, handler *app.InvitationHandler, auth gin.HandlerFunc, limiter middleware.Limiter) {
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	key := func(c *gin.Context) string { return middleware.UserID(c) }
	limited := middleware.RateLimit(limiter, middleware.RateLimitConfig{Enabled: limiter != nil, Rules: []middleware.Rule{
		{Method: "POST", Path: APIV1Prefix + "/conferences/:id/invitations", Scope: "meeting_invitation_send", Limit: 10, Window: time.Minute, Key: key},
		{Method: "GET", Path: APIV1Prefix + "/conferences/:id/invitation-users", Scope: "meeting_invitation_search", Limit: 60, Window: time.Minute, Key: key},
	}})
	group := router.Group(APIV1Prefix+"/conferences", private, auth, limited)
	group.GET("/:id/invitation-users", handler.Search)
	group.POST("/:id/invitations", handler.Invite)
}
