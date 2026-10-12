package chatapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/chat"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

type SearchService interface {
	Search(context.Context, string, string, string, string, int) (chat.Page, error)
	Context(context.Context, string, string, string) (chat.Page, error)
}

type SearchHandler struct{ Service SearchService }

func (h *SearchHandler) Search(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	limit, ok := actionPage(c)
	if !ok {
		return
	}
	page, err := h.Service.Search(c.Request.Context(), httpmiddleware.UserID(c), id, c.Query("q"), c.Query("before"), limit)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	searchPageResponse(c, page)
}

func (h *SearchHandler) Context(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	messageID, ok := parameter(c, "messageId")
	if !ok {
		return
	}
	page, err := h.Service.Context(c.Request.Context(), httpmiddleware.UserID(c), id, messageID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	searchPageResponse(c, page)
}

func searchPageResponse(c *gin.Context, page chat.Page) {
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "nextCursor": page.NextCursor, "unreadCount": page.UnreadCount, "lastReadMessageId": page.LastReadMessageID})
}
