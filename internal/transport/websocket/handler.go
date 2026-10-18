package websocket

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/janickiy/meet-space/internal/operations"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/ratelimit"
	domain "github.com/janickiy/meet-space/internal/domain/realtime"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	usecase "github.com/janickiy/meet-space/internal/usecase/realtime"
)

// Verifier defines the replaceable dependency contract for websocket.
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

// PersistentVerifier binds an access token to a revocable account session.
// Guest and legacy tokens retain their original expiration deadline.
type PersistentVerifier interface {
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

// Durable media bindings remain stable while the WebSocket checks the backing
// session on every heartbeat and command. Access JWT rotation cannot change a peer.
var durableAuthorizationUntil = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

// Tickets defines the replaceable dependency contract for websocket.
type Tickets interface {
	// SaveTicket stores a one-use connection ticket with a bounded expiration.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: One-use connection ticket or its server-side identity.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Verified account and physical-session identity.
	//   - argument 5: Lifetime assigned to the temporary value or ownership record.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SaveTicket(context.Context, string, string, domain.Identity, time.Duration) error
	// ConsumeTicket atomically consumes a connection ticket so it cannot be replayed.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: One-use connection ticket or its server-side identity.
	//   - argument 3: Conference identifier limiting the operation scope.
	//
	// Returns:
	//   - result 1: The resulting identity.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ConsumeTicket(context.Context, string, string) (domain.Identity, error)
}

// Limiter defines the replaceable dependency contract for websocket.
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

// Handler adapts HTTP requests to websocket, validates input and writes the response.
type Handler struct {
	hub         *usecase.Hub
	verifier    Verifier
	tickets     Tickets
	limiter     Limiter
	cfg         config.RealtimeConfig
	media       MediaController
	connections atomic.Int64
}

// MediaController defines the replaceable dependency contract for websocket.
type MediaController interface {
	// Handle dispatches the validated websocket operation to its application handler.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Persistent or physical session state, as defined by its type.
	//   - argument 3: Absolute expiration of the verified access credential.
	//   - argument 4: Trusted event to publish or process.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Handle(context.Context, domain.Session, time.Time, domain.Envelope) error
}

// NewHandler constructs the handler with the dependencies and limits required for websocket.
//
// Parameters:
//   - hub: Realtime hub coordinating authorized socket delivery.
//   - verifier: Token verifier or private PKCE verifier, as defined by its type.
//   - tickets: One-use or signed media-ticket issuer and verifier.
//   - limiter: Shared request-rate limiter.
//   - cfg: Operator-controlled configuration and processing limits.
//
// Returns:
//   - result 1: The constructed or loaded handler.
func NewHandler(hub *usecase.Hub, verifier Verifier, tickets Tickets, limiter Limiter, cfg config.RealtimeConfig) *Handler {
	return &Handler{hub: hub, verifier: verifier, tickets: tickets, limiter: limiter, cfg: cfg}
}

// SetMedia updates media for websocket.
//
// Parameters:
//   - controller: Media policy and command controller.
//
// Returns:
//   - result 1: The constructed or loaded handler.
func (h *Handler) SetMedia(controller MediaController) *Handler { h.media = controller; return h }

// RegisterRoutes registers the authenticated realtime endpoints and their request limits.
//
// Parameters:
//   - router: HTTP router on which the component endpoints are registered.
func (h *Handler) RegisterRoutes(router gin.IRouter) {
	router.GET("/api/v1/conferences/:id/ws", h.Connect)
	router.POST("/api/v1/conferences/:id/ws-ticket", h.Ticket)
	router.GET("/api/v1/webrtc/config", h.ICE)
}

// identity resolves the verified account and connection scope for a WebSocket request.
//
// Parameters:
//   - r: Incoming HTTP request.
//
// Returns:
//   - result 1: The resulting identity.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Handler) identity(r *http.Request) (domain.Identity, error) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return domain.Identity{}, apperrors.ErrUnauthorized
	}
	if persistent, ok := h.verifier.(PersistentVerifier); ok {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		id, scope, sessionID, expiry, err := persistent.VerifyAuthorization(ctx, parts[1])
		if err != nil {
			return domain.Identity{}, err
		}
		if !httpmiddleware.GuestRequestAllowed(r.Method, r.URL.Path, scope) {
			return domain.Identity{}, apperrors.ErrForbidden
		}
		return domain.Identity{UserID: id, ExpiresAt: expiry, AuthSessionID: sessionID}, nil
	}
	if scoped, ok := h.verifier.(httpmiddleware.SessionVerifier); ok {
		id, conferenceID, expiry, err := scoped.VerifySession(parts[1])
		if err != nil {
			return domain.Identity{}, err
		}
		if !httpmiddleware.GuestRequestAllowed(r.Method, r.URL.Path, conferenceID) {
			return domain.Identity{}, apperrors.ErrForbidden
		}
		return domain.Identity{UserID: id, ExpiresAt: expiry}, nil
	}
	id, expiry, err := h.verifier.VerifyWithExpiry(parts[1])
	return domain.Identity{UserID: id, ExpiresAt: expiry}, err
}

