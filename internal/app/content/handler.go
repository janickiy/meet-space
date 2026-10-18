package contentapp

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/content"
	"github.com/janickiy/meet-space/internal/operations"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	"net/http"
	"strconv"
	"time"
)

// Service defines content operations that never invoke external transcription or AI inside an
// HTTP request.
type Service interface {
	// Transcript returns the current transcript metadata and provider capabilities without starting
	// transcription.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Recording identifier used by the current operation.
	//
	// Returns:
	//   - result 1: The resulting transcript state.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Transcript(context.Context, string, string, string) (domain.TranscriptState, error)
	// Segments returns a bounded page of saved transcript segments for timestamp navigation.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Recording identifier used by the current operation.
	//   - argument 5: Maximum number of results or operations in this batch.
	//   - argument 6: Number of results skipped before the requested page.
	//
	// Returns:
	//   - result 1: The resulting segment page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Segments(context.Context, string, string, string, int, int) (domain.SegmentPage, error)
	// Summary returns the current persisted summary without invoking an external provider.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Recording identifier used by the current operation.
	//
	// Returns:
	//   - result 1: The resulting summary state.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Summary(context.Context, string, string, string) (domain.SummaryState, error)
	// RetryTranscript queues an authorized transcription retry instead of running the expensive
	// provider call in the HTTP request.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Recording identifier used by the current operation.
	//
	// Returns:
	//   - result 1: The resulting transcript state.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	RetryTranscript(context.Context, string, string, string) (domain.TranscriptState, error)
	// RegenerateSummary queues a bounded summary generation without blocking the HTTP request on the
	// AI provider.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Recording identifier used by the current operation.
	//
	// Returns:
	//   - result 1: The resulting summary state.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	RegenerateSummary(context.Context, string, string, string) (domain.SummaryState, error)
	// Search searches recording transcripts and summaries using bounded input and the caller's
	// visibility filters.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Search text or validated query filters.
	//
	// Returns:
	//   - result 1: The resulting search page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Search(context.Context, string, domain.SearchQuery) (domain.SearchPage, error)
}

// Handler connects authorized HTTP requests to the recording-content application operations.
type Handler struct{ service Service }

// NewHandler constructs the handler with the dependencies and limits required for recording
// transcripts and summaries.
//
// Parameters:
//   - service: Application service invoked by this adapter.
//
// Returns:
//   - result 1: The constructed or loaded handler.
func NewHandler(service Service) *Handler { return &Handler{service: service} }

// identifiers validates the related conference and recording UUIDs before reading content or
// queueing work.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//
// Returns:
//   - result 1: Validated conference UUID.
//   - result 2: Validated recording UUID.
//   - result 3: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func identifiers(c *gin.Context) (string, string, bool) {
	cid, e := uuid.Parse(c.Param("id"))
	rid, e2 := uuid.Parse(c.Param("recordingId"))
	if e != nil || e2 != nil || cid == uuid.Nil || rid == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return "", "", false
	}
	return cid.String(), rid.String(), true
}

// pageValue parses a decimal pagination value and distinguishes missing input from invalid
// input.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - key: Storage, cache or cryptographic key used by the operation.
//   - fallback: Value used when the optional setting is absent or invalid.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func pageValue(c *gin.Context, key string, fallback int) (int, bool) {
	value, ok := c.GetQuery(key)
	if !ok {
		return fallback, true
	}
	n, e := strconv.Atoi(value)
	if e != nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return 0, false
	}
	return n, true
}

// Transcript returns the current transcript metadata and provider capabilities without starting
// transcription.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Transcript(c *gin.Context) {
	cid, rid, ok := identifiers(c)
	if !ok {
		return
	}
	state, e := h.service.Transcript(c.Request.Context(), httpmiddleware.UserID(c), cid, rid)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": state.Item, "enabled": state.Enabled, "canRetry": state.CanRetry, "providerMode": state.ProviderMode})
}

// Segments returns a bounded page of saved transcript segments for timestamp navigation.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Segments(c *gin.Context) {
	cid, rid, ok := identifiers(c)
	if !ok {
		return
	}
	limit, ok := pageValue(c, "limit", 100)
	if !ok {
		return
	}
	offset, ok := pageValue(c, "offset", 0)
	if !ok {
		return
	}
	page, e := h.service.Segments(c.Request.Context(), httpmiddleware.UserID(c), cid, rid, limit, offset)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "total": page.Total, "limit": page.Limit, "offset": page.Offset})
}

// Summary returns the current persisted summary without invoking an external provider.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Summary(c *gin.Context) {
	cid, rid, ok := identifiers(c)
	if !ok {
		return
	}
	state, e := h.service.Summary(c.Request.Context(), httpmiddleware.UserID(c), cid, rid)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": state.Item, "enabled": state.Enabled, "canRegenerate": state.CanRegenerate, "providerMode": state.ProviderMode})
}

// RetryTranscript queues an authorized transcription retry instead of running the expensive
// provider call in the HTTP request.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) RetryTranscript(c *gin.Context) {
	cid, rid, ok := identifiers(c)
	if !ok || !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	state, e := h.service.RetryTranscript(c.Request.Context(), httpmiddleware.UserID(c), cid, rid)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "success", "item": state.Item, "enabled": state.Enabled, "canRetry": false, "providerMode": state.ProviderMode})
}

// RegenerateSummary queues a bounded summary generation without blocking the HTTP request on the
// AI provider.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) RegenerateSummary(c *gin.Context) {
	cid, rid, ok := identifiers(c)
	if !ok || !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	state, e := h.service.RegenerateSummary(c.Request.Context(), httpmiddleware.UserID(c), cid, rid)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "success", "item": state.Item, "enabled": state.Enabled, "canRegenerate": false, "providerMode": state.ProviderMode})
}

// Search searches recording transcripts and summaries using bounded input and the caller's
// visibility filters.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Search(c *gin.Context) {
	started := time.Now()
	defer func() { operations.Search(time.Since(started), c.Writer.Status() >= http.StatusBadRequest) }()
	limit, ok := pageValue(c, "limit", 20)
	if !ok {
		return
	}
	offset, ok := pageValue(c, "offset", 0)
	if !ok {
		return
	}
	q := domain.SearchQuery{Query: c.Query("q"), Source: c.Query("source"), ConferenceID: c.Query("conferenceId"), Mode: c.Query("mode"), ParticipantID: c.Query("participantId"), Membership: c.Query("membership"), Limit: limit, Offset: offset}
	for key, target := range map[string]**time.Time{"from": &q.From, "to": &q.To} {
		if value, exists := c.GetQuery(key); exists {
			at, e := time.Parse(time.RFC3339, value)
			if e != nil {
				httpresponse.Fail(c, apperrors.ErrInvalidInput)
				return
			}
			utc := at.UTC()
			*target = &utc
		}
	}
	page, e := h.service.Search(c.Request.Context(), httpmiddleware.UserID(c), q)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "total": page.Total, "limit": page.Limit, "offset": page.Offset, "effectiveMode": page.EffectiveMode, "fallbackReason": page.FallbackReason})
}
