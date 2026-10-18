package foldersapp

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/folders"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

type Repository interface {
	// Account checks that the acting identity is a registered non-guest account.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Account identifier that must belong to a registered non-guest user.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Account(context.Context, string) error
	// List returns a bounded page of private folder records using the supplied pagination and
	// filters.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Validated kind of resource linked to a folder.
	//   - argument 4: Identifier of the resource linked to a folder.
	//
	// Returns:
	//   - result 1: The collected folder values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	List(context.Context, string, string, string) ([]folders.Folder, error)
	// Create creates and persists a new private folder for the acting account.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Name of the setting, cookie, resource or provider being addressed.
	//
	// Returns:
	//   - result 1: The resulting folder.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Create(context.Context, string, string) (folders.Folder, error)
	// Get loads the requested private folder within the caller's authorized scope.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to check folder ownership.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting folder.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Get(context.Context, string, string) (folders.Folder, error)
	// Rename normalizes a new folder name and updates the acting account's owned folder.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Name of the setting, cookie, resource or provider being addressed.
	//
	// Returns:
	//   - result 1: The resulting folder.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Rename(context.Context, string, string, string) (folders.Folder, error)
	// Delete removes the requested private folder using the caller's scope and the resource
	// lifecycle rules.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Delete(context.Context, string, string) error
	// Order stores a validated ordering of the acting account's private folders.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Resource identifiers to process as a bounded batch.
	//
	// Returns:
	//   - result 1: The collected folder values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Order(context.Context, string, []string) ([]folders.Folder, error)
	// SetItem updates item for private folders.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Identifier of the acting account's private folder.
	//   - argument 4: Validated event, job, media or resource category.
	//   - argument 5: Identifier of the resource addressed by this operation.
	//   - argument 6: Whether to add rather than remove the target item.
	//
	// Returns:
	//   - result 1: The resulting folder.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SetItem(context.Context, string, string, string, string, bool) (folders.Folder, error)
	// Items returns a filtered, bounded page of resources linked to an owned folder.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Identifier of the acting account's private folder.
	//   - argument 4: Opaque continuation boundary from the previous page.
	//   - argument 5: Maximum number of results or operations in this batch.
	//   - argument 6: Validated filters restricting the result set.
	//
	// Returns:
	//   - result 1: The resulting page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Items(context.Context, string, string, string, int, folders.Filter) (folders.Page, error)
	// Candidates returns accessible resources that can be linked to the acting account's folder.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Identifier of the acting account's private folder.
	//   - argument 4: Opaque continuation boundary from the previous page.
	//   - argument 5: Maximum number of results or operations in this batch.
	//   - argument 6: Validated filters restricting the result set.
	//
	// Returns:
	//   - result 1: The resulting page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Candidates(context.Context, string, string, string, int, folders.Filter) (folders.Page, error)
}
type Emitter interface {
	// PublishFolder publishes a committed folder change only to the owning account's realtime
	// channel.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Validated event, job, media or resource category.
	//   - argument 4: Identifier of the acting account's private folder.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	PublishFolder(context.Context, string, string, string) error
}
type Handler struct {
	Repo   Repository
	Events Emitter
}

// NewHandler constructs the handler with the dependencies and limits required for private
// folders.
//
// Parameters:
//   - repo: Persistence dependency used by the application operation.
//   - events: Destination for committed lifecycle or chat events.
//
// Returns:
//   - result 1: The constructed or loaded handler.
func NewHandler(repo Repository, events Emitter) *Handler {
	return &Handler{Repo: repo, Events: events}
}

// emit sends a trusted lifecycle event through the component-owned bounded delivery path.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - kind: Validated event, job, media or resource category.
//   - id: Identifier of the resource addressed by this operation.
func (h *Handler) emit(c *gin.Context, kind, id string) {
	if h.Events != nil {
		_ = h.Events.PublishFolder(c.Request.Context(), middleware.UserID(c), kind, id)
	}
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

// parameter validates a required route parameter before invoking the application operation.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - name: Name of the setting, cookie, resource or provider being addressed.
//
// Returns:
//   - result 1: Validated required route parameter.
//   - result 2: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func parameter(c *gin.Context, name string) (string, bool) {
	id, err := chat.UUID(c.Param(name))
	if err != nil {
		httpresponse.Fail(c, err)
		return "", false
	}
	return id, true
}

