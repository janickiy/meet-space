package notifications

import (
	"context"
	"log/slog"
	"time"

	domain "github.com/janickiy/meet-space/internal/domain/notifications"
	realtime "github.com/janickiy/meet-space/internal/domain/realtime"
)

// Repository defines the replaceable dependency contract for personal notification delivery.
type Repository interface {
	// List returns a bounded page of personal notification records using the supplied pagination and
	// filters.
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
	List(context.Context, string, string, int) (domain.Page, error)
	// Read idempotently marks the account-owned notification as read and returns its saved state.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting notification.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Read(context.Context, string, string) (domain.Notification, error)
	// Generate creates a bounded batch of persistent notifications from saved schedules and
	// transactional events.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Generate(context.Context) error
	// Pending returns persisted notifications that have not yet completed event publication.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: The collected notification values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Pending(context.Context) ([]domain.Notification, error)
	// Publishable checks whether a saved notification should still be published to its recipient.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Publishable(context.Context, string) (bool, error)
	// Visible checks whether the notification and its source remain visible to the account.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Visible(context.Context, string, string) (bool, error)
	// Published records that a persisted notification has completed publication or suppression.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Published(context.Context, string) error
}

// Publisher defines the replaceable dependency contract for personal notification delivery.
type Publisher interface {
	// Publish publishes the saved personal notification change through its configured event or
	// command transport.
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

// Service coordinates personal notification delivery through its configured persistence and
// transport dependencies.
type Service struct {
	repo Repository
	bus  Publisher
}

// NewService constructs the service with the dependencies and limits required for personal
// notification delivery.
//
// Parameters:
//   - repo: Persistence dependency used by the application operation.
//   - bus: Scoped publication and subscription transport.
//
// Returns:
//   - result 1: The constructed or loaded service.
func NewService(repo Repository, bus Publisher) *Service { return &Service{repo: repo, bus: bus} }

// List returns a bounded page of personal notification records using the supplied pagination and
// filters.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - cursor: Opaque continuation boundary from the previous page.
//   - limit: Maximum number of results or operations in this batch.
//
// Returns:
//   - result 1: The resulting page.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) List(ctx context.Context, userID, cursor string, limit int) (domain.Page, error) {
	return s.repo.List(ctx, userID, cursor, limit)
}

// Read idempotently marks the account-owned notification as read and returns its saved state.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting notification.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Read(ctx context.Context, userID, id string) (domain.Notification, error) {
	item, err := s.repo.Read(ctx, userID, id)
	if err != nil {
		return item, err
	}
	// Persist read state before publication. Polling and reconnection recover missed Redis events;
	// private content never enters the conference-wide broadcast.
	event := realtime.Event("notification.read", "", map[string]any{"id": item.ID})
	_ = s.bus.Publish(ctx, userID, event)
	return item, nil
}

// Visible authorizes a queued notification again immediately before delivery.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Visible(ctx context.Context, userID, id string) (bool, error) {
	return s.repo.Visible(ctx, userID, id)
}

// Tick performs one bounded maintenance or delivery iteration after persisted state is
// available.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Tick(ctx context.Context) error {
	if err := s.repo.Generate(ctx); err != nil {
		return err
	}
	pending, err := s.repo.Pending(ctx)
	if err != nil {
		return err
	}
	for _, item := range pending {
		allowed, err := s.repo.Publishable(ctx, item.ID)
		if err != nil {
			return err
		}
		if allowed {
			event := realtime.Event("notification.created", "", map[string]any{"notification": item})
			event.ID = item.ID // Repeated deliveries of one event retain a stable identifier.
			if err = s.bus.Publish(ctx, item.UserID, event); err != nil {
				return err
			}
		}
		if err = s.repo.Published(ctx, item.ID); err != nil {
			return err
		}
	}
	return nil
}

// Run runs personal notification delivery until the context is canceled or processing fails.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		op, cancel := context.WithTimeout(ctx, 4*time.Second)
		err := s.Tick(op)
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.Warn("notification job failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
