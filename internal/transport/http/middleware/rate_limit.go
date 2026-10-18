package httpmiddleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/ratelimit"
)

// Limiter defines the replaceable dependency contract for HTTP.
type Limiter interface {
	// Allow evaluates the rate limit and returns the remaining budget and reset boundary.
	//
	// Parameters:
	//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
	//   - key: Storage, cache or cryptographic key used by the operation.
	//   - limit: Maximum number of results or operations in this batch.
	//   - window: Duration of the rate-limit or observation window.
	//
	// Returns:
	//   - result 1: The resulting result.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (ratelimit.Result, error)
}

// KeyFunc builds the client identity for one server-controlled rate rule.
type KeyFunc func(c *gin.Context) string

// Rule defines a separate rate budget, scope, identity key and time window.
type Rule struct {
	Method string
	Path   string
	Scope  string
	Limit  int
	Window time.Duration
	Key    KeyFunc
}

// RateLimitConfig contains the rate rules and limiter failure policy.
type RateLimitConfig struct {
	Enabled bool
	Rules   []Rule
}

// RateLimit constructs the Gin middleware that enforces the configured shared request budget.
//
// Parameters:
//   - limiter: Shared request-rate limiter.
//   - cfg: Operator-controlled configuration and processing limits.
//
// Returns:
//   - result 1: The HTTP handler to register for this operation.
func RateLimit(limiter Limiter, cfg RateLimitConfig) gin.HandlerFunc {
	rulesByRoute := make(map[string][]Rule)
	for _, rule := range cfg.Rules {
		if rule.Limit <= 0 || rule.Key == nil {
			continue
		}
		method := strings.ToUpper(strings.TrimSpace(rule.Method))
		path := strings.TrimSpace(rule.Path)
		if method == "" || path == "" {
			continue
		}
		rule.Method = method
		rule.Path = path
		rule.Scope = strings.TrimSpace(rule.Scope)
		if rule.Scope == "" {
			rule.Scope = "default"
		}
		if rule.Window <= 0 {
			rule.Window = time.Minute
		}
		routeKey := method + " " + path
		rulesByRoute[routeKey] = append(rulesByRoute[routeKey], rule)
	}

	return func(c *gin.Context) {
		if !cfg.Enabled || limiter == nil {
			c.Next()
			return
		}
		routeRules := rulesByRoute[c.Request.Method+" "+c.FullPath()]
		if len(routeRules) == 0 {
			c.Next()
			return
		}

		for _, rule := range routeRules {
			identity := strings.TrimSpace(rule.Key(c))
			if identity == "" {
				continue
			}
			result, err := limiter.Allow(c.Request.Context(), redisRateLimitKey(rule, identity), rule.Limit, rule.Window)
			if err != nil {
				abortRateLimitError(c, http.StatusInternalServerError, "rate limit unavailable")
				return
			}
			if !result.Allowed {
				abortRateLimitExceeded(c, result)
				return
			}
			setRateLimitHeaders(c, result)
		}

		c.Next()
	}
}

// ClientIPKey builds the rate-limit subject from the trusted client-address resolution.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//
// Returns:
//   - result 1: Rate-limit identity derived from the trusted client address.
func ClientIPKey(c *gin.Context) string {
	return c.ClientIP()
}

// redisRateLimitKey builds a namespaced Redis key without including an unbounded request value.
//
// Parameters:
//   - rule: Server-controlled request-rate policy.
//   - identity: Verified account and physical-session identity.
//
// Returns:
//   - result 1: Namespaced rate-limit bucket key.
func redisRateLimitKey(rule Rule, identity string) string {
	return strings.Join([]string{
		"rate",
		safeKeyPart(rule.Scope),
		safeKeyPart(identity),
		safeKeyPart(rule.Method),
		safeKeyPart(rule.Path),
	}, ":")
}

// safeKeyPart bounds and normalizes a value before using it as a rate-limit key component.
//
// Parameters:
//   - value: Value to validate, normalize or convert.
//
// Returns:
//   - result 1: Bounded safe key component.
func safeKeyPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "/", ".")

	return value
}

// abortRateLimitExceeded writes the rate-limit response and stops the Gin handler chain.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - result: Validated processing or operation result.
func abortRateLimitExceeded(c *gin.Context, result ratelimit.Result) {
	setRateLimitHeaders(c, result)
	if result.RetryAfter > 0 {
		c.Header("Retry-After", strconv.FormatInt(int64(result.RetryAfter.Seconds()), 10))
	}
	httpresponse.Fail(c, apperrors.ErrRateLimited)
}

// abortRateLimitError writes the safe limiter-failure response and stops request processing.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - status: Lifecycle state or response status to record.
//   - message: Bounded message text used by this operation; do not include private credentials.
func abortRateLimitError(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"status":  "failed",
		"message": message,
	})
}

// setRateLimitHeaders adds the accepted rate-limit budget and reset metadata to the response.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - result: Validated processing or operation result.
func setRateLimitHeaders(c *gin.Context, result ratelimit.Result) {
	if result.Limit > 0 {
		c.Header("X-RateLimit-Limit", strconv.Itoa(result.Limit))
	}
	if result.Remaining >= 0 {
		c.Header("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
	}
	if !result.ResetAt.IsZero() {
		c.Header("X-RateLimit-Reset", strconv.FormatInt(result.ResetAt.Unix(), 10))
	}
}