// result writes the folder operation response using the agreed status and public folder
// representation.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - status: Lifecycle state or response status to record.
//   - item: The folder state processed by this operation.
func result(c *gin.Context, status int, item folders.Folder) {
	c.JSON(status, gin.H{"status": "success", "item": item})
}

// List returns a bounded page of private folder records using the supplied pagination and
// filters.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) List(c *gin.Context) {
	items, err := h.Repo.List(c.Request.Context(), middleware.UserID(c), c.Query("itemKind"), c.Query("itemId"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success", "items": items})
}

// Create validates the request and returns the newly created private folder for the acting
// account.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Create(c *gin.Context) {
	var body struct {
		Name string `json:"name"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	item, err := h.Repo.Create(c.Request.Context(), middleware.UserID(c), body.Name)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.emit(c, "folder.created", item.ID)
	result(c, 201, item)
}

// Get serves the requested private folder within the caller's authorized scope.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Get(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	item, err := h.Repo.Get(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	result(c, 200, item)
}

// Rename normalizes a new folder name and updates the acting account's owned folder.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Rename(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	item, err := h.Repo.Rename(c.Request.Context(), middleware.UserID(c), id, body.Name)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.emit(c, "folder.updated", id)
	result(c, 200, item)
}

// Delete removes the requested private folder using the caller's scope and the resource
// lifecycle rules.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	if err := h.Repo.Delete(c.Request.Context(), middleware.UserID(c), id); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.emit(c, "folder.deleted", id)
	c.JSON(200, gin.H{"status": "success"})
}

// Order stores a validated ordering of the acting account's private folders.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Order(c *gin.Context) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	if body.IDs == nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	items, err := h.Repo.Order(c.Request.Context(), middleware.UserID(c), body.IDs)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.emit(c, "folder.reordered", "")
	c.JSON(200, gin.H{"status": "success", "items": items})
}

// AddItem adds an accessible resource to the acting account's private folder.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) AddItem(c *gin.Context) { h.setItem(c, true) }

// RemoveItem removes a resource link from the acting account's private folder without deleting
// the resource.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) RemoveItem(c *gin.Context) { h.setItem(c, false) }

// setItem applies the add or remove operation for a validated private-folder item.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - add: Whether to add rather than remove the target item.
func (h *Handler) setItem(c *gin.Context, add bool) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	itemID, ok := parameter(c, "itemId")
	if !ok {
		return
	}
	kind := c.Param("kind")
	if !folders.ValidKind(kind) {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	item, err := h.Repo.SetItem(c.Request.Context(), middleware.UserID(c), id, kind, itemID, add)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.emit(c, "folder.items.updated", id)
	result(c, 200, item)
}

// pageLimit parses and validates the bounded folder-item page size.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func pageLimit(c *gin.Context) (int, bool) {
	limit := 50
	if raw, exists := c.GetQuery("limit"); exists {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return 0, false
		}
		limit = n
	}
	return limit, true
}

// Items returns a filtered, bounded page of resources linked to an owned folder.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Items(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	limit, ok := pageLimit(c)
	if !ok {
		return
	}
	page, err := h.Repo.Items(c.Request.Context(), middleware.UserID(c), id, c.Query("before"), limit, folders.Filter{Type: c.Query("type"), Search: c.Query("search")})
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, page)
}

// Candidates returns accessible resources that can be linked to the acting account's folder.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Candidates(c *gin.Context) {
	limit, ok := pageLimit(c)
	if !ok {
		return
	}
	page, err := h.Repo.Candidates(c.Request.Context(), middleware.UserID(c), c.Query("folderId"), c.Query("before"), limit, folders.Filter{Type: c.Query("type"), Search: c.Query("search")})
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, page)
}
