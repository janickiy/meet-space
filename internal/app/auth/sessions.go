package authapp

import (
	"context"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

const SessionCookieName = "meetrix_session"
const sessionCookiePath = "/api/v1/auth"
const sessionCookieMaxAge = 400 * 24 * 60 * 60

type persistentService interface {
	// Refresh renews the short-lived access credential using the current provider or account refresh
	// session.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Untrusted serialized value to decode or validate.
	//
	// Returns:
	//   - result 1: The resulting login response.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Refresh(context.Context, string) (users.LoginResponse, error)
	// Bootstrap upgrades an existing valid access credential to a persistent account session.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Untrusted serialized value to decode or validate.
	//   - argument 4: Current signed access token, used only for session verification.
	//
	// Returns:
	//   - result 1: The resulting login response.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Bootstrap(context.Context, string, string, string) (users.LoginResponse, error)
	// Logout revokes the selected persistent session and invalidates the supplied access-session
	// binding.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Untrusted serialized value to decode or validate.
	//   - argument 3: Current signed access token, used only for session verification.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Logout(context.Context, string, string) error
}

// WithSessionCookies configures the public origin rather than trusting forwarded hosts.
//
// Parameters:
//   - publicURL: Operator-configured public frontend origin.
//   - secure: Whether cookies must be restricted to HTTPS.
//
// Returns:
//   - result 1: The constructed or loaded handler.
func (h *Handler) WithSessionCookies(publicURL string, secure bool) *Handler {
	if parsed, err := url.Parse(publicURL); err == nil && parsed.Host != "" {
		h.publicOrigin = parsed.Scheme + "://" + parsed.Host
	}
	h.secureCookie = secure
	h.persistentSessions = true
	return h
}

// PersistentSessionsEnabled reports whether durable account sessions are configured for this
// handler.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (h *Handler) PersistentSessionsEnabled() bool { return h.persistentSessions }

// sameOrigin rejects cross-origin session mutations using the configured public origin and
// request metadata.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (h *Handler) sameOrigin(c *gin.Context) bool {
	if strings.EqualFold(c.GetHeader("Sec-Fetch-Site"), "cross-site") {
		httpresponse.Fail(c, apperrors.ErrForbidden)
		return false
	}
	origin := c.GetHeader("Origin")
	if origin == "" {
		// Non-browser CLI clients do not send Origin or Fetch Metadata.
		if c.GetHeader("Sec-Fetch-Site") != "" {
			httpresponse.Fail(c, apperrors.ErrForbidden)
			return false
		}
	} else {
		expected := h.publicOrigin
		if expected == "" {
			scheme := "http"
			if h.secureCookie || c.Request.TLS != nil {
				scheme = "https"
			}
			expected = scheme + "://" + c.Request.Host
		}
		if origin != expected {
			httpresponse.Fail(c, apperrors.ErrForbidden)
			return false
		}
	}
	if c.Request.ContentLength != 0 {
		mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || mediaType != "application/json" {
			c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, gin.H{"status": "failed", "message": "application/json is required"})
			return false
		}
	}
	return true
}

// sessionCookie reads the selected account's refresh cookie for a session operation.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//
// Returns:
//   - result 1: Selected account's raw refresh secret, or an empty string when absent.
func (h *Handler) sessionCookie(c *gin.Context) string {
	raw, _ := c.Cookie(SessionCookieName)
	return raw
}

// setSessionCookie sets or clears the selected persistent refresh cookie with the configured
// security attributes.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - raw: Serialized access token or refresh credential to validate; never log or expose it.
func (h *Handler) setSessionCookie(c *gin.Context, raw string) {
	h.setNamedSessionCookie(c, SessionCookieName, raw)
}

// setNamedSessionCookie sets or clears a named refresh cookie without exposing its secret to
// JavaScript.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - name: Name of the setting, cookie, resource or provider being addressed.
//   - raw: Serialized access token or refresh credential to validate; never log or expose it.
func (h *Handler) setNamedSessionCookie(c *gin.Context, name, raw string) {
	cookie := &http.Cookie{Name: name, Value: raw, Path: sessionCookiePath,
		MaxAge: sessionCookieMaxAge, Expires: time.Now().UTC().Add(time.Duration(sessionCookieMaxAge) * time.Second),
		HttpOnly: true, Secure: h.secureCookie, SameSite: http.SameSiteLaxMode}
	if raw == "" {
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(1, 0).UTC()
	}
	http.SetCookie(c.Writer, cookie)
}

// bearerToken extracts the access token from a well-formed Bearer authorization header.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//
// Returns:
//   - result 1: Raw Bearer access token, or an empty string for an invalid header.
func bearerToken(c *gin.Context) string {
	fields := strings.Fields(c.GetHeader("Authorization"))
	if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
		return fields[1]
	}
	return ""
}

// Refresh accepts a browser session even when its previous JWT has expired.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Refresh(c *gin.Context) { h.sessionOperation(c, false) }

// Bootstrap migrates existing signed-in accounts without requiring a new login.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Bootstrap(c *gin.Context) { h.sessionOperation(c, true) }

// sessionOperation handles refresh or bootstrap requests with strict origin, content-type and
// account checks.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - bootstrap: Whether to upgrade an existing access token rather than refresh a persistent
//     session.
func (h *Handler) sessionOperation(c *gin.Context, bootstrap bool) {
	if !h.sameOrigin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	var request struct {
		UserID string `json:"userId"`
	}
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	sessions, ok := h.service.(persistentService)
	if !ok || !h.persistentSessions {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	var response users.LoginResponse
	var err error
	if bootstrap {
		id := httpmiddleware.UserID(c)
		raw, _ := c.Cookie(accountCookieName(id))
		if raw == "" {
			raw = h.sessionCookie(c)
		}
		response, err = sessions.Bootstrap(c.Request.Context(), id, raw, bearerToken(c))
	} else {
		raw := h.sessionCookie(c)
		if request.UserID != "" {
			raw, _ = c.Cookie(accountCookieName(request.UserID))
			// Existing browsers initially have only the original cookie. Verify its
			// owner below before migrating it; never restore another identity.
			if raw == "" {
				raw = h.sessionCookie(c)
			}
		}
		response, err = sessions.Refresh(c.Request.Context(), raw)
		if err == nil && request.UserID != "" && response.User.ID != request.UserID {
			err = apperrors.ErrForbidden
		}
	}
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	// A background renewal must not change the active account of other tabs.
	if bootstrap || request.UserID == "" {
		h.setSessionCookie(c, response.SessionToken)
	}
	h.setNamedSessionCookie(c, accountCookieName(response.User.ID), response.SessionToken)
	c.JSON(http.StatusOK, response)
}
