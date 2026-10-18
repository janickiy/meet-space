package recordingsapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/records"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// Service defines the replaceable dependency contract for conference recording lifecycle
// management.
type Service interface {
	// Delete removes the requested conference recording using the caller's scope and the resource
	// lifecycle rules.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: External recording UUID.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Delete(context.Context, string, string, string) error
	// Start starts conference recording lifecycle management and prepares the resources owned by
	// that operation.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting record card.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Start(context.Context, string, string, records.ConferenceStartRequest) (records.RecordCard, error)
	// Stop stops the active conference recording operation and releases its owned resources.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: External recording UUID.
	//
	// Returns:
	//   - result 1: The resulting record card.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Stop(context.Context, string, string, string) (records.RecordCard, error)
	// Read reads the conference recording state or stream required by the caller.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: External recording UUID.
	//
	// Returns:
	//   - result 1: The resulting record card.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Read(context.Context, string, string, string) (records.RecordCard, error)
	// List returns a bounded page of conference recording records using the supplied pagination and
	// filters.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Maximum number of results or operations in this batch.
	//   - argument 5: Number of results skipped before the requested page.
	//
	// Returns:
	//   - result 1: The collected record card values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	List(context.Context, string, string, int, int) ([]records.RecordCard, error)
}

// Handler adapts HTTP requests to conference recording lifecycle management, validates input and
// writes the response.
type Handler struct{ service Service }

// NewHandler constructs the handler with the dependencies and limits required for conference
// recording lifecycle management.
//
// Parameters:
//   - service: Application service invoked by this adapter.
//
// Returns:
//   - result 1: The constructed or loaded handler.
func NewHandler(service Service) *Handler { return &Handler{service: service} }

// parameter validates a required route parameter before invoking the application operation.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - key: Storage, cache or cryptographic key used by the operation.
//
// Returns:
//   - result 1: Validated required route parameter.
//   - result 2: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func parameter(c *gin.Context, key string) (string, bool) {
	id, err := uuid.Parse(c.Param(key))
	if err != nil || id == uuid.Nil {
		httpresponse.Fail(c, apperrors.New(apperrors.ErrInvalidInput, key+" must be a non-zero UUID"))
		return "", false
	}
	return id.String(), true
}

// Start starts conference recording lifecycle management and prepares the resources owned by
// that operation.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Start(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	var request records.ConferenceStartRequest
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	item, err := h.service.Start(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "success", "item": item})
}

// Stop stops the active conference recording operation and releases its owned resources.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Stop(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	recordID, ok := parameter(c, "recordingId")
	if !ok {
		return
	}
	if !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	item, err := h.service.Stop(c.Request.Context(), httpmiddleware.UserID(c), id, recordID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "success", "item": item})
}

// Delete removes the requested conference recording using the caller's scope and the resource
// lifecycle rules.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	recordID, ok := parameter(c, "recordingId")
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), httpmiddleware.UserID(c), id, recordID); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// Read reads the conference recording state or stream required by the caller.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Read(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	recordID, ok := parameter(c, "recordingId")
	if !ok {
		return
	}
	item, err := h.service.Read(c.Request.Context(), httpmiddleware.UserID(c), id, recordID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// List returns a bounded page of conference recording records using the supplied pagination and
// filters.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) List(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	limit, offset, ok := httpresponse.Pagination(c)
	if !ok {
		return
	}
	items, err := h.service.List(c.Request.Context(), httpmiddleware.UserID(c), id, limit, offset)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items})
}
