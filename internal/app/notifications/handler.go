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

// List возвращает ограниченный список личных уведомлений пользователя с принятыми в данном слое фильтрами.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
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

// Read идемпотентно отмечает принадлежащее пользователю уведомление прочитанным.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
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
func (h *Handler) Events(c *gin.Context) { h.events.serve(c) }
