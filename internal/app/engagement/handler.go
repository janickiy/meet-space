package engagementapp

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/ratelimit"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	usecase "github.com/janickiy/meet-space/internal/usecase/realtime"
)

// Limiter defines the replaceable dependency contract for temporary conference reactions.
type Limiter interface {
	// Allow evaluates the rate limit and returns the remaining budget and reset boundary.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Storage, cache or cryptographic key used by the operation.
	//   - argument 3: Maximum number of results or operations in this batch.
	//   - argument 4: Duration of the rate-limit or observation window.
	//
	// Returns:
	//   - result 1: The resulting result.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Allow(context.Context, string, int, time.Duration) (ratelimit.Result, error)
}

// Handler adapts HTTP requests to temporary conference reactions, validates input and writes the
// response.
type Handler struct {
	service   *usecase.Engagement
	limiter   Limiter
	namespace string
}

// NewHandler constructs the handler with the dependencies and limits required for temporary
// conference reactions.
//
// Parameters:
//   - service: Application service invoked by this adapter.
//   - limiter: Shared request-rate limiter.
//   - namespace: Namespace isolating this component's distributed keys and channels.
//
// Returns:
//   - result 1: The constructed or loaded handler.
func NewHandler(service *usecase.Engagement, limiter Limiter, namespace string) *Handler {
	return &Handler{service: service, limiter: limiter, namespace: namespace}
}

// id validates the resource UUID from the HTTP route before an application or database
// operation.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//
// Returns:
//   - result 1: Validated resource UUID.
//   - result 2: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func id(c *gin.Context) (string, bool) {
	v, err := uuid.Parse(c.Param("id"))
	if err != nil || v == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return "", false
	}
	return v.String(), true
}

// limit checks the independent per-user and per-conference reaction budgets.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - kind: Validated event, job, media or resource category.
//   - n: Number of items or operations requested.
//   - window: Duration of the rate-limit or observation window.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (h *Handler) limit(c *gin.Context, conferenceID, kind string, n int, window time.Duration) bool {
	if h.limiter == nil {
		return true
	}
	keys := []string{h.namespace + ":engagement:" + kind + ":user:" + httpmiddleware.UserID(c), h.namespace + ":engagement:" + kind + ":conference:" + conferenceID}
	for i, key := range keys {
		count := n
		if i == 1 {
			// An unauthorized account must not consume another conference's shared reaction budget.
			if err := h.service.Authorize(c.Request.Context(), conferenceID, httpmiddleware.UserID(c)); err != nil {
				httpresponse.Fail(c, err)
				return false
			}
			count = 30
			window = time.Second
		}
		result, err := h.limiter.Allow(c.Request.Context(), key, count, window)
		if err != nil {
			httpresponse.Fail(c, err)
			return false
		}
		if !result.Allowed {
			c.Header("Retry-After", "10")
			c.AbortWithStatusJSON(429, gin.H{"error": "too many " + kind})
			return false
		}
	}
	return true
}

// Reaction publishes a supported temporary reaction on behalf of an admitted conference
// participant.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Reaction(c *gin.Context) {
	conf, ok := id(c)
	if !ok {
		return
	}
	var request struct {
		Emoji string `json:"emoji"`
	}
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	if !h.limit(c, conf, "reactions", 5, 10*time.Second) {
		return
	}
	if err := h.service.Reaction(c.Request.Context(), conf, httpmiddleware.UserID(c), request.Emoji); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success"})
}
