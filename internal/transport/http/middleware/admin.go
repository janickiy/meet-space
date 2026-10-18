package httpmiddleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// AdminChecker reads the authenticated account's currently persisted global administrator
// privilege.
type AdminChecker interface {
	// IsAdmin checks the configured administrative identity without exposing the administrator
	// configuration.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//
	// Returns:
	//   - result 1: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	IsAdmin(context.Context, string) (bool, error)
}

// RequireAdmin checks persisted administrator access on each request rather than trusting
// conference roles or stale token claims.
//
// Parameters:
//   - checker: Persisted administrator-access checker.
//
// Returns:
//   - result 1: The HTTP handler to register for this operation.
func RequireAdmin(checker AdminChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if checker == nil || UserID(c) == "" {
			httpresponse.Fail(c, apperrors.ErrUnauthorized)
			return
		}
		allowed, err := checker.IsAdmin(c.Request.Context(), UserID(c))
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		if !allowed {
			httpresponse.Fail(c, apperrors.ErrForbidden)
			return
		}
		c.Next()
	}
}
