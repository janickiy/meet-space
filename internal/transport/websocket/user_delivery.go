package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/personal"
	domain "github.com/janickiy/meet-space/internal/domain/realtime"
)

type conversationAccessChecker interface {
	// HasActiveMembership checks the account's current conversation membership immediately before
	// protected access or delivery.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Acting account identifier used to scope access.
	//
	// Returns:
	//   - result 1: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	HasActiveMembership(context.Context, string, string) (bool, error)
}

// A delivery guard serializes the bounded socket write with member revocation.
// Concurrent deliveries take a shared lock; membership changes take an exclusive
// lock. A completed removal therefore cannot be followed by a queued message.
type conversationDeliveryGuard interface {
	// WithConversationDelivery attaches conversation delivery to the component before its operations
	// are served.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Acting account identifier used to scope access.
	//   - argument 4: Delivery callback invoked only while the recipient or resource authorization
	//     remains valid.
	//
	// Returns:
	//   - result 1: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	WithConversationDelivery(context.Context, string, string, func() error) (bool, error)
}

// conversationMessageDeliveryGuard prevents a delayed queued message from bypassing history
// clearing and supplies the recipient's current personal preferences.
type conversationMessageDeliveryGuard interface {
	// WithConversationMessageDelivery attaches conversation message delivery to the component before
	// its operations are served.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Acting account identifier used to scope access.
	//   - argument 4: Message identifier within the authorized conversation.
	//   - argument 5: Delivery callback invoked only while the recipient or resource authorization
	//     remains valid.
	//
	// Returns:
	//   - result 1: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	WithConversationMessageDelivery(context.Context, string, string, string, func(bool, int64) error) (bool, error)
}

// conversationActionDeliveryGuard replaces delayed personal payloads with the owner's current
// view and rejects hiding a conversation that has already been restored.
type conversationActionDeliveryGuard interface {
	// WithConversationActionDelivery attaches conversation action delivery to the component before
	// its operations are served.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Acting account identifier used to scope access.
	//   - argument 4: Validated event, job, media or resource category.
	//   - argument 5: Delivery callback invoked only while the recipient or resource authorization
	//     remains valid.
	//
	// Returns:
	//   - result 1: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	WithConversationActionDelivery(context.Context, string, string, string, func(*personal.Conversation) error) (bool, error)
}

// deliver checks the current recipient authorization immediately before the saved event or
// command is delivered.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - data: Input bytes or structured event payload.
//   - write: Callback that writes the authorized serialized event to the receiving socket.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *UserHandler) deliver(ctx context.Context, user string, data []byte, write func([]byte) error) error {
	var event domain.Envelope
	if json.Unmarshal(data, &event) != nil {
		return nil
	}
	if !strings.HasPrefix(event.Type, "message.") && !strings.HasPrefix(event.Type, "conversation.") {
		return write(data)
	}
	var payload struct {
		ConversationID string        `json:"conversationId"`
		Type           string        `json:"type"`
		UserID         string        `json:"userId"`
		Message        *chat.Message `json:"message"`
	}
	if json.Unmarshal(event.Data, &payload) != nil || (payload.Type != "direct" && payload.Type != "group") {
		return nil
	}
	conversation, err := uuid.Parse(payload.ConversationID)
	if err != nil || conversation == uuid.Nil || conversation.String() != payload.ConversationID {
		return nil
	}
	// A revoked user needs only this lifecycle event to discard local caches.
	// Reject a purported revoke carrying metadata/messages before bypassing auth.
	if event.Type == "conversation.member.removed" && payload.UserID == user {
		var fields map[string]json.RawMessage
		if json.Unmarshal(event.Data, &fields) != nil {
			return nil
		}
		for key := range fields {
			if key != "conversationId" && key != "type" && key != "userId" && key != "role" {
				return nil
			}
		}
		return write(data)
	}
	if event.Type == "conversation.preferences.updated" || event.Type == "conversation.history.cleared" || event.Type == "conversation.hidden" {
		// Never publish personal preferences or history clearing to the conversation peer.
		if payload.UserID != user || (payload.Type == "group" && event.Type != "conversation.preferences.updated") {
			return nil
		}
		if guard, ok := h.accounts.(conversationActionDeliveryGuard); ok {
			_, err := guard.WithConversationActionDelivery(ctx, payload.ConversationID, user, event.Type, func(item *personal.Conversation) error {
				if item == nil {
					return errors.New("missing authoritative conversation state")
				}
				fields := map[string]any{"conversationId": payload.ConversationID, "type": item.Type, "userId": user}
				fields["historyClearedThrough"] = item.HistoryClearedThrough
				if event.Type == "conversation.hidden" {
					fields["hidden"] = true
				} else {
					fields["item"] = *item
				}
				var err error
				event.Data, err = json.Marshal(fields)
				if err != nil {
					return err
				}
				outbound, err := json.Marshal(event)
				if err != nil {
					return err
				}
				return write(outbound)
			})
			return err
		}
	}
	if strings.HasPrefix(event.Type, "message.") {
		if guard, ok := h.accounts.(conversationMessageDeliveryGuard); ok {
			if payload.Message == nil || payload.Message.ConversationID != payload.ConversationID {
				return nil
			}
			messageID, err := uuid.Parse(payload.Message.ID)
			if err != nil || messageID == uuid.Nil || messageID.String() != payload.Message.ID {
				return nil
			}
			_, err = guard.WithConversationMessageDelivery(ctx, payload.ConversationID, user, payload.Message.ID, func(notificationsEnabled bool, cutoff int64) error {
				message := *payload.Message
				if cutoff > 0 && message.ReplyPreview != nil && message.ReplyPreview.Sequence <= cutoff {
					message.ReplyPreview = nil
					message.ReplyTo = nil
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(event.Data, &fields); err != nil {
					return err
				}
				fields["message"], err = json.Marshal(message)
				if err != nil {
					return err
				}
				fields["notificationsEnabled"], err = json.Marshal(notificationsEnabled)
				if err != nil {
					return err
				}
				fields["historyClearedThrough"], err = json.Marshal(cutoff)
				if err != nil {
					return err
				}
				event.Data, err = json.Marshal(fields)
				if err != nil {
					return err
				}
				outbound, err := json.Marshal(event)
				if err != nil {
					return err
				}
				return write(outbound)
			})
			return err
		}
	}
	if guard, ok := h.accounts.(conversationDeliveryGuard); ok {
		_, err := guard.WithConversationDelivery(ctx, payload.ConversationID, user, func() error { return write(data) })
		return err
	}
	if checker, ok := h.accounts.(conversationAccessChecker); ok {
		allowed, err := checker.HasActiveMembership(ctx, payload.ConversationID, user)
		if err != nil || !allowed {
			return err
		}
		return write(data)
	}
	// Compatibility for direct-only test/adapters; group delivery fails closed.
	if payload.Type == "direct" {
		return write(data)
	}
	return nil
}
