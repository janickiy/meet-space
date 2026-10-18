package httptransport

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/telemetry"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// RegisterClientErrorRoutes registers the client error endpoints with the supplied
// authentication and request middleware.
//
// Parameters:
//   - router: HTTP router on which the component endpoints are registered.
//   - handler: Callback responsible for processing the scoped operation.
//   - limiter: Shared request-rate limiter.
func RegisterClientErrorRoutes(router gin.IRouter, handler *telemetry.Handler, limiter middleware.Limiter) {
	path := APIV1Prefix + "/client-errors"
	router.POST(path, middleware.RateLimit(limiter, middleware.RateLimitConfig{
		Enabled: true,
		Rules: []middleware.Rule{
			{Method: "POST", Path: path, Scope: "client-errors-ip", Limit: 30, Window: time.Minute, Key: middleware.ClientIPKey},
			{Method: "POST", Path: path, Scope: "client-errors-global", Limit: 300, Window: time.Minute, Key: func(*gin.Context) string { return "all" }},
		},
	}), handler.Receive)
}