// conferenceID validates and normalizes the conference identifier from the request route.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
//
// Returns:
//   - result 1: Validated conference UUID.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func conferenceID(c *gin.Context) (string, error) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil {
		return "", apperrors.New(apperrors.ErrInvalidInput, "invalid conferenceId")
	}
	return id.String(), nil
}

// Ticket issues a short-lived one-use connection ticket after authorization succeeds.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Ticket(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	identity, err := h.identity(c.Request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	id, err := conferenceID(c)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	var request struct {
		ClientID string `json:"clientId"`
	}
	if err = json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 1024)).Decode(&request); err != nil && !errors.Is(err, io.EOF) {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	if request.ClientID != "" {
		clientID, parseErr := uuid.Parse(request.ClientID)
		if parseErr != nil || clientID == uuid.Nil {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		identity.ClientID = clientID.String()
	}
	if _, err = h.hub.AuthorizeClient(ctx, id, identity.UserID, identity.ClientID); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if h.limiter != nil {
		limit, err := h.limiter.Allow(ctx, h.cfg.Namespace+":ticket-limit:"+identity.UserID, 30, time.Minute)
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		if !limit.Allowed {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(429, gin.H{"error": "too many connection tickets"})
			return
		}
	}
	var random [32]byte
	if _, err = rand.Read(random[:]); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	ticket := base64.RawURLEncoding.EncodeToString(random[:])
	ttl := min(h.cfg.TicketTTL, time.Until(identity.ExpiresAt))
	if ttl <= 0 {
		httpresponse.Fail(c, apperrors.ErrUnauthorized)
		return
	}
	if err = h.tickets.SaveTicket(ctx, ticket, id, identity, ttl); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ticket": ticket, "expiresAt": time.Now().UTC().Add(ttl)})
}

// ICE applies a validated ICE candidate to the current media connection.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) ICE(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if _, err := h.identity(c.Request); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, h.cfg.TURN.ClientICE(h.cfg.ICE, time.Now()))
}

// origin checks the WebSocket Origin against the configured allowlist.
//
// Parameters:
//   - r: Incoming HTTP request.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (h *Handler) origin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	} // Native clients use Bearer authorization headers.
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if len(h.cfg.AllowedOrigins) == 0 {
		return strings.EqualFold(u.Host, r.Host)
	}
	for _, allowed := range h.cfg.AllowedOrigins {
		if strings.TrimSuffix(allowed, "/") == origin {
			return true
		}
	}
	return false
}

// Connect opens the configured dependency or starts its authorized connection flow.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Connect(c *gin.Context) {
	if h.cfg.MaxConnections > 0 && h.connections.Add(1) > int64(h.cfg.MaxConnections) {
		h.connections.Add(-1)
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	if h.cfg.MaxConnections <= 0 {
		h.connections.Add(1)
	}
	reserved := true
	defer func() {
		if reserved {
			h.connections.Add(-1)
		}
	}()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if !h.origin(c.Request) {
		httpresponse.Fail(c, apperrors.ErrForbidden)
		return
	}
	id, err := conferenceID(c)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil || len(c.Request.URL.RawQuery) > 512 || len(query) > 1 || (len(query) == 1 && len(query["ticket"]) != 1) {
		httpresponse.Fail(c, apperrors.ErrUnauthorized)
		return
	}
	var identity domain.Identity
	if ticket := query.Get("ticket"); ticket != "" {
		if c.GetHeader("Authorization") != "" {
			httpresponse.Fail(c, apperrors.ErrUnauthorized)
			return
		}
		identity, err = h.tickets.ConsumeTicket(ctx, ticket, id)
	} else {
		identity, err = h.identity(c.Request)
	}
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if identity.AuthSessionID != "" {
		persistent, ok := h.verifier.(PersistentVerifier)
		if !ok {
			httpresponse.Fail(c, apperrors.ErrUnauthorized)
			return
		}
		if err = persistent.AuthorizeSession(ctx, identity.UserID, identity.AuthSessionID); err != nil {
			httpresponse.Fail(c, err)
			return
		}
	}
	session, err := h.hub.PrepareClient(ctx, id, identity.UserID, identity.ClientID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if !identity.ExpiresAt.After(time.Now()) {
		h.hub.Abort(session)
		httpresponse.Fail(c, apperrors.ErrUnauthorized)
		return
	}
	upgrader := ws.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096, HandshakeTimeout: 5 * time.Second, CheckOrigin: h.origin, Subprotocols: []string{"go-recorder.v1"}}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.hub.Abort(session)
		return
	}
	client := newClient(conn, h, session, identity.ExpiresAt)
	client.authSessionID = identity.AuthSessionID
	if client.authSessionID != "" {
		client.expiresAt = durableAuthorizationUntil
	}
	registration, cancelRegistration := context.WithTimeout(context.Background(), 5*time.Second)
	err = h.hub.Register(registration, session, client)
	cancelRegistration()
	if err != nil {
		client.Stop("registration_failed")
		_ = conn.Close()
		return
	}
	reserved = false
	go func() {
		operations.WSActive(1)
		defer operations.WSActive(-1)
		defer h.connections.Add(-1)
		client.run()
	}()
}

