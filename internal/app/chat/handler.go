package chatapp

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	chatusecase "github.com/janickiy/meet-space/internal/usecase/chat"
)

// Handler adapts HTTP requests to persistent chat messages and private attachments, validates
// input and writes the response.
type Handler struct {
	service        *chatusecase.Service
	namespace      string
	groupDownloads GroupDownloads
}

// NewHandler constructs the handler with the dependencies and limits required for persistent
// chat messages and private attachments.
//
// Parameters:
//   - service: Application service invoked by this adapter.
//
// Returns:
//   - result 1: The constructed or loaded handler.
func NewHandler(service *chatusecase.Service) *Handler {
	return &Handler{service: service, namespace: "conferences"}
}

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
	id, err := chat.UUID(c.Param(key))
	if err != nil {
		httpresponse.Fail(c, err)
		return "", false
	}
	return id, true
}

// List returns a bounded page of chat message records using the supplied pagination and filters.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) List(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	limit := 50
	if raw, exists := c.GetQuery("limit"); exists {
		var err error
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			httpresponse.Fail(c, apperrors.New(apperrors.ErrInvalidInput, "limit must be between 1 and 100"))
			return
		}
	}
	page, err := h.service.List(c.Request.Context(), httpmiddleware.UserID(c), id, c.Query("before"), limit)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "nextCursor": page.NextCursor, "unreadCount": page.UnreadCount, "lastReadMessageId": page.LastReadMessageID})
}

// Send validates access, reply and attachment references and persists the chat message with
// idempotency protection against duplicate retries.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Send(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	var request chat.SendRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, created, err := h.service.Send(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"status": "success", "item": item})
}

// Edit changes the author's existing message text after checking visibility and edit
// eligibility.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Edit(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	messageID, ok := parameter(c, "messageId")
	if !ok {
		return
	}
	var request chat.EditRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, err := h.service.Edit(c.Request.Context(), httpmiddleware.UserID(c), id, messageID, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// Delete removes the requested chat message using the caller's scope and the resource lifecycle
// rules.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	messageID, ok := parameter(c, "messageId")
	if !ok {
		return
	}
	item, err := h.service.Delete(c.Request.Context(), httpmiddleware.UserID(c), id, messageID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// ReadState returns the member's saved read boundary and visible unread-message count.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) ReadState(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	item, err := h.service.ReadState(c.Request.Context(), httpmiddleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// MarkRead advances the saved read boundary without allowing late or repeated requests to move
// it backward.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) MarkRead(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	var request chat.ReadRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, err := h.service.MarkRead(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// InitAttachment creates or reuses pending attachment metadata so an upload can be retried
// safely.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) InitAttachment(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	var request chat.InitRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, created, err := h.service.InitAttachment(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"status": "success", "item": item, "uploadUrl": "/api/v1/" + h.namespace + "/" + id + "/attachments/" + item.ID + "/content"})
}

// Upload validates a bounded content stream and stores it for the currently owned upload
// attempt.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Upload(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	attachmentID, ok := parameter(c, "attachmentId")
	if !ok {
		return
	}
	// Bound slow clients and input size. Uploads use a separate path from signaling and moderation;
	// the service also bounds concurrent in-memory uploads.
	controller := http.NewResponseController(c.Writer)
	_ = controller.SetReadDeadline(time.Now().Add(60 * time.Second))
	defer controller.SetReadDeadline(time.Time{})
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, chat.MaxAttachmentBytes+1)
	item, err := h.service.Upload(c.Request.Context(), httpmiddleware.UserID(c), id, attachmentID, c.Request.Body)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// FinalizeAttachment marks a successfully uploaded attachment ready to be referenced by a
// message.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) FinalizeAttachment(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	attachmentID, ok := parameter(c, "attachmentId")
	if !ok {
		return
	}
	if !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	item, err := h.service.FinalizeAttachment(c.Request.Context(), httpmiddleware.UserID(c), id, attachmentID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// Download checks attachment visibility and returns a short-lived private download URL.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Download(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	attachmentID, ok := parameter(c, "attachmentId")
	if !ok {
		return
	}
	if h.downloadGroup(c, id, attachmentID) {
		return
	}
	url, expires, err := h.service.Download(c.Request.Context(), httpmiddleware.UserID(c), id, attachmentID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"status": "success", "url": url, "expiresAt": expires})
}

// ForConversations preserves the shared HTTP/file flow with a distinct public scope.
//
// Returns:
//   - result 1: The constructed or loaded handler.
func (h *Handler) ForConversations() *Handler { h.namespace = "conversations"; return h }
