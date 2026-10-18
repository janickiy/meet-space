package chatapp

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// GroupDownloads replaces bearer-capability URLs for groups only. The returned
// URL requires authentication again when its bytes are requested.
type GroupDownloads interface {
	// GroupDownload authorizes the group attachment and returns a bounded private download target.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conversation identifier limiting the operation scope.
	//   - argument 4: Private attachment identity or metadata, as defined by its type.
	//
	// Returns:
	//   - result 1: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 2: Temporary authorized attachment download URL.
	//   - result 3: The timestamp or validity boundary associated with the result.
	//   - result 4: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	GroupDownload(context.Context, string, string, string) (bool, string, time.Time, error)
}

// WithGroupDownloads attaches group downloads to the component before its operations are served.
//
// Parameters:
//   - provider: Configured external-provider adapter.
//
// Returns:
//   - result 1: The constructed or loaded handler.
func (h *Handler) WithGroupDownloads(provider GroupDownloads) *Handler {
	h.groupDownloads = provider
	return h
}

// downloadGroup serves the group-attachment download path when this repository scope belongs to
// a group.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - scope: Authorized conversation or resource scope.
//   - attachment: Private attachment identity or metadata, as defined by its type.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (h *Handler) downloadGroup(c *gin.Context, scope, attachment string) bool {
	if h.namespace != "conversations" || h.groupDownloads == nil {
		return false
	}
	handled, url, expires, err := h.groupDownloads.GroupDownload(c.Request.Context(), middleware.UserID(c), scope, attachment)
	if !handled {
		return false
	}
	if err != nil {
		httpresponse.Fail(c, err)
		return true
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"status": "success", "url": url, "expiresAt": expires, "authenticated": true})
	return true
}
