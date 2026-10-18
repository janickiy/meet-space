package httptransport

import (
	"github.com/gin-gonic/gin"
	chatapp "github.com/janickiy/meet-space/internal/app/chat"
	personalapp "github.com/janickiy/meet-space/internal/app/personal"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	"time"
)

// RegisterPersonalRoutes регистрирует персональные конечные точки доступа с использованием
// предоставленного промежуточного программного обеспечения для аутентификации и обработки
// запросов.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - h: Адаптер обработчика HTTP для регистрации или привязки к клиенту.
//   - chat: обработчик чата HTTP зарегистрирован для персональных чатов.
//   - auth: Промежуточное ПО для аутентификации или обработчик учётных записей.
//   - limiter: Общий ограничитель частоты запросов.
func RegisterPersonalRoutes(router gin.IRouter, h *personalapp.Handler, chat *chatapp.Handler, auth gin.HandlerFunc, limiter httpmiddleware.Limiter) {
	rules := []httpmiddleware.Rule{}
	for _, v := range []struct {
		Method, Path, Scope string
		Limit               int
	}{{"GET", "/users", "personal_search", 30}, {"GET", "/conversations", "personal_list", 120}, {"POST", "/conversations/direct", "personal_create", 30}, {"GET", "/conversations/:id", "personal_detail", 120}, {"GET", "/conversations/:id/peer-presence", "personal_peer_presence", 120}, {"POST", "/conversations/:id/read", "personal_read", 30},
		{"POST", "/conversations/group", "group_create", 20}, {"PATCH", "/conversations/:id", "group_metadata", 30}, {"DELETE", "/conversations/:id", "group_delete", 20},
		{"GET", "/conversations/:id/members", "group_members", 120}, {"POST", "/conversations/:id/members", "group_add", 30},
		{"DELETE", "/conversations/:id/members/:userId", "group_remove", 30}, {"PATCH", "/conversations/:id/members/:userId", "group_role", 30},
		{"POST", "/conversations/:id/leave", "group_leave", 30}, {"POST", "/conversations/:id/ownership", "group_ownership", 20},
		{"PATCH", "/conversations/:id/preferences", "direct_preferences", 30}, {"POST", "/conversations/:id/clear-history", "direct_clear", 20}, {"POST", "/conversations/:id/hide", "direct_hide", 20},
		{"GET", "/conversations/:id/pins", "personal_pins", 60}, {"PUT", "/conversations/:id/pins/:messageId", "personal_pin_update", 30}, {"DELETE", "/conversations/:id/pins/:messageId", "personal_pin_update", 30}} {
		rules = append(rules, httpmiddleware.Rule{Method: v.Method, Path: APIV1Prefix + v.Path, Scope: v.Scope, Limit: v.Limit, Window: time.Minute, Key: httpmiddleware.UserID})
	}
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	g := router.Group(APIV1Prefix, private, auth, h.AccountOnly, httpmiddleware.RateLimit(limiter, httpmiddleware.RateLimitConfig{Enabled: limiter != nil, Rules: rules}))
	g.GET("/users", h.Search)
	g.GET("/conversations", h.List)
	g.POST("/conversations/direct", h.Create)
	g.POST("/conversations/group", h.CreateGroup)
	g.GET("/conversations/:id", h.Get)
	g.GET("/conversations/:id/peer-presence", h.PeerPresence)
	g.PATCH("/conversations/:id/preferences", h.SetConversationPreferences)
	g.POST("/conversations/:id/clear-history", h.ClearDirectHistory)
	g.POST("/conversations/:id/hide", h.HideDirectConversation)
	g.GET("/conversations/:id/pins", h.Pins)
	g.PUT("/conversations/:id/pins/:messageId", h.SetPin(true))
	g.DELETE("/conversations/:id/pins/:messageId", h.SetPin(false))
	g.PATCH("/conversations/:id", h.UpdateGroup)
	g.DELETE("/conversations/:id", h.DeleteGroup)
	g.GET("/conversations/:id/members", h.GroupMembers)
	g.POST("/conversations/:id/members", h.AddGroupMembers)
	g.DELETE("/conversations/:id/members/:userId", h.RemoveGroupMember)
	g.PATCH("/conversations/:id/members/:userId", h.ChangeGroupRole)
	g.POST("/conversations/:id/leave", h.LeaveGroup)
	g.POST("/conversations/:id/ownership", h.TransferGroupOwnership)
	g.POST("/conversations/:id/read", chat.MarkRead)
	// AccountOnly также применяется ко всем общим маршрутам для сообщений и файлов.
	registerMessageRoutes(router, chat, auth, limiter, "conversations", h.AccountOnly)
}
