package personalapp

import (
	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/personal"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// publishConversationAction publishes a committed personal conversation action to the intended
// account or group recipients.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - kind: Validated event, job, media or resource category.
//   - id: Identifier of the resource addressed by this operation.
//   - item: The conversation state processed by this operation.
//   - cutoff: Member-specific history or liveness boundary.
func (h *Handler) publishConversationAction(c *gin.Context, kind, id string, item *personal.Conversation, cutoff int64) {
	if h.Events == nil {
		return
	}
	data := gin.H{"conversationId": id, "type": "direct", "userId": middleware.UserID(c)}
	if item != nil {
		data["type"] = item.Type
		data["item"] = *item
		data["historyClearedThrough"] = item.HistoryClearedThrough
	} else if kind == "conversation.hidden" {
		data["historyClearedThrough"] = cutoff
		data["hidden"] = true
	}
	_ = h.Events.PublishUser(c.Request.Context(), middleware.UserID(c), kind, data)
}

// SetConversationPreferences updates only the acting member's conversation notification
// preference.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) SetConversationPreferences(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	var body struct {
		NotificationsEnabled *bool `json:"notificationsEnabled"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	if body.NotificationsEnabled == nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	item, err := h.Repo.SetConversationPreferences(c.Request.Context(), middleware.UserID(c), id, *body.NotificationsEnabled)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.publishConversationAction(c, "conversation.preferences.updated", id, &item, item.HistoryClearedThrough)
	c.JSON(200, gin.H{"status": "success", "item": item})
}

// ClearDirectHistory moves only the acting member's direct-chat history boundary without
// deleting the peer's history.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) ClearDirectHistory(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	item, err := h.Repo.ClearDirectHistory(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.publishConversationAction(c, "conversation.history.cleared", id, &item, item.HistoryClearedThrough)
	c.JSON(200, gin.H{"status": "success", "item": item})
}

// HideDirectConversation hides the direct conversation for the acting account and retains the
// peer's saved history.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) HideDirectConversation(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	cutoff, err := h.Repo.HideDirectConversation(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.publishConversationAction(c, "conversation.hidden", id, nil, cutoff)
	c.JSON(200, gin.H{"status": "success", "hidden": true, "historyClearedThrough": cutoff})
}
