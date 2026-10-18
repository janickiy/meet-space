package integrationsapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	d "github.com/janickiy/meet-space/internal/domain/integrations"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	u "github.com/janickiy/meet-space/internal/usecase/integrations"
)

// Handler exposes only public account integration views; provider credentials are never
// serialized into API responses.
type Handler struct{ service *u.Service }

// NewHandler constructs the handler with the dependencies and limits required for account
// integrations and delivery preferences.
//
// Parameters:
//   - service: Application service invoked by this adapter.
//
// Returns:
//   - result 1: The constructed or loaded handler.
func NewHandler(service *u.Service) *Handler { return &Handler{service: service} }

// Capabilities returns the product or integration capabilities actually enabled by the operator.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Capabilities(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, h.service.Capabilities())
}

// Preferences reads the acting account's saved delivery or conversation preferences.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Preferences(c *gin.Context) {
	value, err := h.service.Preferences(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, value)
}

// SavePreferences validates and saves the account's delivery preferences without triggering a
// delivery.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) SavePreferences(c *gin.Context) {
	var request d.Preferences
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	if err := h.service.SavePreferences(c.Request.Context(), middleware.UserID(c), request); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, request)
}

// Devices returns the account's registered delivery devices without exposing their private
// credentials.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Devices(c *gin.Context) {
	items, err := h.service.Devices(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// RegisterDevice registers the acting account's validated delivery device and protects its
// private token.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) RegisterDevice(c *gin.Context) {
	var request d.DeviceRequest
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	item, err := h.service.RegisterDevice(c.Request.Context(), middleware.UserID(c), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": item})
}

// RevokeDevice revokes only the acting account's selected delivery device.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) RevokeDevice(c *gin.Context) {
	id, ok := resourceID(c, "deviceId")
	if !ok {
		return
	}
	if err := h.service.RevokeDevice(c.Request.Context(), middleware.UserID(c), id); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Calendars returns the acting account's calendar connections with private provider credentials
// omitted.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Calendars(c *gin.Context) {
	items, err := h.service.Connections(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// MockConnect creates a mock calendar connection only when the configured environment explicitly
// permits it.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) MockConnect(c *gin.Context) {
	var request struct{}
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	item, err := h.service.MockConnect(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": item})
}

// Connect opens the configured dependency or starts its authorized connection flow.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Connect(c *gin.Context) {
	authURL, state, err := h.service.OAuthStart(c.Request.Context(), middleware.UserID(c), c.Param("provider"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"authUrl": authURL, "state": state})
}

// oauthCallbackRequest accepts only a one-time code and state, never provider access or refresh
// tokens.
type oauthCallbackRequest struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

// Callback completes an authorized calendar connection using its one-time OAuth state and code.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Callback(c *gin.Context) {
	var request oauthCallbackRequest
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	item, err := h.service.OAuthCallback(c.Request.Context(), middleware.UserID(c), c.Param("provider"), request.Code, request.State)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"item": item})
}

// Disconnect revokes the acting account's integration connection and stops its future
// synchronization.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Disconnect(c *gin.Context) {
	id, ok := resourceID(c, "provider")
	if !ok {
		return
	}
	if err := h.service.Disconnect(c.Request.Context(), middleware.UserID(c), id); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// CalendarMappings returns saved conference-to-calendar mappings for the acting account.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) CalendarMappings(c *gin.Context) {
	id, ok := resourceID(c, "id")
	if !ok {
		return
	}
	items, err := h.service.CalendarMappings(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// resourceID validates the resource identifier from the integration route.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - name: Name of the setting, cookie, resource or provider being addressed.
//
// Returns:
//   - result 1: Validated integration-resource UUID.
//   - result 2: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func resourceID(c *gin.Context, name string) (string, bool) {
	value, err := uuid.Parse(c.Param(name))
	if err != nil || value == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return "", false
	}
	return value.String(), true
}
