package personalapp

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/personal"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	personalusecase "github.com/janickiy/meet-space/internal/usecase/personal"
)

type GroupRepository interface {
	// CreateGroup creates a group conversation with validated registered members and an owner.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting conversation.
	//   - result 2: Whether this request created new state rather than reusing an idempotent result.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	CreateGroup(context.Context, string, personal.CreateGroupRequest) (personal.Conversation, bool, error)
	// UpdateGroup updates the group's validated metadata after checking management permission.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting conversation.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	UpdateGroup(context.Context, string, string, personal.UpdateGroupRequest) (personal.Conversation, error)
	// GroupMembers returns the group members visible to the acting account.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The collected member values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	GroupMembers(context.Context, string, string) ([]personal.Member, error)
	// AddGroupMembers adds validated registered accounts after checking the group membership and
	// management rules.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Untrusted member identifiers to validate and deduplicate.
	//
	// Returns:
	//   - result 1: The resulting conversation.
	//   - result 2: The collected member values.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	AddGroupMembers(context.Context, string, string, []string) (personal.Conversation, []personal.Member, error)
	// RemoveGroupMember removes a group member while enforcing ownership and management
	// restrictions.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Target state, account or decoded value, as defined by its type.
	//
	// Returns:
	//   - result 1: The resulting conversation.
	//   - result 2: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	RemoveGroupMember(context.Context, string, string, string) (personal.Conversation, bool, error)
	// ChangeGroupRole changes a group member's role within the acting manager's authority.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Target state, account or decoded value, as defined by its type.
	//   - argument 5: Validated role to assign to the target group member.
	//
	// Returns:
	//   - result 1: The resulting conversation.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ChangeGroupRole(context.Context, string, string, string, personal.Role) (personal.Conversation, error)
	// TransferGroupOwnership atomically transfers group ownership to an eligible current member.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Target state, account or decoded value, as defined by its type.
	//
	// Returns:
	//   - result 1: The resulting conversation.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	TransferGroupOwnership(context.Context, string, string, string) (personal.Conversation, error)
	// LeaveGroup leaves a group only when its ownership rules permit the acting member to exit.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	LeaveGroup(context.Context, string, string) error
	// DeleteGroup deletes the group for its authorized owner and returns the affected recipients for
	// event delivery.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The collected string values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	DeleteGroup(context.Context, string, string) ([]string, error)
}

// groupResult writes the public group-conversation response after a successful operation.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - item: The conversation state processed by this operation.
func groupResult(c *gin.Context, item personal.Conversation) {
	c.JSON(200, gin.H{"status": "success", "item": item})
}

// groupEvent publishes a committed group metadata or membership change with its bounded event
// payload.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//   - id: Identifier of the resource addressed by this operation.
//   - kind: Validated event, job, media or resource category.
//   - user: Acting account identifier used to scope access.
//   - role: Requested participant or group-member role.
//   - extra: Additional bounded event fields or callbacks.
func (h *Handler) groupEvent(c *gin.Context, id, kind, user string, role personal.Role, extra ...string) {
	if h.Events == nil {
		return
	}
	data := gin.H{"conversationId": id, "type": "group"}
	if user != "" {
		data["userId"] = user
	}
	if role != "" {
		data["role"] = role
	}
	_ = h.Events.PublishConversationTo(c.Request.Context(), id, kind, data, extra...)
}

// CreateGroup creates a group conversation with validated registered members and an owner.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) CreateGroup(c *gin.Context) {
	var body personal.CreateGroupRequest
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	actor := middleware.UserID(c)
	item, created, err := h.Repo.CreateGroup(c.Request.Context(), actor, body)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if created {
		h.groupEvent(c, item.ID, "conversation.member.added", actor, personal.Owner)
	}
	status := 200
	if created {
		status = 201
	}
	c.JSON(status, gin.H{"status": "success", "item": item})
}

// UpdateGroup updates the group's validated metadata after checking management permission.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) UpdateGroup(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	var body personal.UpdateGroupRequest
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	item, err := h.Repo.UpdateGroup(c.Request.Context(), middleware.UserID(c), id, body)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.groupEvent(c, id, "conversation.updated", "", "")
	groupResult(c, item)
}

// GroupMembers returns the group members visible to the acting account.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) GroupMembers(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	items, err := personalusecase.ReadGroupMembers(c.Request.Context(), h.Repo, h.Presence, middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success", "items": items})
}

// AddGroupMembers adds validated registered accounts after checking the group membership and
// management rules.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) AddGroupMembers(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	var body struct {
		UserIDs []string `json:"userIds"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	item, added, err := h.Repo.AddGroupMembers(c.Request.Context(), middleware.UserID(c), id, body.UserIDs)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	for _, member := range added {
		h.groupEvent(c, id, "conversation.member.added", member.ID, member.Role)
	}
	groupResult(c, item)
}

// RemoveGroupMember removes a group member while enforcing ownership and management
// restrictions.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) RemoveGroupMember(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	target, ok := uuidParameter(c, "userId")
	if !ok {
		return
	}
	item, changed, err := h.Repo.RemoveGroupMember(c.Request.Context(), middleware.UserID(c), id, target)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if changed {
		h.groupEvent(c, id, "conversation.member.removed", target, "", target)
	}
	groupResult(c, item)
}

// ChangeGroupRole changes a group member's role within the acting manager's authority.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) ChangeGroupRole(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	target, ok := uuidParameter(c, "userId")
	if !ok {
		return
	}
	var body struct {
		Role personal.Role `json:"role"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	item, err := h.Repo.ChangeGroupRole(c.Request.Context(), middleware.UserID(c), id, target, body.Role)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.groupEvent(c, id, "conversation.member.updated", target, body.Role)
	groupResult(c, item)
}

// TransferGroupOwnership atomically transfers group ownership to an eligible current member.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) TransferGroupOwnership(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	var body struct {
		UserID string `json:"userId"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	target, err := chat.UUID(body.UserID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	actor := middleware.UserID(c)
	item, err := h.Repo.TransferGroupOwnership(c.Request.Context(), actor, id, target)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.groupEvent(c, id, "conversation.member.updated", actor, personal.Admin)
	h.groupEvent(c, id, "conversation.member.updated", target, personal.Owner)
	groupResult(c, item)
}

// LeaveGroup leaves a group only when its ownership rules permit the acting member to exit.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) LeaveGroup(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	var body struct{}
	if !httpresponse.BindJSON(c, &body, true) {
		return
	}
	actor := middleware.UserID(c)
	if err := h.Repo.LeaveGroup(c.Request.Context(), actor, id); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.groupEvent(c, id, "conversation.member.removed", actor, "", actor)
	c.JSON(200, gin.H{"status": "success"})
}

// DeleteGroup deletes the group for its authorized owner and returns the affected recipients for
// event delivery.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) DeleteGroup(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	former, err := h.Repo.DeleteGroup(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	for _, user := range former {
		h.groupEvent(c, id, "conversation.member.removed", user, "", user)
	}
	c.JSON(200, gin.H{"status": "success"})
}
