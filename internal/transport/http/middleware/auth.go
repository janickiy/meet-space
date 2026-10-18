package httpmiddleware

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

const authenticatedUserIDKey = "authenticated_user_id"

type SessionVerifier interface {
	// VerifySession verifies the JWT and returns its account, guest-conference scope and expiration
	// claims.
	//
	// Parameters:
	//   - argument 1: Untrusted serialized value to decode or validate.
	//
	// Returns:
	//   - result 1: Verified account identifier.
	//   - result 2: Guest conference scope, or an empty string for an unrestricted registered
	//     account.
	//   - result 3: The timestamp or validity boundary associated with the result.
	//   - result 4: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	VerifySession(string) (string, string, time.Time, error)
}

// AuthorizationVerifier checks a persistent session using the request deadline.
type AuthorizationVerifier interface {
	// VerifyAuthorization verifies the authorization token and resolves its user, session, guest
	// scope and expiry.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Untrusted serialized value to decode or validate.
	//
	// Returns:
	//   - result 1: Verified account identifier.
	//   - result 2: Guest conference scope, or an empty string for a registered account.
	//   - result 3: Persistent server-session identifier, or an empty string for a guest or legacy
	//     token.
	//   - result 4: The timestamp or validity boundary associated with the result.
	//   - result 5: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	VerifyAuthorization(context.Context, string) (string, string, string, time.Time, error)
}

// GuestRequestAllowed keeps guest credentials inside one meeting, including WS.
// Normal conference admission/role checks still run after this scope check.
//
// Parameters:
//   - method: HTTP method of the requested route.
//   - path: Validated local filesystem or object path.
//   - conferenceID: Conference identifier limiting the operation scope.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func GuestRequestAllowed(method, path, conferenceID string) bool {
	if conferenceID == "" {
		return true
	}
	if method == "GET" && (path == "/api/v1/auth/me" || path == "/api/v1/capabilities" || path == "/api/v1/webrtc/config") {
		return true
	}
	if method == "POST" && path == "/api/v1/auth/logout" {
		return true
	}
	base := "/api/v1/conferences/" + conferenceID
	return path == base || strings.HasPrefix(path, base+"/")
}

// TokenVerifier defines the replaceable dependency contract for HTTP.
type TokenVerifier interface {
	// Verify validates the supplied credentials or signed binding before accepting the caller's
	// identity.
	//
	// Parameters:
	//   - argument 1: Untrusted serialized value to decode or validate.
	//
	// Returns:
	//   - result 1: Verified account identifier.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Verify(string) (string, error)
}

// Authenticate verifies the Bearer JWT and stores its validated account identity in the Gin
// context.
//
// Parameters:
//   - tokens: Token issuer or verifier used for trusted identity checks.
//
// Returns:
//   - result 1: The HTTP handler to register for this operation.
func Authenticate(tokens TokenVerifier) gin.HandlerFunc {

	return func(c *gin.Context) {
		header := strings.Fields(c.GetHeader("Authorization"))
		if len(header) != 2 || !strings.EqualFold(header[0], "Bearer") || tokens == nil {
			httpresponse.Fail(c, apperrors.ErrUnauthorized)
			return
		}
		var id, conferenceID string
		var err error
		if durable, ok := tokens.(AuthorizationVerifier); ok {
			id, conferenceID, _, _, err = durable.VerifyAuthorization(c.Request.Context(), header[1])
		} else if scoped, ok := tokens.(SessionVerifier); ok {
			id, conferenceID, _, err = scoped.VerifySession(header[1])
		} else {
			id, err = tokens.Verify(header[1])
		}
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		if id == "" {
			httpresponse.Fail(c, apperrors.ErrUnauthorized)
			return
		}
		if !GuestRequestAllowed(c.Request.Method, c.Request.URL.Path, conferenceID) {
			httpresponse.Fail(c, apperrors.ErrForbidden)
			return
		}
		c.Set(authenticatedUserIDKey, id)
		c.Next()
	}
}

// UserID reads the authenticated account identifier stored by the HTTP middleware.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//
// Returns:
//   - result 1: Authenticated account identifier from the request context.
func UserID(c *gin.Context) string { return c.GetString(authenticatedUserIDKey) }
