package httptransport

import (
	"github.com/gin-gonic/gin"
	notificationsapp "github.com/janickiy/meet-space/internal/app/notifications"
)

// RegisterNotificationRoutes registers the notification endpoints with the supplied
// authentication and request middleware.
//
// Parameters:
//   - router: HTTP router on which the component endpoints are registered.
//   - handler: Callback responsible for processing the scoped operation.
//   - auth: Authentication middleware or account handler dependency.
func RegisterNotificationRoutes(router gin.IRouter, handler *notificationsapp.Handler, auth gin.HandlerFunc) {
	group := router.Group(APIV1Prefix+"/notifications", auth)
	group.GET("", handler.List)
	group.GET("/events", handler.Events)
	group.POST("/:notificationId/read", handler.Read)
}
