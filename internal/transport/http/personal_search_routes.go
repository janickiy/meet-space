package httptransport

import (
	"time"

	"github.com/gin-gonic/gin"
	chatapp "github.com/janickiy/meet-space/internal/app/chat"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// RegisterPersonalSearchRoutes registers the authenticated personal search endpoints.
//
// Registered accounts and current conversation membership are required independently.
//
// Parameters:
//   - router: HTTP router on which the component endpoints are registered.
//   - handler: Callback responsible for processing the scoped operation.
//   - auth: Authentication middleware or account handler dependency.
//   - accountOnly: Middleware rejecting guest identities for registered-account-only operations.
//   - limiter: Shared request-rate limiter.
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