// control carries one WebSocket control command to the socket's single writer.
type control struct {
	kind int
	data []byte
}

// client owns one physical WebSocket, its single writer, bounded event queues and
// incoming-message limits.
type client struct {
	conn          *ws.Conn
	handler       *Handler
	session       domain.Session
	expiresAt     time.Time
	authSessionID string
	out           chan domain.Envelope
	low           chan domain.Envelope
	controls      chan control
	done          chan struct{}
	once          sync.Once
	reason        string
}

// newClient creates a socket client with bounded queues and its verified physical-session
// identity.
//
// Parameters:
//   - conn: Connection owned by the current operation.
//   - h: HTTP handler adapter to register or bind to the client.
//   - s: The session state processed by this operation.
//   - expiry: Expiration deadline or validity interval, as defined by its type.
//
// Returns:
//   - result 1: The constructed or loaded client.
func newClient(conn *ws.Conn, h *Handler, s domain.Session, expiry time.Time) *client {
	return &client{conn: conn, handler: h, session: s, expiresAt: expiry, out: make(chan domain.Envelope, h.cfg.QueueSize), low: make(chan domain.Envelope, 8), controls: make(chan control, 8), done: make(chan struct{})}
}

// Offer enqueues the event in the socket's bounded queue according to its priority and reports
// whether it was accepted.
//
// Parameters:
//   - event: Trusted event to publish or process.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (c *client) Offer(event domain.Envelope) bool {
	if limit := c.handler.cfg.OutboundBytes; limit > 0 && len(event.Data) > limit {
		c.Stop("outbound_too_large")
		return false
	}
	select {
	case <-c.done:
		return false
	default:
	}
	if domain.LowPriorityEvent(event.Type) {
		select {
		case c.low <- event:
		default:
		}
		return true // Dropping recoverable events must not close media connections.
	}
	select {
	case c.out <- event:
		return true
	default:
		c.Stop("slow_client")
		return false
	}
}

// Stop stops the active websocket operation and releases its owned resources.
//
// Parameters:
//   - reason: Bounded lifecycle or failure reason.
func (c *client) Stop(reason string) {
	c.once.Do(

		func() { c.reason = reason; close(c.done) })
}

// run runs websocket until the context is canceled or processing fails.
func (c *client) run() {
	written := make(chan struct{})
	go func() { defer close(written); c.write() }()
	c.read()
	c.Stop("client_closed")
	// The close frame may wait for the network write timeout. Do not retain presence or media for a
	// physical connection that is already lost.
	c.handler.hub.Unregister(c.session)
	<-written
}

// write serially writes socket events and control frames while prioritizing critical delivery.
func (c *client) write() {
	defer c.conn.Close()
	ticker := time.NewTicker(c.handler.cfg.PingInterval)
	defer ticker.Stop()
	var expiryChannel <-chan time.Time
	if c.authSessionID == "" {
		expiry := time.NewTimer(max(time.Until(c.expiresAt), time.Nanosecond))
		defer expiry.Stop()
		expiryChannel = expiry.C
	}

	write := func(kind int, data []byte) bool {
		_ = c.conn.SetWriteDeadline(time.Now().Add(c.handler.cfg.WriteTimeout))
		return c.conn.WriteMessage(kind, data) == nil
	}
	for {
		select {
		case <-c.done:
			code := ws.CloseNormalClosure
			if c.reason == "server_shutdown" || c.reason == "presence_timeout" {
				code = ws.CloseGoingAway
			} else if c.reason != "client_closed" {
				code = ws.ClosePolicyViolation
			}
			_ = write(ws.CloseMessage, ws.FormatCloseMessage(code, c.reason))
			return
		case <-expiryChannel:
			c.Stop("authentication_expired")
		case <-ticker.C:
			if !write(ws.PingMessage, []byte(c.session.ConnectionID)) {
				c.Stop("write_failed")
				return
			}
		case frame := <-c.controls:
			if !write(frame.kind, frame.data) {
				c.Stop("write_failed")
				return
			}
		case event := <-c.out:
			raw, err := json.Marshal(event)
			if err != nil || !write(ws.TextMessage, raw) {
				c.Stop("write_failed")
				return
			}
		case event := <-c.low:
			// When both queues are ready, send the critical event first. A selected low-priority event may
			// be dropped as if its queue overflowed.
			select {
			case event = <-c.out:
			default:
			}
			raw, err := json.Marshal(event)
			if err != nil || !write(ws.TextMessage, raw) {
				c.Stop("write_failed")
				return
			}
		}
	}
}

