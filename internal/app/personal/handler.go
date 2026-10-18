package personalapp

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/personal"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	personalusecase "github.com/janickiy/meet-space/internal/usecase/personal"
	"strconv"
)

type Repository interface {
	GroupRepository
	// Account checks that the acting identity is a registered non-guest account.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Account(context.Context, string) error
	// GetOrCreate returns the existing direct conversation or creates its unique account-pair
	// conversation.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Target state, account or decoded value, as defined by its type.
	//
	// Returns:
	//   - result 1: The resulting conversation.
	//   - result 2: Whether this request created new state rather than reusing an idempotent result.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	GetOrCreate(context.Context, string, string) (personal.Conversation, bool, error)
	// Get loads the requested conversation within the caller's authorized scope.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting conversation.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Get(context.Context, string, string) (personal.Conversation, error)
	// List returns a bounded page of conversation records using the supplied pagination and filters.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Opaque continuation boundary from the previous page.
	//   - argument 4: Maximum number of results or operations in this batch.
	//
	// Returns:
	//   - result 1: The resulting page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	List(context.Context, string, string, int) (personal.Page, error)
	// ListFiltered returns a member-scoped conversation page using the validated personal, group or
	// unread filters.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Opaque continuation boundary from the previous page.
	//   - argument 4: Maximum number of results or operations in this batch.
	//   - argument 5: Validated filters restricting the result set.
	//
	// Returns:
	//   - result 1: The resulting page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ListFiltered(context.Context, string, string, int, personal.ListFilter) (personal.Page, error)
	// Search searches personal and group conversations using bounded input and the caller's
	// visibility filters.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Search text or validated query filters.
	//
	// Returns:
	//   - result 1: The collected peer values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Search(context.Context, string, string) ([]personal.Peer, error)
	// SetConversationPreferences updates only the acting member's conversation notification
	// preference.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Whether the requested feature or preference should be enabled.
	//
	// Returns:
	//   - result 1: The resulting conversation.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SetConversationPreferences(context.Context, string, string, bool) (personal.Conversation, error)
	// ClearDirectHistory moves only the acting member's direct-chat history boundary without
	// deleting the peer's history.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting conversation.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ClearDirectHistory(context.Context, string, string) (personal.Conversation, error)
	// HideDirectConversation hides the direct conversation for the acting account and retains the
	// peer's saved history.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting count, size or numeric value.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	HideDirectConversation(context.Context, string, string) (int64, error)
	// ConversationPins returns the authorized conversation's pinned-message page.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Opaque continuation boundary from the previous page.
	//   - argument 5: Maximum number of results or operations in this batch.
	//
	// Returns:
	//   - result 1: The resulting message page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ConversationPins(context.Context, string, string, string, int) (chat.MessagePage, error)
	// SetConversationPin pins or unpins a visible message and returns the conversation scope
	// affected by the change.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Message identifier within the authorized conversation.
	//   - argument 5: Whether the selected message should be pinned.
	//
	// Returns:
	//   - result 1: Identifier of the conversation whose pin state changed.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SetConversationPin(context.Context, string, string, string, bool) (string, error)
}
type Handler struct {
	Repo     Repository
	Events   *personalusecase.Events
	Presence personalusecase.PresenceReader
}

// uuidParameter normalizes a route UUID and writes the existing error response
// when validation fails, before any repository operation can run.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - name: Name of the setting, cookie, resource or provider being addressed.
//
// Returns:
//   - result 1: Validated nonzero resource UUID.
//   - result 2: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func uuidParameter(c *gin.Context, name string) (string, bool) {
	id, err := chat.UUID(c.Param(name))
	if err != nil {
		httpresponse.Fail(c, err)
		return "", false
	}
	return id, true
}

// AccountOnly rejects guest identities before serving registered-account-only conversation or
// folder operations.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) AccountOnly(c *gin.Context) {
	if err := h.Repo.Account(c.Request.Context(), middleware.UserID(c)); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Next()
}

// Create validates the request and returns the newly created conversation for the acting
// account.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Create(c *gin.Context) {
	var body struct {
		UserID string `json:"userId"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	id, err := chat.UUID(body.UserID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	item, created, err := h.Repo.GetOrCreate(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if created && h.Events != nil {
		_ = h.Events.PublishConversation(c.Request.Context(), item.ID, "conversation.updated", gin.H{"conversationId": item.ID, "type": "direct"})
	}
	status := 200
	if created {
		status = 201
	}
	c.JSON(status, gin.H{"status": "success", "item": item})
}

// List returns a bounded page of conversation records using the supplied pagination and filters.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) List(c *gin.Context) {
	limit := 50
	if raw, exists := c.GetQuery("limit"); exists {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		limit = n
	}
	filter := personal.ListFilter{Type: c.Query("type"), Search: c.Query("search")}
	if raw, present := c.GetQuery("unreadOnly"); present {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		filter.UnreadOnly = value
	}
	page, err := h.Repo.ListFiltered(c.Request.Context(), middleware.UserID(c), c.Query("before"), limit, filter)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, page)
}

// Get serves the requested conversation within the caller's authorized scope.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Get(c *gin.Context) {
	id, err := chat.UUID(c.Param("id"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	item, err := h.Repo.Get(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success", "item": item})
}

// Search searches personal and group conversations using bounded input and the caller's
// visibility filters.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Search(c *gin.Context) {
	items, err := h.Repo.Search(c.Request.Context(), middleware.UserID(c), c.Query("search"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success", "items": items})
}
