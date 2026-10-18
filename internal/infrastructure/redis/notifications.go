package redis

import (
	"context"
	"encoding/json"

	domain "github.com/janickiy/meet-space/internal/domain/realtime"
	goredis "github.com/redis/go-redis/v9"
)

// NotificationBus isolates personal notifications in account-specific Redis channels.
type NotificationBus struct {
	client *goredis.Client
	prefix string
}

// NewNotificationBus constructs the notification bus with the dependencies and limits required
// for distributed presence and ownership.
//
// Parameters:
//   - client: Browser media-client identity or transport dependency, as defined by its type.
//   - namespace: Namespace isolating this component's distributed keys and channels.
//
// Returns:
//   - result 1: The constructed or loaded notification bus.
func NewNotificationBus(client *goredis.Client, namespace string) *NotificationBus {
	return &NotificationBus{client: client, prefix: namespace + ":notification:"}
}

// Publish publishes the saved redis change through its configured event or command transport.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - event: Trusted event to publish or process.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (b *NotificationBus) Publish(ctx context.Context, userID string, event domain.Envelope) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return b.client.Publish(ctx, b.prefix+userID, data).Err()
}

// Subscribe opens a scoped event subscription whose lifetime is bounded by the supplied context.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: The constructed or loaded pub sub.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (b *NotificationBus) Subscribe(ctx context.Context, userID string) (*goredis.PubSub, error) {
	sub := b.client.Subscribe(ctx, b.prefix+userID)
	if _, err := sub.Receive(ctx); err != nil {
		_ = sub.Close()
		return nil, err
	}
	return sub, nil
}
