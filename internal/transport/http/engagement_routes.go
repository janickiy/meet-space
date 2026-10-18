package httptransport

import (
	"github.com/gin-gonic/gin"
	engagementapp "github.com/janickiy/meet-space/internal/app/engagement"
)

// RegisterEngagementRoutes регистрирует конечные точки для взаимодействия с предоставленными
// промежуточными модулями аутентификации и обработки запросов.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
//   - handler: Обратный вызов, отвечающий за обработку операции с ограниченной областью
//     видимости.
//   - auth: Промежуточное ПО для аутентификации или обработчик учётных записей.
func RegisterEngagementRoutes(router gin.IRouter, handler *engagementapp.Handler, auth gin.HandlerFunc) {
	group := router.Group(APIV1Prefix+"/conferences/:id", auth,

		func(c *gin.Context) {
			c.Header("Cache-Control", "private, no-store")
			c.Header("X-Content-Type-Options", "nosniff")
			c.Next()
		})
	group.POST("/reactions", handler.Reaction)
}
