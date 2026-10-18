package httptransport

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	chatapp "github.com/janickiy/meet-space/internal/app/chat"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// RegisterChatRoutes регистрирует конечные точки чата с предоставленными промежуточными
// модулями аутентификации и обработки запросов.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - handler: Обратный вызов, отвечающий за обработку операции с ограниченной областью
//     видимости.
//   - authentication: Промежуточный модуль, который определяет подлинность аккаунта, прошедшего
//     аутентификацию.
//   - limiter: Общий ограничитель частоты запросов.
func RegisterChatRoutes(router gin.IRouter, handler *chatapp.Handler, authentication gin.HandlerFunc, limiter httpmiddleware.Limiter) {
	registerMessageRoutes(router, handler, authentication, limiter, "conferences")
}

// registerMessageRoutes регистрирует маршруты для сообщений, вложений и состояния чтения для
// выбранной области диалога.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - handler: Обратный вызов, отвечающий за обработку операции с ограниченной областью
//     видимости.
//   - authentication: Промежуточный модуль, который определяет подлинность аккаунта, прошедшего
//     аутентификацию.
//   - limiter: Общий ограничитель частоты запросов.
//   - namespace: Пространство имен, изолирующее распределенные ключи и каналы этого компонента.
//   - checks: Именованные проверки готовности зависимостей или маршрутные проверки.
func registerMessageRoutes(router gin.IRouter, handler *chatapp.Handler, authentication gin.HandlerFunc, limiter httpmiddleware.Limiter, namespace string, checks ...gin.HandlerFunc) {

	key := func(c *gin.Context) string {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil || id == uuid.Nil {
			return ""
		}
		return httpmiddleware.UserID(c) + ":" + id.String()
	}
	path := APIV1Prefix + "/" + namespace + "/:id"
	rules := []httpmiddleware.Rule{}
	for _, rule := range []struct {
		method, suffix, scope string
		limit                 int
	}{
		{"GET", "/messages", "chat_read", 120}, {"POST", "/messages", "chat_send", 60}, {"PATCH", "/messages/:messageId", "chat_edit", 30}, {"DELETE", "/messages/:messageId", "chat_delete", 30},
		{"GET", "/chat/read", "chat_read_state", 120}, {"PUT", "/chat/read", "chat_mark_read", 30},
		{"POST", "/attachments/init", "chat_upload_init", 10}, {"PUT", "/attachments/:attachmentId/content", "chat_upload", 10}, {"POST", "/attachments/:attachmentId/finalize", "chat_upload_finalize", 20}, {"GET", "/attachments/:attachmentId/download", "chat_download", 60},
	} {
		rules = append(rules, httpmiddleware.Rule{Method: rule.method, Path: path + rule.suffix, Scope: namespace + ":" + rule.scope, Limit: rule.limit, Window: time.Minute, Key: key})
	}

	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	handlers := []gin.HandlerFunc{private, authentication}
	handlers = append(handlers, checks...)
	handlers = append(handlers, httpmiddleware.RateLimit(limiter, httpmiddleware.RateLimitConfig{Enabled: limiter != nil, Rules: rules}))
	routes := router.Group(APIV1Prefix+"/"+namespace, handlers...)
	routes.GET("/:id/messages", handler.List)
	routes.POST("/:id/messages", handler.Send)
	routes.PATCH("/:id/messages/:messageId", handler.Edit)
	routes.DELETE("/:id/messages/:messageId", handler.Delete)
	routes.GET("/:id/chat/read", handler.ReadState)
	routes.PUT("/:id/chat/read", handler.MarkRead)
	routes.POST("/:id/attachments/init", handler.InitAttachment)
	routes.PUT("/:id/attachments/:attachmentId/content", handler.Upload)
	routes.POST("/:id/attachments/:attachmentId/finalize", handler.FinalizeAttachment)
	routes.GET("/:id/attachments/:attachmentId/download", handler.Download)
}
