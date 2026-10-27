package httptransport

import (
	"time"

	"github.com/gin-gonic/gin"
	profileapp "github.com/janickiy/meet-space/internal/app/profile"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// RegisterProfileAvatarRoutes requires an account, bounded request rates and
// private responses for profile photos. auth and accountOnly validate the actor.
func RegisterProfileAvatarRoutes(router gin.IRouter, h *profileapp.AvatarHandler, auth, accountOnly gin.HandlerFunc, limiter middleware.Limiter) {
	rules := []middleware.Rule{
		{Method: "PUT", Path: APIV1Prefix + "/auth/me/avatar", Scope: "profile_avatar_put", Limit: 10, Window: time.Minute, Key: middleware.UserID},
		{Method: "DELETE", Path: APIV1Prefix + "/auth/me/avatar", Scope: "profile_avatar_delete", Limit: 30, Window: time.Minute, Key: middleware.UserID},
		{Method: "GET", Path: APIV1Prefix + "/users/:id/avatar/content", Scope: "profile_avatar_read", Limit: 120, Window: time.Minute, Key: middleware.UserID},
	}
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	g := router.Group(APIV1Prefix, private, auth, accountOnly, middleware.RateLimit(limiter, middleware.RateLimitConfig{Enabled: limiter != nil, Rules: rules}))
	g.PUT("/auth/me/avatar", h.Put)
	g.DELETE("/auth/me/avatar", h.Delete)
	g.GET("/users/:id/avatar/content", h.Content)
}
