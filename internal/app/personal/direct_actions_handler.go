package personalapp

import (
	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/personal"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// publishConversationAction публикует зафиксированное действие по ведению личной переписки для указанных получателей.
// учетная запись или группа получателей.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//   - kind: валидированное событие, задание, медиафайл или категория ресурса.
//   - id: идентификатор ресурса, к которому относится эта операция.
//   - item: состояние переписки, обработанное в ходе этой операции.
//   - cutoff: граница истории или активности конкретного участника.
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

// SetConversationPreferences обновляет только уведомления о переписке текущего участника.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
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

// ClearDirectHistory перемещает только границу истории прямого чата действующего участника, не
// удаляя историю собеседника.
//
// @Parameters:
//   - c: Контекст запроса и ответа Gin, включая аутентифицированные идентификационные данные.
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

// HideDirectConversation скрывает личный диалог для текущего аккаунта,
// сохраняя историю переписки собеседника.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
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
