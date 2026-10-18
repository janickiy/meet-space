package calls

import (
	"context"
	"log/slog"
	"time"

	domain "github.com/janickiy/meet-space/internal/domain/calls"
	"github.com/janickiy/meet-space/internal/domain/realtime"
)

type Repository interface {
	// Create creates and persists a new direct call for the acting account.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Target state, account or decoded value, as defined by its type.
	//   - argument 4: Validated application request or incoming operation parameters.
	//   - argument 5: Browser media-client identity or transport dependency, as defined by its type.
	//
	// Returns:
	//   - result 1: The resulting call.
	//   - result 2: Whether this request created new state rather than reusing an idempotent result.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Create(context.Context, string, string, string, string) (domain.Call, bool, error)
	// Get loads the requested direct call within the caller's authorized scope.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting call.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Get(context.Context, string, string) (domain.Call, error)
	// Active returns the account's current nonterminal direct call, if any.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//
	// Returns:
	//   - result 1: The constructed or loaded call.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Active(context.Context, string) (*domain.Call, error)
	// Change applies the requested direct-call lifecycle action and returns its updated state.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Requested lifecycle or moderation action.
	//   - argument 5: Browser media-client identity or transport dependency, as defined by its type.
	//
	// Returns:
	//   - result 1: The resulting call.
	//   - result 2: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Change(context.Context, string, string, string, string) (domain.Call, bool, error)
	// Expiring returns the direct calls eligible for timeout processing.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: The collected string values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Expiring(context.Context) ([]string, error)
}
type Bus interface {
	// Publish publishes the saved direct call change through its configured event or command
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
type Observer interface {
	// ConferenceChanged notifies connected sessions or observers of the conference's committed state
	// change.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	ConferenceChanged(context.Context, string)
}
type Service struct {
	Repo     Repository
	Bus      Bus
	Observer Observer
}

// notify publishes the committed direct-call change to its authorized participants.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - c: The call value processed by this operation.
func (s *Service) notify(ctx context.Context, c domain.Call) {
	event := realtime.Event("call.updated", "", map[string]any{"call": c})
	// Pubsub is an acceleration only: every client also restores authoritative
	// state after reconnect and periodically while a call is displayed.
	for _, user := range []string{c.CallerUserID, c.CalleeUserID} {
		if s.Bus != nil {
			if err := s.Bus.Publish(ctx, user, event); err != nil {
				slog.Warn("call delivery failed", "call_id", c.ID, "error", err)
			}
			_ = s.Bus.Publish(ctx, user, realtime.Event("conversation.updated", "", map[string]any{"conversationId": c.ConversationID, "type": "direct"}))
		}
	}
	if c.ConferenceID != nil && s.Observer != nil {
		s.Observer.ConferenceChanged(ctx, *c.ConferenceID)
	}
}

// Create creates and persists a new direct call for the acting account.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - actor: Identifier of the account performing the operation.
//   - target: Target state, account or decoded value, as defined by its type.
//   - request: Client-generated idempotency key identifying this direct-call request.
//   - client: Browser media-client identity or transport dependency, as defined by its type.
//
// Returns:
//   - result 1: The resulting call.
//   - result 2: Whether this request created new state rather than reusing an idempotent result.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Create(ctx context.Context, actor, target, request, client string) (domain.Call, bool, error) {
	c, created, err := s.Repo.Create(ctx, actor, target, request, client)
	if err == nil && created {
		s.notify(ctx, c)
	}
	return c, created, err
}

// Get loads the requested direct call within the caller's authorized scope.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - actor: Identifier of the account performing the operation.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting call.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Get(ctx context.Context, actor, id string) (domain.Call, error) {
	c, err := s.Repo.Get(ctx, actor, id)
	if err != nil || !c.Expired(time.Now().UTC()) {
		return c, err
	}
	return s.Change(ctx, actor, id, "expire", "")
}

// Active returns the account's current nonterminal direct call, if any.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - actor: Identifier of the account performing the operation.
//
// Returns:
//   - result 1: The constructed or loaded call.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Active(ctx context.Context, actor string) (*domain.Call, error) {
	c, err := s.Repo.Active(ctx, actor)
	if err != nil || c == nil || !c.Expired(time.Now().UTC()) {
		return c, err
	}
	_, err = s.Change(ctx, actor, c.ID, "expire", "")
	return nil, err
}

// Change applies the requested direct-call lifecycle action and returns its updated state.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - actor: Identifier of the account performing the operation.
//   - id: Identifier of the resource addressed by this operation.
//   - action: Requested lifecycle or moderation action.
//   - client: Browser media-client identity or transport dependency, as defined by its type.
//
// Returns:
//   - result 1: The resulting call.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Change(ctx context.Context, actor, id, action, client string) (domain.Call, error) {
	c, changed, err := s.Repo.Change(ctx, actor, id, action, client)
	if err == nil && changed {
		s.notify(ctx, c)
	}
	return c, err
}

// Run runs direct-call lifecycle management until the context is canceled or processing fails.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		op, cancel := context.WithTimeout(ctx, 10*time.Second)
		ids, err := s.Repo.Expiring(op)
		if err != nil && ctx.Err() == nil {
			slog.Warn("call expiry failed", "error", err)
		}
		for _, id := range ids {
			if _, err := s.Change(op, "", id, "expire", ""); err != nil && ctx.Err() == nil {
				slog.Warn("call expiry failed", "call_id", id, "error", err)
			}
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
