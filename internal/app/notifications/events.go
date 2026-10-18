package notificationsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/ratelimit"
	domain "github.com/janickiy/meet-space/internal/domain/realtime"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	usecase "github.com/janickiy/meet-space/internal/usecase/notifications"
	goredis "github.com/redis/go-redis/v9"
)

// Subscriber opens the account-specific notification channel used by SSE.
type Subscriber interface {
	// Subscribe opens a scoped event subscription whose lifetime is bounded by the supplied context.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//
	// Returns:
	//   - result 1: The constructed or loaded pub sub.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Subscribe(context.Context, string) (*goredis.PubSub, error)
}

// Verifier validates the token and determines the maximum lifetime of the SSE connection.
type Verifier interface {
	// VerifyWithExpiry verifies an access token and returns the account identity and token deadline.
	//
	// Parameters:
	//   - argument 1: Untrusted serialized value to decode or validate.
	//
	// Returns:
	//   - result 1: Authenticated account identifier.
	//   - result 2: The timestamp or validity boundary associated with the result.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	VerifyWithExpiry(string) (string, time.Time, error)
}

// Limiter bounds attempts to open an SSE connection.
type Limiter interface {
	// Allow evaluates the rate limit and returns the remaining budget and reset boundary.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Storage, cache or cryptographic key used by the operation.
	//   - argument 3: Maximum number of results or operations in this batch.
	//   - argument 4: Duration of the rate-limit or observation window.
	//
	// Returns:
	//   - result 1: The resulting result.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Allow(context.Context, string, int, time.Duration) (ratelimit.Result, error)
}

// eventStreams owns SSE admission limits, subscriptions and graceful draining.
// Each serve call owns its request context, Redis subscription, write deadline
// and keepalive ticker; completion releases capacity for the next connection.
type eventStreams struct {
	service   *usecase.Service
	bus       Subscriber
	tokens    Verifier
	limiter   Limiter
	streams   atomic.Int64
	counts    syncCounts
	namespace string
	drain     chan struct{}
	drainOnce sync.Once
}

// beginDrain stops accepting new work while existing operations finish.
func (s *eventStreams) beginDrain() {
	s.drainOnce.Do(func() {
		if s.drain != nil {
			close(s.drain)
		}
	})
}

type persistentVerifier interface {
	// VerifyAuthorization verifies the authorization token and resolves its user, session, guest
	// scope and expiry.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Untrusted serialized value to decode or validate.
	//
	// Returns:
	//   - result 1: Verified account identifier.
	//   - result 2: Guest conference scope, or an empty string for a registered account.
	//   - result 3: Persistent server-session identifier, or an empty string for a guest or legacy
	//     token.
	//   - result 4: The timestamp or validity boundary associated with the result.
	//   - result 5: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	VerifyAuthorization(context.Context, string) (string, string, string, time.Time, error)
	// AuthorizeSession checks that the server session is current, unrevoked and bound to the
	// requested account.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Persistent or physical session identifier, as defined by the operation.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	AuthorizeSession(context.Context, string, string) error
}

type streamIdentity struct {
	userID    string
	sessionID string
	expiry    time.Time
}

// authorize checks the same header credential already bound to the HTTP actor.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//
// Returns:
//   - result 1: The resulting stream identity.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *eventStreams) authorize(c *gin.Context) (streamIdentity, error) {
	// Credentials remain in the header, never in URLs or access logs.
	fields := strings.Fields(c.GetHeader("Authorization"))
	if len(fields) != 2 || len(c.Request.URL.RawQuery) > 0 {
		return streamIdentity{}, apperrors.ErrUnauthorized
	}
	var identity streamIdentity
	var err error
	if persistent, ok := s.tokens.(persistentVerifier); ok {
		verifyContext, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		identity.userID, _, identity.sessionID, identity.expiry, err = persistent.VerifyAuthorization(verifyContext, fields[1])
		cancel()
	} else {
		identity.userID, identity.expiry, err = s.tokens.VerifyWithExpiry(fields[1])
	}
	if err != nil {
		return streamIdentity{}, err
	}
	if identity.userID != httpmiddleware.UserID(c) {
		return streamIdentity{}, apperrors.ErrUnauthorized
	}
	return identity, nil
}

