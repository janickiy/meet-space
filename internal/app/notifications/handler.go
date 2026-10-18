package notificationsapp

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	usecase "github.com/janickiy/meet-space/internal/usecase/notifications"
)

// Handler serves notification queries and delegates SSE connection ownership.
type Handler struct {
	service *usecase.Service
	events  *eventStreams
}

// NewHandler preserves the HTTP API while giving SSE its own lifecycle owner.
//
// Parameters:
//   - service: Application service invoked by this adapter.
//   - bus: Scoped publication and subscription transport.
//   - tokens: Token issuer or verifier used for trusted identity checks.
//   - limiter: Shared request-rate limiter.
//   - namespace: Namespace isolating this component's distributed keys and channels.
//
// Returns:
//   - result 1: The constructed or loaded handler.
func NewHandler(service *usecase.Service, bus Subscriber, tokens Verifier, limiter Limiter, namespace string) *Handler {
	return &Handler{service: service, events: &eventStreams{
		service: service, bus: bus, tokens: tokens, limiter: limiter,
		namespace: namespace, counts: syncCounts{values: map[string]int{}},
		drain: make(chan struct{}),
	}}
}

// BeginDrain releases SSE requests so a graceful API deployment can finish.
func (h *Handler) BeginDrain() {
	if h.events != nil {
		h.events.beginDrain()
	}
}

// List returns a bounded page of personal notification records using the supplied pagination and
// filters.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) List(c *gin.Context) {
	limit := 30
	if raw := c.Query("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		limit = v
	}
	page, err := h.service.List(c.Request.Context(), httpmiddleware.UserID(c), c.Query("cursor"), limit)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, page)
}

// Read idempotently marks the account-owned notification as read and returns its saved state.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Read(c *gin.Context) {
	id, err := uuid.Parse(c.Param("notificationId"))
	if err != nil || id == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	if !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	item, err := h.service.Read(c.Request.Context(), httpmiddleware.UserID(c), id.String())
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"item": item})
}

// Events opens an authorized notification stream owned by the SSE component.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Events(c *gin.Context) { h.events.serve(c) }
