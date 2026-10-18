package redis

import (
	"context"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	goredis "github.com/redis/go-redis/v9"
)

// One lease per physical account socket. Pruning and mutation are atomic across APIs.
type UserPresence struct {
	client *goredis.Client
	prefix string
	ttl    time.Duration
}

// NewUserPresence constructs the user presence with the dependencies and limits required for
// distributed presence and ownership.
//
// Parameters:
//   - c: Redis client used for atomic presence and ownership operations.
//   - namespace: Namespace isolating this component's distributed keys and channels.
//   - ttl: Lifetime assigned to the temporary value or ownership record.
//
// Returns:
//   - result 1: The constructed or loaded user presence.
func NewUserPresence(c *goredis.Client, namespace string, ttl time.Duration) *UserPresence {
	return &UserPresence{c, namespace + ":user-presence:", ttl}
}

var userPresenceScript = goredis.NewScript(`
local clock=redis.call('TIME');local now=clock[1]*1000+math.floor(clock[2]/1000)
redis.call('ZREMRANGEBYSCORE',KEYS[1],'-inf',now)
if ARGV[2]=='remove' then redis.call('ZREM',KEYS[1],ARGV[1]) else redis.call('ZADD',KEYS[1],now+tonumber(ARGV[3]),ARGV[1]) end
local n=redis.call('ZCARD',KEYS[1]);if n==0 then redis.call('DEL',KEYS[1]) else redis.call('PEXPIRE',KEYS[1],ARGV[3]) end
return n`)

var userPresenceCountScript = goredis.NewScript(`
local clock=redis.call('TIME');local now=clock[1]*1000+math.floor(clock[2]/1000)
return redis.call('ZCOUNT',KEYS[1],'('..now,'+inf')`)

// Touch renews presence using the supplied confirmed transport activity time.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - connection: Current account integration or physical transport connection.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *UserPresence) Touch(ctx context.Context, user, connection string) (int64, error) {
	return p.change(ctx, user, connection, "touch")
}

// Remove removes the specified physical client from the account's presence set.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - connection: Current account integration or physical transport connection.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *UserPresence) Remove(ctx context.Context, user, connection string) (int64, error) {
	return p.change(ctx, user, connection, "remove")
}

// change atomically updates the account presence set and returns its resulting state.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//   - action: Requested lifecycle or moderation action.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *UserPresence) change(ctx context.Context, user, id, action string) (int64, error) {
	return userPresenceScript.Run(ctx, p.client, []string{p.prefix + user}, id, action, p.ttl.Milliseconds()).Int64()
}

// Count returns the number of currently live physical sessions for the account.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *UserPresence) Count(ctx context.Context, user string) (int64, error) {
	return userPresenceCountScript.Run(ctx, p.client, []string{p.prefix + user}).Int64()
}

// A page uses one Redis command and a shared Redis clock, independent of API clocks.
const userPresenceOnlineScript = `
local clock=redis.call('TIME');local now=clock[1]*1000+math.floor(clock[2]/1000)
local result={}
for i,key in ipairs(KEYS) do result[i]=redis.call('ZCOUNT',key,'('..now,'+inf') end
return result`

// Online projects confirmed global account leases without extending their lifetime.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - users: Account identifiers or account repository, as defined by its type.
//
// Returns:
//   - result 1: The resulting map[string]bool.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *UserPresence) Online(ctx context.Context, users []string) (map[string]bool, error) {
	if len(users) > 100 {
		return nil, apperrors.ErrInvalidInput
	}
	keys, ids := []string{}, []string{}
	seen := make(map[string]bool, len(users))
	for _, user := range users {
		if user == "" || seen[user] {
			continue
		}
		seen[user] = true
		keys, ids = append(keys, p.prefix+user), append(ids, user)
	}
	result := make(map[string]bool, len(ids))
	if len(keys) == 0 {
		return result, nil
	}
	// EVAL avoids the extra NOSCRIPT retry on an API instance's first lookup.
	counts, err := p.client.Eval(ctx, userPresenceOnlineScript, keys).Int64Slice()
	if err != nil {
		return nil, err
	}
	if len(counts) != len(ids) {
		return nil, apperrors.ErrUnavailable
	}
	for i, id := range ids {
		result[id] = counts[i] > 0
	}
	return result, nil
}
