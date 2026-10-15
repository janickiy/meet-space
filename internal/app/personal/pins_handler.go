package personalapp

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// Pins lists shared pins using the current member's visible message history.
func (h *Handler) Pins(c *gin.Context) {
	id, ok := directActionID(c)
	if !ok {
		return
	}
	limit := 50
	if raw, present := c.GetQuery("limit"); present {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		limit = value
	}
	page, err := h.Repo.ConversationPins(c.Request.Context(), middleware.UserID(c), id, c.Query("before"), limit)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

// SetPin persists shared pin state before telling current members to refresh it.
// The event contains no message ID, text or preview from another member's history.
func (h *Handler) SetPin(pinned bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := directActionID(c)
		if !ok {
			return
		}
		messageID, err := chat.UUID(c.Param("messageId"))
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		kind, err := h.Repo.SetConversationPin(c.Request.Context(), middleware.UserID(c), id, messageID, pinned)
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		if h.Events != nil {
			_ = h.Events.PublishConversation(c.Request.Context(), id, "conversation.pins.updated", gin.H{"conversationId": id, "type": kind})
		}
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	}
}
