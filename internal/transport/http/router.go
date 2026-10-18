package httptransport

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/operations"
)

// APIV1Prefix — это основная версия префикса REST API.
const APIV1Prefix = "/api/v1"

// NewRouter создает маршрутизатор с зависимостями и ограничениями, необходимыми для HTTP.
//
// @Parameters:
//   - middleware: Дополнительное промежуточное ПО для обработки запросов, применяемое в порядке
//     регистрации.
//
// @returns:
//   - result1: Созданный или загруженный движок.
func NewRouter(middleware ...gin.HandlerFunc) *gin.Engine {
	router := gin.New()
	router.Use(

		func(c *gin.Context) {
			defer func() {
				if recover() != nil {
					slog.Error("http request panic", "method", c.Request.Method, "route", c.FullPath())
					c.AbortWithStatusJSON(500, gin.H{"status": "failed", "message": "internal server error"})
				}
			}()
			c.Next()
		},

		func(c *gin.Context) {
			start := time.Now()
			c.Next()
			// Никогда не логируйте строки запроса: одноразовые билеты WebSocket — это тоже
			// учетные данные.
			slog.Info("http request", "method", c.Request.Method, "route", c.FullPath(), "status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds(), "request_id", operations.ID(c.Request.Context()))
		})
	if len(middleware) > 0 {
		router.Use(middleware...)
	}
	router.GET("/health",

		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
	return router
}
