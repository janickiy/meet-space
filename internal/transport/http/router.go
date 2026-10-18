package httptransport

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/operations"
)

// APIV1Prefix is the primary versioned REST API prefix.
const APIV1Prefix = "/api/v1"

// NewRouter constructs the router with the dependencies and limits required for HTTP.
//
// Parameters:
//   - middleware: Additional request middleware applied in registration order.
//
// Returns:
//   - result 1: The constructed or loaded engine.
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
			// Never log query strings: one-use WebSocket tickets are credentials too.
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
