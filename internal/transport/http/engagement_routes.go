package httptransport

import (
	"github.com/gin-gonic/gin"
	engagementapp "github.com/janickiy/meet-space/internal/app/engagement"
)

// RegisterEngagementRoutes registers the engagement endpoints with the supplied authentication
// and request middleware.
//
// Parameters:
//   - router: HTTP router on which the component endpoints are registered.
//   - handler: Callback responsible for processing the scoped operation.
//   - auth: Authentication middleware or account handler dependency.
func RegisterEngagementRoutes(router gin.IRouter, handler *engagementapp.Handler, auth gin.HandlerFunc) {
	group := router.Group(APIV1Prefix+"/conferences/:id", auth,

		func(c *gin.Context) {
			c.Header("Cache-Control", "private, no-store")
			c.Header("X-Content-Type-Options", "nosniff")
			c.Next()
		})
	group.POST("/reactions", handler.Reaction)
}
