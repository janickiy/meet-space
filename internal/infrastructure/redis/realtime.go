package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/realtime"
	goredis "github.com/redis/go-redis/v9"
)

// The lease boundary starts at the API-confirmed pong time and is capped by Redis TIME plus TTL;
// API and Redis clocks must be synchronized. The expiring route key is authoritative. The index
// retains metadata needed to report failure after that key expires. All changes are atomic.
var presenceScript = goredis.NewScript(`
local prefix = ARGV[1]
local op = ARGV[2]
local clock = redis.call('TIME')
local now = tonumber(clock[1])*1000 + math.floor(tonumber(clock[2])/1000)
local meta = prefix .. ':metadata'
local expiry = prefix .. ':expiry'
local channel = prefix .. ':bus'
local function remove(id, kind)
    local raw = redis.call('HGET', meta, id)
    if raw then
        local s = cjson.decode(raw)
        redis.call('ZREM', prefix .. ':conference:' .. s.conferenceId, id)
        redis.call('PUBLISH', channel, cjson.encode({kind=kind, conferenceId=s.conferenceId, session=s}))
    end
    redis.call('DEL', prefix .. ':route:' .. id)
    redis.call('HDEL', meta, id)
    redis.call('ZREM', expiry, id)
end
for _, id in ipairs(redis.call('ZRANGEBYSCORE', expiry, '-inf', now, 'LIMIT', 0, 100)) do
    remove(id, 'expired')
end
local id = ARGV[3]
if op == 'join' then
    local raw = ARGV[4]
    local ttl = tonumber(ARGV[5])
    local deadline = math.min(tonumber(ARGV[6]), now+ttl)
    if deadline <= now then return 0 end
    local s = cjson.decode(raw)
    if redis.call('HEXISTS', meta, id) == 1 then return 0 end
    redis.call('HSET', meta, id, raw)
    redis.call('ZADD', expiry, deadline, id)
    redis.call('ZADD', prefix .. ':conference:' .. s.conferenceId, deadline, id)
    redis.call('SET', prefix .. ':route:' .. id, raw, 'PXAT', deadline)
    redis.call('PUBLISH', channel, cjson.encode({kind='connected', conferenceId=s.conferenceId, session=s}))
    return 1
elseif op == 'leave' then
    remove(id, 'disconnected')
    return 1
elseif op == 'touch' then
    local raw = redis.call('GET', prefix .. ':route:' .. id)
    if not raw then return 0 end
    local ttl = tonumber(ARGV[5])
    local deadline = math.min(tonumber(ARGV[6]), now+ttl)
    if deadline <= now then return 0 end
    local s = cjson.decode(raw)
    s.lastSeenAt = ARGV[4]
    raw = cjson.encode(s)
    redis.call('SET', prefix .. ':route:' .. id, raw, 'PXAT', deadline)
    redis.call('HSET', meta, id, raw)
    redis.call('ZADD', expiry, deadline, id)
    redis.call('ZADD', prefix .. ':conference:' .. s.conferenceId, deadline, id)
    return 1
elseif op == 'active' then
    local result = {}
    for _, cid in ipairs(redis.call('ZRANGEBYSCORE', prefix .. ':conference:' .. id, now+1, '+inf')) do
        local raw = redis.call('GET', prefix .. ':route:' .. cid)
        if raw then table.insert(result, raw) end
    end
    return result
end
return 1
`)

// RealtimeStore persists distributed presence and one-use tickets and delivers scoped Redis
// events.
type RealtimeStore struct {
	client *goredis.Client
	prefix string
}

// NewRealtimeStore constructs the realtime store with the dependencies and limits required for
// distributed presence and ownership.
//
// Parameters:
//   - client: Browser media-client identity or transport dependency, as defined by its type.
//   - prefix: Server-controlled object or key prefix.
//
// Returns:
//   - result 1: The constructed or loaded realtime store.
func NewRealtimeStore(client *goredis.Client, prefix string) *RealtimeStore {
	return &RealtimeStore{client: client, prefix: prefix}
}

