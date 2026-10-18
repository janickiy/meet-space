package conferencesapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// Service defines the replaceable dependency contract for conference lifecycle and participant
// membership.
type Service interface {
	// Create creates and persists a new conference for the acting account.
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
	Create(context.Context, string, conferences.CreateRequest) (conferences.View, error)
	// List returns a bounded page of conference records using the supplied pagination and filters.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Maximum number of results or operations in this batch.
	//   - argument 4: Number of results skipped before the requested page.
	//
	// Returns:
	//   - result 1: The collected view values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	List(context.Context, string, int, int) ([]conferences.View, error)
	// Read reads the conference state or stream required by the caller.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting view.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Read(context.Context, string, string) (conferences.View, error)
	// Transition applies an allowed conference lifecycle transition without overwriting a newer
	// state.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Target state, account or decoded value, as defined by its type.
	//
	// Returns:
	//   - result 1: The resulting view.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Transition(context.Context, string, string, conferences.Status) (conferences.View, error)
	// Participants returns the conference participants visible to the acting account.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Maximum number of results or operations in this batch.
	//   - argument 5: Number of results skipped before the requested page.
	//
	// Returns:
	//   - result 1: The collected participant view values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Participants(context.Context, string, string, int, int) ([]conferences.ParticipantView, error)
	// Join creates or resumes the authorized membership or media-room connection, preserving its
	// current identity.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting participant view.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Join(context.Context, string, string, conferences.JoinRequest) (conferences.ParticipantView, error)
	// Leave removes the active membership or media connection while preserving the saved
	// conversation or conference history.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting participant view.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Leave(context.Context, string, string) (conferences.ParticipantView, error)
	// LookupInvite returns the limited conference information associated with a valid invitation
	// code.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Bounded public status or failure code.
	//
	// Returns:
	//   - result 1: The resulting invite view.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	LookupInvite(context.Context, string) (conferences.InviteView, error)
	// JoinInvite joins an authenticated account through a valid invitation while preserving existing
	// membership.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Bounded public status or failure code.
	//
	// Returns:
	//   - result 1: The resulting participant view.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	JoinInvite(context.Context, string, string) (conferences.ParticipantView, error)
	// Self returns the acting account's membership, including waiting-room and admission state.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting participant view.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Self(context.Context, string, string) (conferences.ParticipantView, error)
	// Admission accepts or rejects waiting-room admission after checking the acting member's
	// authority.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Conference participant identifier.
	//   - argument 5: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting participant view.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Admission(context.Context, string, string, string, conferences.AdmissionRequest) (conferences.ParticipantView, error)
	// Schedule updates a planned conference schedule after validating ownership and time
	// constraints.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting view.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Schedule(context.Context, string, string, conferences.ScheduleRequest) (conferences.View, error)
	// Timeline returns the account's paginated upcoming, active or past conference timeline.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Search text or validated query filters.
	//
	// Returns:
	//   - result 1: The resulting timeline page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Timeline(context.Context, string, conferences.TimelineQuery) (conferences.TimelinePage, error)
	// History builds the authorized completed-conference view with participant history and recording
	// summaries.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting history view.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	History(context.Context, string, string) (conferences.HistoryView, error)
}

// Handler adapts HTTP requests to conference lifecycle and participant membership, validates
// input and writes the response.
type Handler struct{ service Service }

// NewHandler constructs the handler with the dependencies and limits required for conference
// lifecycle and participant membership.
//
// Parameters:
//   - service: Application service invoked by this adapter.
//
// Returns:
//   - result 1: The constructed or loaded handler.
func NewHandler(service Service) *Handler { return &Handler{service: service} }

// Create validates the request and returns the newly created conference for the acting account.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Create(c *gin.Context) {
	var request conferences.CreateRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, err := h.service.Create(c.Request.Context(), httpmiddleware.UserID(c), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "success", "item": item})
}

// List returns a bounded page of conference records using the supplied pagination and filters.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) List(c *gin.Context) {
	limit, offset, ok := httpresponse.Pagination(c)
	if !ok {
		return
	}
	items, err := h.service.List(c.Request.Context(), httpmiddleware.UserID(c), limit, offset)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items})
}

// Read reads the conference state or stream required by the caller.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Read(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	item, err := h.service.Read(c.Request.Context(), httpmiddleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// Transition applies an allowed conference lifecycle transition without overwriting a newer
// state.
//
// Parameters:
//   - target: Target state, account or decoded value, as defined by its type.
//
// Returns:
//   - result 1: The HTTP handler to register for this operation.
func (h *Handler) Transition(target conferences.Status) gin.HandlerFunc {

	return func(c *gin.Context) {
		id, ok := conferenceID(c)
		if !ok {
			return
		}
		if !httpresponse.BindJSON(c, &struct{}{}, true) {
			return
		}
		item, err := h.service.Transition(c.Request.Context(), httpmiddleware.UserID(c), id, target)
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
	}
}

// Participants returns the conference participants visible to the acting account.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Participants(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	limit, offset, ok := httpresponse.Pagination(c)
	if !ok {
		return
	}
	items, err := h.service.Participants(c.Request.Context(), httpmiddleware.UserID(c), id, limit, offset)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items})
}

// Join creates or resumes the authorized membership or media-room connection, preserving its
// current identity.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Join(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	var request conferences.JoinRequest
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	item, err := h.service.Join(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// Leave removes the active membership or media connection while preserving the saved
// conversation or conference history.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Leave(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	if !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	item, err := h.service.Leave(c.Request.Context(), httpmiddleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// LookupInvite returns the limited conference information associated with a valid invitation
// code.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) LookupInvite(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	item, err := h.service.LookupInvite(c.Request.Context(), c.Param("code"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// JoinInvite joins an authenticated account through a valid invitation while preserving existing
// membership.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) JoinInvite(c *gin.Context) {
	if !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	item, err := h.service.JoinInvite(c.Request.Context(), httpmiddleware.UserID(c), c.Param("code"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// conferenceID validates and normalizes the conference identifier from the request route.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//
// Returns:
//   - result 1: Validated conference UUID.
//   - result 2: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func conferenceID(c *gin.Context) (string, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil {
		httpresponse.Fail(c, apperrors.New(apperrors.ErrInvalidInput, "conference id must be a non-zero UUID"))
		return "", false
	}
	return id.String(), true
}
