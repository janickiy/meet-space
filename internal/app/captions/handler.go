// Package captionsapp connects live-caption settings to authenticated HTTP endpoints.
package captionsapp

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/config"
	analytics "github.com/janickiy/meet-space/internal/domain/analytics"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	live "github.com/janickiy/meet-space/internal/usecase/captions"
	"net/http"
	"strconv"
	"time"
)

// Handler returns saved final captions and settings without opening a speech-provider session in
// the API.
type Handler struct {
	Repo      live.Repository
	Enabled   bool
	Analytics interface {
		// Read loads the saved technical conference aggregates after checking the account's history
		// access.
		//
		// Parameters:
		//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
		//   - argument 2: Acting account identifier used to scope access.
		//   - argument 3: Conference identifier limiting the operation scope.
		//
		// Returns:
		//   - result 1: The resulting conference.
		//   - result 2: nil on success; otherwise the validation, access, transport or persistence
		//     failure.
		Read(context.Context, string, string) (analytics.Conference, error)
	}
	Search interface {
		// Reindex queues a bounded transcript reindex after checking the actor's management permission.
		//
		// Parameters:
		//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
		//   - argument 2: Acting account identifier used to scope access.
		//   - argument 3: Conference identifier limiting the operation scope.
		//   - argument 4: Recording identifier used by the current operation.
		//   - argument 5: Operator-controlled configuration and processing limits.
		//   - argument 6: Minimum interval between expensive retriable processing requests.
		//
		// Returns:
		//   - result 1: nil on success; otherwise the validation, access, transport or persistence
		//     failure.
		Reindex(context.Context, string, string, string, config.StageEightConfig, time.Duration) error
	}
	Config config.StageEightConfig
}

// ReadAnalytics returns saved technical conference aggregates only to admitted participants.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) ReadAnalytics(c *gin.Context) {
	cid, ok := id(c)
	if !ok {
		return
	}
	value, err := h.Analytics.Read(c.Request.Context(), middleware.UserID(c), cid)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": value})
}

// Reindex queues a bounded transcript reindex after checking the actor's management permission.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Reindex(c *gin.Context) {
	cid, ok := id(c)
	if !ok {
		return
	}
	rid, err := uuid.Parse(c.Param("recordingId"))
	if err != nil || rid == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	err = h.Search.Reindex(c.Request.Context(), middleware.UserID(c), cid, rid.String(), h.Config, 5*time.Minute)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "success"})
}

// id validates the resource UUID from the HTTP route before an application or database
// operation.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//
// Returns:
//   - result 1: Validated resource UUID.
//   - result 2: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func id(c *gin.Context) (string, bool) {
	v, e := uuid.Parse(c.Param("id"))
	if e != nil || v == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return "", false
	}
	return v.String(), true
}

// Read reads the live-caption state state or stream required by the caller.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Read(c *gin.Context) {
	cid, ok := id(c)
	if !ok {
		return
	}
	s, e := h.Repo.Read(c.Request.Context(), middleware.UserID(c), cid)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	s.Available = h.Enabled
	s.CanManage = s.CanManage && h.Enabled
	if !s.Enabled {
		s.Status = "off"
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": s})
}

// Set updates the persisted live-caption state settings after validating the request.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Set(c *gin.Context) {
	cid, ok := id(c)
	if !ok {
		return
	}
	var request struct {
		Enabled  *bool  `json:"enabled"`
		Language string `json:"language"`
	}
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	if request.Enabled == nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	if !h.Enabled && *request.Enabled {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	if request.Language == "" {
		request.Language = "auto"
	}
	s, e := h.Repo.Set(c.Request.Context(), middleware.UserID(c), cid, *request.Enabled, request.Language)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	s.Available = h.Enabled
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": s})
}

// Finals returns a cursor-bounded page of saved final captions so clients can recover missed
// realtime events.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Finals(c *gin.Context) {
	cid, ok := id(c)
	if !ok {
		return
	}
	after, e := strconv.ParseInt(c.DefaultQuery("afterCursor", "0"), 10, 64)
	limit, e2 := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if e != nil || e2 != nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	items, e := h.Repo.Finals(c.Request.Context(), middleware.UserID(c), cid, after, limit)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	cursor := after
	for _, v := range items {
		cursor = max(cursor, v.Cursor)
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items, "nextCursor": cursor, "hasMore": len(items) == limit})
}
