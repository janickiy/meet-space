package httptransport

import (
	"github.com/gin-gonic/gin"
	authapp "github.com/janickiy/meet-space/internal/app/auth"
	conferencesapp "github.com/janickiy/meet-space/internal/app/conferences"
	"github.com/janickiy/meet-space/internal/domain/conferences"
)

// RegisterPlatformRoutes registers the platform endpoints with the supplied authentication and
// request middleware.
//
// Parameters:
//   - router: HTTP router on which the component endpoints are registered.
//   - auth: Authentication middleware or account handler dependency.
//   - conference: Conference HTTP handler whose endpoints are being registered.
//   - authentication: Middleware that resolves the authenticated account identity.
func RegisterPlatformRoutes(router gin.IRouter, auth *authapp.Handler, conference *conferencesapp.Handler, authentication gin.HandlerFunc) {
	public := router.Group(APIV1Prefix + "/auth")
	public.POST("/register", auth.Register)
	public.POST("/login", auth.Login)
	public.POST("/refresh", auth.Refresh)
	public.POST("/accounts/list", auth.Accounts)
	public.POST("/accounts/switch", auth.SwitchAccount)
	if auth.PersistentSessionsEnabled() {
		public.POST("/logout", auth.Logout)
	}
	protected := public.Group("", authentication)
	protected.GET("/me", auth.Me)
	protected.PATCH("/me", auth.UpdateProfile)
	protected.DELETE("/me", auth.DeleteAccount)
	protected.POST("/session", auth.Bootstrap)
	if !auth.PersistentSessionsEnabled() {
		protected.POST("/logout", auth.Logout)
	}

	conferenceRoutes := router.Group(APIV1Prefix+"/conferences", authentication)
	conferenceRoutes.POST("", conference.Create)
	conferenceRoutes.GET("", conference.List)
	conferenceRoutes.GET("/:id", conference.Read)
	conferenceRoutes.POST("/:id/start", conference.Transition(conferences.Active))
	conferenceRoutes.POST("/:id/finish", conference.Transition(conferences.Finished))
	conferenceRoutes.POST("/:id/cancel", conference.Transition(conferences.Cancelled))
	conferenceRoutes.POST("/:id/join", conference.Join)
	conferenceRoutes.POST("/:id/leave", conference.Leave)
	conferenceRoutes.GET("/:id/participants", conference.Participants)
	conferenceRoutes.GET("/:id/participants/me", conference.Self)
	conferenceRoutes.POST("/:id/participants/:participantId/admission", conference.Admission)
	conferenceRoutes.PUT("/:id/schedule", conference.Schedule)
	conferenceRoutes.GET("/:id/history", conference.History)
	router.GET(APIV1Prefix+"/me/conferences", authentication, conference.Timeline)

	// A separate prefix avoids conflicting :id and :inviteCode route templates and exposes only a
	// limited invitation view.
	invites := router.Group(APIV1Prefix + "/conference-invites")
	invites.GET("/:code", conference.LookupInvite)
	invites.POST("/:code/join", authentication, conference.JoinInvite)
}

// RegisterControlRoutes registers the control endpoints with the supplied authentication and
// request middleware.
//
// Parameters:
//   - router: HTTP router on which the component endpoints are registered.
//   - handler: Callback responsible for processing the scoped operation.
//   - authentication: Middleware that resolves the authenticated account identity.
func RegisterControlRoutes(router gin.IRouter, handler *conferencesapp.ControlHandler, authentication gin.HandlerFunc) {
	routes := router.Group(APIV1Prefix+"/conferences", authentication)
	routes.PUT("/:id/participants/me/media", handler.Media)
	routes.POST("/:id/participants/:participantId/moderation", handler.Moderate)
}
