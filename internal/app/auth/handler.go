package authapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// Service defines the replaceable dependency contract for account authentication and persistent
// sessions.
type Service interface {
	// Register validates registration fields, hashes the password and creates a new account without
	// returning private credential data.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting view.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Register(context.Context, users.RegisterRequest) (users.View, error)
	// Login checks the supplied credentials and returns the safe account view and access token.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting login response.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Login(context.Context, users.LoginRequest) (users.LoginResponse, error)
	// Me returns the public view of the currently authenticated account.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//
	// Returns:
	//   - result 1: The resulting view.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Me(context.Context, string) (users.View, error)
	// UpdateProfile updates only the authenticated account's validated public profile fields.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting view.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	UpdateProfile(context.Context, string, users.UpdateProfileRequest) (users.View, error)
}

// Handler adapts HTTP requests to account authentication and persistent sessions, validates
// input and writes the response.
type Handler struct {
	service            Service
	publicOrigin       string
	secureCookie       bool
	persistentSessions bool
}

// NewHandler constructs the handler with the dependencies and limits required for account
// authentication and persistent sessions.
//
// Parameters:
//   - service: Application service invoked by this adapter.
//
// Returns:
//   - result 1: The constructed or loaded handler.
func NewHandler(service Service) *Handler { return &Handler{service: service} }

// Register validates registration fields, hashes the password and creates a new account without
// returning private credential data.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Register(c *gin.Context) {
	if !h.sameOrigin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	var request users.RegisterRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	user, err := h.service.Register(c.Request.Context(), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "success", "user": user})
}

// Login checks the supplied credentials and returns the safe account view and access token.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Login(c *gin.Context) {
	if !h.sameOrigin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	var request users.LoginRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	response, err := h.service.Login(c.Request.Context(), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if h.persistentSessions {
		if err := h.preserveSignIn(c, response); err != nil {
			if sessions, ok := h.service.(persistentService); ok {
				_ = sessions.Logout(c.Request.Context(), response.SessionToken, "")
			}
			httpresponse.Fail(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, response)
}

// Me returns the public view of the currently authenticated account.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Me(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	user, err := h.service.Me(c.Request.Context(), httpmiddleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "user": user})
}

// UpdateProfile updates only the authenticated account's validated public profile fields.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) UpdateProfile(c *gin.Context) {
	var request users.UpdateProfileRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	user, err := h.service.UpdateProfile(c.Request.Context(), httpmiddleware.UserID(c), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "user": user})
}

// Logout revokes the selected persistent session and invalidates the supplied access-session
// binding.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Logout(c *gin.Context) {
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
	if h.persistentSessions {
		sessions, ok := h.service.(persistentService)
		if !ok {
			httpresponse.Fail(c, apperrors.ErrUnavailable)
			return
		}
		raw := h.sessionCookie(c)
		id := request.UserID
		if id != "" {
			raw, _ = c.Cookie(accountCookieName(id))
			if raw == "" {
				if service, ok := h.service.(accountService); ok {
					if current, err := service.SessionAccount(c.Request.Context(), h.sessionCookie(c)); err == nil && current.ID == id {
						raw = h.sessionCookie(c)
					}
				}
			}
		}
		if service, ok := h.service.(accountService); ok {
			if user, err := service.SessionAccount(c.Request.Context(), raw); err == nil {
				id = user.ID
			}
		}
		if err := sessions.Logout(c.Request.Context(), raw, bearerToken(c)); err != nil {
			httpresponse.Fail(c, err)
			return
		}
		if id != "" {
			h.setNamedSessionCookie(c, accountCookieName(id), "")
		}
		if request.UserID == "" || raw == h.sessionCookie(c) {
			h.setSessionCookie(c, "")
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