// action executes the selected Redis Lua operation atomically within the component namespace.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - op: Atomic operation or command selected by the component.
//   - id: Identifier of the resource addressed by this operation.
//   - raw: Untrusted serialized value to decode or validate.
//   - ttl: Lifetime assigned to the temporary value or ownership record.
//   - deadline: Absolute deadline bounding the resource lifetime.
//
// Returns:
//   - result 1: The constructed or loaded cmd.
func (s *RealtimeStore) action(ctx context.Context, op, id, raw string, ttl time.Duration, deadline time.Time) *goredis.Cmd {
	return presenceScript.Run(ctx, s.client, []string{s.prefix + ":metadata", s.prefix + ":expiry"}, s.prefix, op, id, raw, ttl.Milliseconds(), deadline.UnixMilli())
}

// Register registers the physical connection and its bounded presence lifetime.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - session: Persistent or physical session state, as defined by its type.
//   - ttl: Lifetime assigned to the temporary value or ownership record.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *RealtimeStore) Register(ctx context.Context, session realtime.Session, ttl time.Duration) error {
	if session.LastSeenAt.IsZero() || ttl < time.Millisecond {
		return apperrors.ErrConflict
	}
	raw, err := json.Marshal(session)
	if err != nil {
		return err
	}
	n, err := s.action(ctx, "join", session.ConnectionID, string(raw), ttl, session.LastSeenAt.Add(ttl)).Int()
	if err == nil && n != 1 {
		return apperrors.ErrConflict
	}
	return err
}

// Unregister closes the physical session and updates the participant's distributed presence.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *RealtimeStore) Unregister(ctx context.Context, id string) error {
	return s.action(ctx, "leave", id, "", 0, time.Time{}).Err()
}

// Touch renews presence using the supplied confirmed transport activity time.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//   - confirmedAt: Timestamp of a transport-confirmed pong or other liveness acknowledgement.
//   - ttl: Lifetime assigned to the temporary value or ownership record.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *RealtimeStore) Touch(ctx context.Context, id string, confirmedAt time.Time, ttl time.Duration) error {
	if confirmedAt.IsZero() || ttl < time.Millisecond {
		return apperrors.ErrNotFound
	}
	n, err := s.action(ctx, "touch", id, confirmedAt.UTC().Format(time.RFC3339Nano), ttl, confirmedAt.Add(ttl)).Int()
	if err == nil && n != 1 {
		return apperrors.ErrNotFound
	}
	return err
}

// Get loads the requested redis within the caller's authorized scope.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting session.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *RealtimeStore) Get(ctx context.Context, id string) (realtime.Session, error) {
	raw, err := s.client.Get(ctx, s.prefix+":route:"+id).Bytes()
	if errors.Is(err, goredis.Nil) {
		return realtime.Session{}, apperrors.ErrNotFound
	}
	var session realtime.Session
	if err == nil {
		err = json.Unmarshal(raw, &session)
	}
	return session, err
}

// Missing checks route existence without reading session JSON. Each pipeline is
// bounded to the reconciliation page size. Partial replies on transport errors
// are discarded so a Redis failure cannot close live SQL sessions.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - ids: Resource identifiers to process as a bounded batch.
//
// Returns:
//   - result 1: The collected string values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *RealtimeStore) Missing(ctx context.Context, ids []string) ([]string, error) {
	var missing []string
	for start := 0; start < len(ids); start += 500 {
		batch := ids[start:min(start+500, len(ids))]
		commands := make([]*goredis.IntCmd, len(batch))
		_, err := s.client.Pipelined(ctx, func(pipe goredis.Pipeliner) error {
			for i, id := range batch {
				commands[i] = pipe.Exists(ctx, s.prefix+":route:"+id)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		for i, command := range commands {
			if command.Val() == 0 {
				missing = append(missing, batch[i])
			}
		}
	}
	return missing, nil
}

// Active returns physical sessions whose presence has not expired.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//
// Returns:
//   - result 1: The collected session values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *RealtimeStore) Active(ctx context.Context, conferenceID string) ([]realtime.Session, error) {
	raw, err := s.action(ctx, "active", conferenceID, "", 0, time.Time{}).StringSlice()
	if err != nil {
		return nil, err
	}
	result := make([]realtime.Session, 0, len(raw))
	for _, entry := range raw {
		var session realtime.Session
		if err := json.Unmarshal([]byte(entry), &session); err != nil {
			return nil, err
		}
		result = append(result, session)
	}
	return result, nil
}

// Prune removes expired physical sessions and returns the state needed to reconcile presence.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *RealtimeStore) Prune(ctx context.Context) error {
	return s.action(ctx, "prune", "", "", 0, time.Time{}).Err()
}

