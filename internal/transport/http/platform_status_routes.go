package httptransport

import (
	"github.com/gin-gonic/gin"
	platformapp "github.com/janickiy/meet-space/internal/app/platform"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// RegisterPlatformStatusRoutes registers the platform status endpoints with the supplied
// authentication and request middleware.
//
// Parameters:
//   - router: HTTP router on which the component endpoints are registered.
//   - handler: Callback responsible for processing the scoped operation.
//   - auth: Authentication middleware or account handler dependency.
//   - checker: Persisted administrator-access checker.
func RegisterPlatformStatusRoutes(router gin.IRouter, handler *platformapp.Handler, auth gin.HandlerFunc, checker middleware.AdminChecker) {
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	router.GET(APIV1Prefix+"/capabilities", private, auth, handler.Capabilities)
	admin := router.Group(APIV1Prefix+"/admin", private, auth, middleware.RequireAdmin(checker))
	admin.GET("/summary", handler.Summary)
}
