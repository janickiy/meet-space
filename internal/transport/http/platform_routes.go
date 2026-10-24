// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package httptransport

import (
	"github.com/gin-gonic/gin"
	authapp "github.com/janickiy/meet-space/internal/app/auth"
	conferencesapp "github.com/janickiy/meet-space/internal/app/conferences"
	"github.com/janickiy/meet-space/internal/domain/conferences"
)

// RegisterPlatformRoutes регистрирует платформенные конечные точки доступа с использованием
// предоставленного промежуточного программного обеспечения для аутентификации и обработки
// запросов.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - auth: Промежуточное ПО для аутентификации или обработчик учётных записей.
//   - conference: обработчик конференции HTTP, конечные точки которого регистрируются.
//   - authentication: Промежуточный модуль, который определяет подлинность аккаунта, прошедшего
//     аутентификацию.
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

	// Отдельный префикс позволяет избежать конфликтов с шаблонами маршрутов :id и :inviteCode и
	// предоставляет доступ только к ограниченному представлению приглашений.
	invites := router.Group(APIV1Prefix + "/conference-invites")
	invites.GET("/:code", conference.LookupInvite)
	invites.POST("/:code/join", authentication, conference.JoinInvite)
}

// RegisterControlRoutes регистрирует управляющие конечные точки доступа с использованием
// предоставленного промежуточного программного обеспечения для аутентификации и обработки
// запросов.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - handler: Обратный вызов, отвечающий за обработку операции с ограниченной областью
//     видимости.
//   - authentication: Промежуточный модуль, который определяет подлинность аккаунта, прошедшего
//     аутентификацию.
func RegisterControlRoutes(router gin.IRouter, handler *conferencesapp.ControlHandler, authentication gin.HandlerFunc) {
	routes := router.Group(APIV1Prefix+"/conferences", authentication)
	routes.PUT("/:id/participants/me/media", handler.Media)
	routes.POST("/:id/participants/:participantId/moderation", handler.Moderate)
}
