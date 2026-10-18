package conferencesapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	d "github.com/janickiy/meet-space/internal/domain/conferences"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

type InvitationService interface {
	// Search searches conference lifecycle and participant membership using bounded input and the
	// caller's visibility filters.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Search text or validated query filters.
	//
	// Returns:
	//   - result 1: The collected invitation user values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Search(context.Context, string, string, string) ([]d.InvitationUser, error)
	// Invite creates or updates conference invitations for the validated recipients.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The collected invitation result values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Invite(context.Context, string, string, d.InvitationRequest) ([]d.InvitationResult, error)
}
type InvitationHandler struct{ Service InvitationService }

// Search searches conference lifecycle and participant membership using bounded input and the
// caller's visibility filters.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *InvitationHandler) Search(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	items, err := h.Service.Search(c.Request.Context(), middleware.UserID(c), id, c.Query("query"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items})
}

// Invite creates or updates conference invitations for the validated recipients.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *InvitationHandler) Invite(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	var request d.InvitationRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	items, err := h.Service.Invite(c.Request.Context(), middleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "success", "items": items})
}
