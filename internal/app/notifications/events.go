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

// Subscriber открывает персональный канал уведомлений для SSE.
type Subscriber interface {
	// Subscribe использует контекст для подключения к каналу пользователя.
	// После успешного подключения вызывающая сторона закрывает PubSub сама.
	Subscribe(context.Context, string) (*goredis.PubSub, error)
}

// Verifier проверяет токен и определяет предельный срок SSE-подключения.
type Verifier interface {
	// VerifyWithExpiry принимает JWT и возвращает ID пользователя и срок действия.
	VerifyWithExpiry(string) (string, time.Time, error)
}

// Limiter ограничивает частоту попыток открыть SSE-подключение.
type Limiter interface {
	// Allow проверяет число обращений по ключу за указанное временное окно.
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

func (s *eventStreams) beginDrain() {
	s.drainOnce.Do(func() {
		if s.drain != nil {
			close(s.drain)
		}
	})
}

type persistentVerifier interface {
	VerifyAuthorization(context.Context, string) (string, string, string, time.Time, error)
	AuthorizeSession(context.Context, string, string) error
}

type streamIdentity struct {
	userID    string
	sessionID string
	expiry    time.Time
}

// authorize checks the same header credential already bound to the HTTP actor.
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
func (s *eventStreams) pump(ctx context.Context, writer gin.ResponseWriter, identity streamIdentity, messages <-chan *goredis.Message) {
	controller := http.NewResponseController(writer)
	defer controller.SetWriteDeadline(time.Time{})
	write := func(value string) bool {
		// Обёртка Gin не всегда поддерживает сроки записи. Ограниченная очередь SSE
		// не позволяет медленному читателю блокировать сигнализацию комнаты.
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