// serve serves the authorized notification event stream until cancellation, expiry or drain.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (s *eventStreams) serve(c *gin.Context) {
	identity, err := s.authorize(c)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	userID := identity.userID
	if s.limiter != nil {
		result, err := s.limiter.Allow(c.Request.Context(), s.namespace+":notifications:sse:"+userID, 12, time.Minute)
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		if !result.Allowed {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(429, gin.H{"error": "too many notification connections"})
			return
		}
	}
	if s.streams.Add(1) > 256 {
		s.streams.Add(-1)
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	defer s.streams.Add(-1)
	if !s.counts.acquire(userID) {
		c.AbortWithStatusJSON(429, gin.H{"error": "too many notification connections"})
		return
	}
	defer s.counts.release(userID)
	ctx, cancel := context.WithDeadline(c.Request.Context(), identity.expiry)
	defer cancel()
	connect, cancelConnect := context.WithTimeout(ctx, 5*time.Second)
	sub, err := s.bus.Subscribe(connect, userID)
	cancelConnect()
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	defer sub.Close()
	messages := sub.Channel(goredis.WithChannelSize(32), goredis.WithChannelSendTimeout(time.Millisecond))
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	c.Status(200)
	s.pump(ctx, c.Writer, identity, messages)
}

// pump owns the write deadline and ticker until the stream finishes. Returning
// unwinds serve's subscription, request context and admission-slot defers.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - writer: Destination response or content writer.
//   - identity: Verified account and physical-session identity.
//   - messages: Incoming scoped event stream.
func (s *eventStreams) pump(ctx context.Context, writer gin.ResponseWriter, identity streamIdentity, messages <-chan *goredis.Message) {
	controller := http.NewResponseController(writer)
	defer controller.SetWriteDeadline(time.Time{})
	write := func(value string) bool {
		// The Gin wrapper does not always support write deadlines. A bounded SSE queue prevents a slow
		// reader from blocking room signaling.
		if err := controller.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil && err != http.ErrNotSupported {
			return false
		}
		if _, err := fmt.Fprint(writer, value); err != nil {
			return false
		}
		writer.Flush()
		return true
	}
	if !write(": connected\n\n") {
		return
	}
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.drain:
			return
		case <-keepalive.C:
			if !s.active(ctx, identity) {
				return
			}
			if !write(": keepalive\n\n") {
				return
			}
		case message, ok := <-messages:
			if !ok || !s.active(ctx, identity) {
				return
			}
			allowed, err := s.visible(ctx, identity.userID, message.Payload)
			if err != nil {
				return
			}
			if !allowed {
				continue
			}
			if !write("data: " + message.Payload + "\n\n") {
				return
			}
		}
	}
}

// active rechecks persistent-session revocation before each delivery/keepalive.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - identity: Verified account and physical-session identity.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (s *eventStreams) active(ctx context.Context, identity streamIdentity) bool {
	if identity.sessionID == "" {
		return true
	}
	persistent := s.tokens.(persistentVerifier)
	checkContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return persistent.AuthorizeSession(checkContext, identity.userID, identity.sessionID) == nil
}

// visible preserves the notification event allowlist and reauthorizes created
// notifications at delivery time, so queued events cannot bypass current ACLs.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - payload: Structured event or command payload.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *eventStreams) visible(ctx context.Context, userID, payload string) (bool, error) {
	var event domain.Envelope
	if len(payload) > 8192 {
		return false, apperrors.ErrInvalidInput
	}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return false, err
	}
	if event.Type != "notification.created" && event.Type != "notification.read" {
		return false, nil
	}
	if event.Type == "notification.created" {
		checkContext, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return s.service.Visible(checkContext, userID, event.ID)
	}
	return true, nil
}