// bucket contains the local incoming-message rate state of one physical socket.
type bucket struct {
	tokens      float64
	updated     time.Time
	rate, burst float64
}

// allow checks the local per-socket incoming-message budget.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (b *bucket) allow() bool {
	now := time.Now()
	b.tokens = min(b.burst, b.tokens+now.Sub(b.updated).Seconds()*b.rate)
	b.updated = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// authorize keeps durable sessions revocable without tying media to a JWT timer.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *client) authorize() error {
	if c.authSessionID == "" {
		if c.expiresAt.After(time.Now()) {
			return nil
		}
		c.Stop("authentication_expired")
		return apperrors.ErrUnauthorized
	}
	persistent, ok := c.handler.verifier.(PersistentVerifier)
	if !ok {
		c.Stop("authentication_revoked")
		return apperrors.ErrUnauthorized
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := persistent.AuthorizeSession(ctx, c.session.UserID, c.authSessionID)
	if err != nil {
		if errors.Is(err, apperrors.ErrUnauthorized) || errors.Is(err, apperrors.ErrNotFound) {
			c.Stop("authentication_revoked")
		} else {
			c.Stop("authentication_unavailable")
		}
	}
	return err
}

// read reads incoming stream events until cancellation or terminal failure and enforces the
// destination queue bound.
func (c *client) read() {
	cfg := c.handler.cfg
	c.conn.SetReadLimit(cfg.MessageBytes)
	initialSeen := c.session.LastSeenAt
	if initialSeen.IsZero() {
		initialSeen = time.Now()
	}
	_ = c.conn.SetReadDeadline(initialSeen.Add(cfg.PingInterval + cfg.PongTimeout))
	rate := bucket{tokens: float64(cfg.Burst), updated: time.Now(), rate: float64(cfg.MessagesPerSecond), burst: float64(cfg.Burst)}
	lastPong := time.Time{}
	c.conn.SetPongHandler(

		func(value string) error {
			if !rate.allow() {
				return errors.New("control rate exceeded")
			}
			if value != c.session.ConnectionID || time.Since(lastPong) < cfg.PingInterval/2 {
				return nil
			}
			confirmedAt := time.Now()
			if err := c.authorize(); err != nil {
				return err
			}
			lastPong = confirmedAt
			c.session.LastSeenAt = lastPong.UTC()
			deadline := lastPong.Add(cfg.PingInterval + cfg.PongTimeout)
			// Measure from pong receipt, not from completion of the storage request.
			if err := c.conn.SetReadDeadline(deadline); err != nil {
				return err
			}
			ctx, cancel := context.WithDeadline(context.Background(), deadline)
			defer cancel()
			if err := c.handler.hub.Touch(ctx, c.session); err != nil {
				return errors.New("presence lease lost")
			}
			return nil
		})
	c.conn.SetPingHandler(

		func(value string) error {
			if !rate.allow() {
				return errors.New("control rate exceeded")
			}
			select {
			case c.controls <- control{ws.PongMessage, []byte(value)}:
				return nil
			default:
				return errors.New("control overflow")
			}
		})
	c.conn.SetCloseHandler(

		func(_ int, _ string) error { c.Stop("client_closed"); return nil })
	for {
		kind, raw, err := c.conn.ReadMessage()
		if err != nil {
			var networkError net.Error
			if errors.As(err, &networkError) && networkError.Timeout() {
				c.Stop("presence_timeout")
			}
			return
		}
		if !rate.allow() {
			c.Stop("rate_limited")
			return
		}
		if kind != ws.TextMessage {
			c.Stop("text_messages_only")
			return
		}
		if !c.handleMessage(raw) {
			return
		}
	}
}

// failure builds a bounded protocol-error event containing only the safe code and validated
// correlation identifier.
//
// Parameters:
//   - replyTo: Validated client correlation identifier for the error response.
//   - code: Bounded public status or failure code.
func (c *client) failure(replyTo, code string) {
	event := domain.Event("error", c.session.ConferenceID, map[string]string{"code": code})
	if len(replyTo) <= 36 {
		event.ReplyTo = replyTo
	}
	c.Offer(event)
}

// strictJSON decodes bounded JSON with unknown-field and trailing-data rejection.
//
// Parameters:
//   - raw: Untrusted serialized value to decode or validate.
//   - value: Value to validate, normalize or convert.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func strictJSON(raw []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}
