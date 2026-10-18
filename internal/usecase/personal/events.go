package personal

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/realtime"
)

type Members interface {
	// Members returns the current member identifiers or member views for the authorized
	// conversation.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The collected string values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Members(context.Context, string) ([]string, error)
}
type conversationTypes interface {
	// ConversationType returns whether the persisted conversation is direct or group-scoped.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: Persisted direct or group conversation kind.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ConversationType(context.Context, string) (string, error)
}
type Bus interface {
	// Publish publishes the saved conversation change through its configured event or command
	// transport.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Trusted event to publish or process.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Publish(context.Context, string, realtime.Envelope) error
}

// Events adapts the shared chat application to per-account delivery after persistence.
type Events struct {
	Members Members
	Bus     Bus
}

// Broadcast publishes a trusted event only to the currently authorized conference or
// conversation recipients.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - event: Trusted event to publish or process.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (e *Events) Broadcast(ctx context.Context, event realtime.Envelope) error {
	var m chat.Message
	if err := json.Unmarshal(event.Data, &m); err != nil {
		return err
	}
	kind, err := e.conversationType(ctx, m.ConversationID)
	if err != nil {
		return err
	}
	return e.PublishConversation(ctx, m.ConversationID, strings.TrimPrefix(event.Type, "chat."), map[string]any{"conversationId": m.ConversationID, "type": kind, "message": m})
}

// PublishConversation publishes a committed conversation change to its authorized current
// members.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//   - kind: Validated event, job, media or resource category.
//   - data: Input bytes or structured event payload.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (e *Events) PublishConversation(ctx context.Context, id, kind string, data any) error {
	return e.PublishConversationTo(ctx, id, kind, data)
}

// PublishUser delivers a personal preference or history change only to its owner's tabs; peer
// accounts do not receive private notification or history settings.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - kind: Validated event, job, media or resource category.
//   - data: Input bytes or structured event payload.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (e *Events) PublishUser(ctx context.Context, user, kind string, data any) error {
	if kind != "conversation.preferences.updated" && kind != "conversation.history.cleared" && kind != "conversation.hidden" {
		return errors.New("unsupported personal conversation event")
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var payload struct {
		ConversationID string `json:"conversationId"`
		Type           string `json:"type"`
		UserID         string `json:"userId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	id, err := uuid.Parse(payload.ConversationID)
	validType := payload.Type == "direct" || (payload.Type == "group" && kind == "conversation.preferences.updated")
	if user == "" || payload.UserID != user || !validType || err != nil || id == uuid.Nil || id.String() != payload.ConversationID {
		return errors.New("invalid personal conversation event payload")
	}
	return e.Bus.Publish(ctx, user, realtime.Event(kind, "", data))
}

// PublishConversationTo also delivers a minimal lifecycle event to a removed user.
// Callers persist first; extra recipients must never receive private messages.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//   - kind: Validated event, job, media or resource category.
//   - data: Input bytes or structured event payload.
//   - extraUsers: Additional authorized recipients included in the committed group-change event.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (e *Events) PublishConversationTo(ctx context.Context, id, kind string, data any, extraUsers ...string) error {
	ids, err := e.Members.Members(ctx, id)
	if err != nil {
		return err
	}
	if len(extraUsers) > 0 && kind != "conversation.member.removed" {
		return errors.New("extra recipients require a membership removal event")
	}
	ids = append(ids, extraUsers...)
	seen := make(map[string]struct{}, len(ids))
	event := realtime.Event(kind, "", data)
	for _, user := range ids {
		if _, exists := seen[user]; exists {
			continue
		}
		seen[user] = struct{}{}
		err = errors.Join(err, e.Bus.Publish(ctx, user, event))
	}
	return err
}

// SendToParticipant publishes a trusted event to the physical sessions of the selected
// authorized participant.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//   - user: Acting account identifier used to scope access.
//   - event: Trusted event to publish or process.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (e *Events) SendToParticipant(ctx context.Context, id, user string, event realtime.Envelope) error {
	var state chat.ReadState
	if err := json.Unmarshal(event.Data, &state); err != nil {
		return err
	}
	kind, err := e.conversationType(ctx, id)
	if err != nil {
		return err
	}
	return e.Bus.Publish(ctx, user, realtime.Event("conversation.read.updated", "", map[string]any{"conversationId": id, "type": kind, "state": state}))
}

// conversationType resolves the persisted conversation kind before selecting its event-delivery
// path.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: Persisted direct or group conversation kind.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (e *Events) conversationType(ctx context.Context, id string) (string, error) {
	if types, ok := e.Members.(conversationTypes); ok {
		return types.ConversationType(ctx, id)
	}
	// Compatibility for existing direct-only adapters.
	return "direct", nil
}
