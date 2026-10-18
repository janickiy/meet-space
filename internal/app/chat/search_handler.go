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
	// Search searches persistent chat messages and private attachments using bounded input and the
	// caller's visibility filters.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference or conversation identifier restricting the search.
	//   - argument 4: Search text or validated query filters.
	//   - argument 5: Opaque continuation boundary from the previous page.
	//   - argument 6: Maximum number of results or operations in this batch.
	//
	// Returns:
	//   - result 1: The resulting page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Search(context.Context, string, string, string, string, int) (chat.Page, error)
	// Context returns the visible messages surrounding a selected message so clients can navigate to
	// a search result.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference or conversation identifier containing the selected message.
	//   - argument 4: Message identifier within the authorized conversation.
	//
	// Returns:
	//   - result 1: The resulting page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Context(context.Context, string, string, string) (chat.Page, error)
}

type SearchHandler struct{ Service SearchService }

// Search searches persistent chat messages and private attachments using bounded input and the
// caller's visibility filters.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
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

// Context returns the visible messages surrounding a selected message so clients can navigate to
// a search result.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
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

// searchPageResponse writes a message-search page with the response metadata expected by the
// client.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - page: Page of results to format or process.
func searchPageResponse(c *gin.Context, page chat.Page) {
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "nextCursor": page.NextCursor, "unreadCount": page.UnreadCount, "lastReadMessageId": page.LastReadMessageID})
}
