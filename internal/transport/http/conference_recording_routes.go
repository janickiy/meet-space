package httptransport

import (
	"github.com/gin-gonic/gin"
	recordingsapp "github.com/janickiy/meet-space/internal/app/recordings"
)

// RegisterConferenceRecordingRoutes registers the conference recording endpoints with the
// supplied authentication and request middleware.
//
// Parameters:
//   - router: HTTP router on which the component endpoints are registered.
//   - handler: Callback responsible for processing the scoped operation.
//   - authentication: Middleware that resolves the authenticated account identity.
func RegisterConferenceRecordingRoutes(router gin.IRouter, handler *recordingsapp.Handler, authentication gin.HandlerFunc) {
	routes := router.Group(APIV1Prefix+"/conferences", authentication)
	routes.POST("/:id/recordings", handler.Start)
	routes.GET("/:id/recordings", handler.List)
	routes.GET("/:id/recordings/:recordingId", handler.Read)
	routes.DELETE("/:id/recordings/:recordingId", handler.Delete)
	routes.POST("/:id/recordings/:recordingId/stop", handler.Stop)
}
