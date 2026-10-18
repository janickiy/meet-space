package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const conferenceLockPrefix = "record:conference:"

var releaseLockScript = goredis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

var acquireLockScript = goredis.NewScript(`
local owner = redis.call("GET", KEYS[1])
if owner == ARGV[1] then
    redis.call("PEXPIRE", KEYS[1], ARGV[2])
    return 1
end
if not owner then
    redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
    return 1
end
return 0
`)

// ConferenceLock maintains the Redis lock for one active recording per conference.
type ConferenceLock struct {
	client *goredis.Client
	ttl    time.Duration
}

// NewConferenceLock constructs the conference lock with the dependencies and limits required for
// distributed presence and ownership.
//
// Parameters:
//   - client: Browser media-client identity or transport dependency, as defined by its type.
//   - ttl: Lifetime assigned to the temporary value or ownership record.
//
// Returns:
//   - result 1: The constructed or loaded conference lock.
func NewConferenceLock(client *goredis.Client, ttl time.Duration) *ConferenceLock {
	return &ConferenceLock{client: client, ttl: ttl}
}

// Acquire atomically acquires a conference recording lock with the specified recording as its
// owner.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - recordID: External recording UUID.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (l *ConferenceLock) Acquire(ctx context.Context, conferenceID string, recordID string) (bool, error) {
	if l == nil || l.client == nil {
		return true, nil
	}
	ttl := l.ttl
	if ttl <= 0 {
		ttl = 6 * time.Hour
	}
	result, err := acquireLockScript.Run(ctx, l.client, []string{lockKey(conferenceID)}, recordID, ttl.Milliseconds()).Int()
	if err != nil {
		return false, fmt.Errorf("acquire conference record lock: %w", err)
	}

	return result == 1, nil
}

// Release releases only the redis resource owned by the matching operation or token.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - recordID: External recording UUID.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (l *ConferenceLock) Release(ctx context.Context, conferenceID string, recordID string) error {
	if l == nil || l.client == nil || strings.TrimSpace(conferenceID) == "" || strings.TrimSpace(recordID) == "" {
		return nil
	}
	if err := releaseLockScript.Run(ctx, l.client, []string{lockKey(conferenceID)}, recordID).Err(); err != nil {
		return fmt.Errorf("release conference record lock: %w", err)
	}

	return nil
}

// lockKey builds the namespaced Redis key for a conference recording lock.
//
// Parameters:
//   - conferenceID: Conference identifier limiting the operation scope.
//
// Returns:
//   - result 1: Namespaced conference recording-lock key.
func lockKey(conferenceID string) string {
	return conferenceLockPrefix + strings.TrimSpace(conferenceID) + ":lock"
}
