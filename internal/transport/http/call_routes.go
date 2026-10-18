package httptransport

import (
	"github.com/gin-gonic/gin"
	app "github.com/janickiy/meet-space/internal/app/calls"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	"time"
)

// RegisterCallRoutes регистрирует конечные точки вызовов с предоставленными средствами
// аутентификации и промежуточным ПО для обработки запросов.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - h: Адаптер обработчика HTTP для регистрации или привязки к клиенту.
//   - auth: Промежуточное ПО для аутентификации или обработчик учётных записей.
//   - accountOnly: Промежуточное ПО, отклоняющее гостевые идентификаторы для операций,
//     доступных только для зарегистрированных учётных записей.
//   - limiter: Общий ограничитель частоты запросов.
func RegisterCallRoutes(router gin.IRouter, h *app.Handler, auth, accountOnly gin.HandlerFunc, limiter middleware.Limiter) {
	rules := []middleware.Rule{
		{Method: "POST", Path: APIV1Prefix + "/calls", Scope: "call_create", Limit: 15, Window: time.Minute, Key: middleware.UserID},
		{Method: "GET", Path: APIV1Prefix + "/calls/active", Scope: "call_active", Limit: 90, Window: time.Minute, Key: middleware.UserID},
		{Method: "GET", Path: APIV1Prefix + "/calls/:id", Scope: "call_get", Limit: 90, Window: time.Minute, Key: middleware.UserID},
	}
	for _, action := range []string{"accept", "decline", "cancel", "end", "heartbeat"} {
		rules = append(rules, middleware.Rule{Method: "POST", Path: APIV1Prefix + "/calls/:id/" + action, Scope: "call_" + action, Limit: 60, Window: time.Minute, Key: middleware.UserID})
	}
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	g := router.Group(APIV1Prefix+"/calls", private, auth, accountOnly, middleware.RateLimit(limiter, middleware.RateLimitConfig{Enabled: limiter != nil, Rules: rules}))
	g.POST("", h.Create)
	g.GET("/active", h.Active)
	g.GET("/:id", h.Get)
	for _, action := range []string{"accept", "decline", "cancel", "end", "heartbeat"} {
		g.POST("/:id/"+action, h.Change(action))
	}
}
