package callsapp

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	usecase "github.com/janickiy/meet-space/internal/usecase/calls"
)

type Handler struct{ Service *usecase.Service }

// validID checks that the supplied resource identifier is a valid nonzero UUID.
//
// Parameters:
//   - value: Value to validate, normalize or convert.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func validID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

// Create validates the request and returns the newly created direct call for the acting account.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Create(c *gin.Context) {
	var body struct {
		TargetUserID    string `json:"targetUserId"`
		ClientRequestID string `json:"clientRequestId"`
		ClientID        string `json:"clientId"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	actor := middleware.UserID(c)
	if !validID(body.TargetUserID) || !validID(body.ClientRequestID) || !validID(body.ClientID) || actor == body.TargetUserID {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	item, created, err := h.Service.Create(c.Request.Context(), actor, body.TargetUserID, body.ClientRequestID, body.ClientID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	code := 200
	if created {
		code = 201
	}
	c.JSON(code, gin.H{"status": "success", "item": item})
}

// Get serves the requested direct call within the caller's authorized scope.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Get(c *gin.Context) {
	id := c.Param("id")
	if !validID(id) {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	item, err := h.Service.Get(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success", "item": item})
}

// Active returns the account's current nonterminal direct call, if any.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Active(c *gin.Context) {
	item, err := h.Service.Active(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success", "item": item})
}

// Change applies the requested direct-call lifecycle action and returns its updated state.
//
// Parameters:
//   - action: Requested lifecycle or moderation action.
//
// Returns:
//   - result 1: The HTTP handler to register for this operation.
func (h *Handler) Change(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if !validID(id) {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		var body struct {
			ClientID string `json:"clientId"`
		}
		if !httpresponse.BindJSON(c, &body, false) {
			return
		}
		if action != "decline" && !validID(body.ClientID) {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		item, err := h.Service.Change(c.Request.Context(), middleware.UserID(c), id, action, body.ClientID)
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		c.JSON(200, gin.H{"status": "success", "item": item})
	}
}