// Publish publishes the saved redis change through its configured event or command transport.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - bus: Scoped publication and subscription transport.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *RealtimeStore) Publish(ctx context.Context, bus realtime.Bus) error {
	raw, err := json.Marshal(bus)
	if err != nil {
		return err
	}
	return s.client.Publish(ctx, s.prefix+":bus", raw).Err()
}

// Subscribe opens a scoped event subscription whose lifetime is bounded by the supplied context.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: The resulting subscription.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *RealtimeStore) Subscribe(ctx context.Context) (realtime.Subscription, error) {
	p := s.client.Subscribe(ctx, s.prefix+":bus")
	if _, err := p.Receive(ctx); err != nil {
		_ = p.Close()
		return nil, err
	}
	return &realtimeSubscription{p}, nil
}

// realtimeSubscription wraps one Redis Pub/Sub subscription and its bounded lifetime.
type realtimeSubscription struct{ pub *goredis.PubSub }

// Close closes the component-owned resources and ends their lifecycle.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *realtimeSubscription) Close() error { return s.pub.Close() }

// Receive waits for and decodes the next scoped subscription event within the caller's
// cancellation lifetime.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: The resulting bus.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *realtimeSubscription) Receive(ctx context.Context) (realtime.Bus, error) {
	m, err := s.pub.ReceiveMessage(ctx)
	if err != nil {
		return realtime.Bus{}, err
	}
	var bus realtime.Bus
	err = json.Unmarshal([]byte(m.Payload), &bus)
	return bus, err
}

// ticketKey builds the namespaced Redis key for a one-use connection ticket.
//
// Parameters:
//   - ticket: One-use connection ticket or its server-side identity.
//
// Returns:
//   - result 1: Namespaced one-use ticket key.
func (s *RealtimeStore) ticketKey(ticket string) string {
	hash := sha256.Sum256([]byte(ticket))
	return s.prefix + ":ticket:" + hex.EncodeToString(hash[:])
}

// SaveTicket stores a one-use connection ticket with a bounded expiration.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - ticket: One-use connection ticket or its server-side identity.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - identity: Verified account and physical-session identity.
//   - ttl: Lifetime assigned to the temporary value or ownership record.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *RealtimeStore) SaveTicket(ctx context.Context, ticket, conferenceID string, identity realtime.Identity, ttl time.Duration) error {
	raw, err := json.Marshal(struct {
		ConferenceID string            `json:"conferenceId"`
		Identity     realtime.Identity `json:"identity"`
	}{conferenceID, identity})
	if err != nil {
		return err
	}
	return s.client.Set(ctx, s.ticketKey(ticket), raw, ttl).Err()
}

// ConsumeTicket atomically consumes a connection ticket so it cannot be replayed.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - ticket: One-use connection ticket or its server-side identity.
//   - conferenceID: Conference identifier limiting the operation scope.
//
// Returns:
//   - result 1: The resulting identity.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *RealtimeStore) ConsumeTicket(ctx context.Context, ticket, conferenceID string) (realtime.Identity, error) {
	if len(ticket) != 43 {
		return realtime.Identity{}, apperrors.ErrUnauthorized
	}
	raw, err := s.client.GetDel(ctx, s.ticketKey(ticket)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return realtime.Identity{}, apperrors.ErrUnauthorized
	}
	if err != nil {
		return realtime.Identity{}, err
	}
	var value struct {
		ConferenceID string            `json:"conferenceId"`
		Identity     realtime.Identity `json:"identity"`
	}
	if json.Unmarshal(raw, &value) != nil || value.ConferenceID != conferenceID || !value.Identity.ExpiresAt.After(time.Now()) {
		return realtime.Identity{}, apperrors.ErrUnauthorized
	}
	return value.Identity, nil
}
