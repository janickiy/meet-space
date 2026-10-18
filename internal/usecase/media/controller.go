package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/config"
	domain "github.com/janickiy/meet-space/internal/domain/media"
	"github.com/janickiy/meet-space/internal/domain/realtime"
)

// Registry defines the replaceable dependency contract for authenticated media-room control.
type Registry interface {
	// Workers returns the currently live media workers eligible to own a room.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: The collected worker values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Workers(context.Context) ([]domain.Worker, error)
	// Claim atomically acquires the current media-room connection lease or ownership record.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//   - argument 3: Identity of the worker owning this operation.
	//   - argument 4: Lifetime assigned to the temporary value or ownership record.
	//
	// Returns:
	//   - result 1: The resulting route.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Claim(context.Context, string, string, time.Duration) (domain.Route, error)
	// GetOwner loads the current distributed owner and fencing version of the media room.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//
	// Returns:
	//   - result 1: The resulting route.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	GetOwner(context.Context, string) (domain.Route, error)
}

// Tickets defines the replaceable dependency contract for authenticated media-room control.
type Tickets interface {
	// Issue creates a signed token bound to the supplied identity and expiry.
	//
	// Parameters:
	//   - argument 1: Verified account, session and media-connection identity.
	//   - argument 2: Current media-room owner and fencing version.
	//
	// Returns:
	//   - result 1: Signed short-lived credential for the verified identity.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Issue(domain.Binding, domain.Route) (string, error)
}

// Transport defines the replaceable dependency contract for authenticated media-room control.
type Transport interface {
	// Call performs the authenticated internal HTTP operation against the selected media worker.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Requested lifecycle or moderation action.
	//   - argument 3: Validated internal recording or media command.
	//
	// Returns:
	//   - result 1: The resulting result.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Call(context.Context, string, domain.Command) (domain.Result, error)
}

// Sessions defines the replaceable dependency contract for authenticated media-room control.
type Sessions interface {
	// ValidateSession binds the media operation to a current, authorized physical WebSocket session.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Persistent or physical session state, as defined by its type.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ValidateSession(context.Context, realtime.Session) error
}

// Publisher defines the replaceable dependency contract for authenticated media-room control.
type Publisher interface {
	// Publish publishes the saved media-room connection change through its configured event or
	// command transport.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Scoped publication and subscription transport.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Publish(context.Context, realtime.Bus) error
}

// PolicyProvider defines the replaceable dependency contract for authenticated media-room
// control.
type PolicyProvider interface {
	// MediaPolicy loads the effective saved media restrictions for a participant.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//   - argument 3: Conference participant identifier.
	//
	// Returns:
	//   - result 1: The resulting participant policy.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	MediaPolicy(context.Context, string, string) (domain.ParticipantPolicy, error)
}

// endpoint holds the endpoint data used by authenticated media-room control.
type endpoint struct {
	binding domain.Binding
	route   domain.Route
	peerID  string
}

// Controller binds client signaling to physical-session authorization, room ownership and
// internal media transport.
type Controller struct {
	registry       Registry
	tickets        Tickets
	transport      Transport
	sessions       Sessions
	publisher      Publisher
	cfg            config.MediaConfig
	limits         config.RealtimeConfig
	mu             sync.Mutex
	peers          map[string]endpoint
	policyProvider PolicyProvider
}

// SetPolicyProvider updates policy provider for authenticated media-room control.
//
// Parameters:
//   - provider: Configured external-provider adapter.
func (c *Controller) SetPolicyProvider(provider PolicyProvider) { c.policyProvider = provider }

// policy loads the participant's effective server-side media restrictions.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - binding: Verified account, session and media-connection identity.
//
// Returns:
//   - result 1: The constructed or loaded participant policy.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Controller) policy(ctx context.Context, binding domain.Binding) (*domain.ParticipantPolicy, error) {
	if c.policyProvider == nil {
		return nil, nil
	}
	policy, err := c.policyProvider.MediaPolicy(ctx, binding.ConferenceID, binding.ParticipantID)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}
	return &policy, nil
}

// SetParticipantPolicy updates participant policy for authenticated media-room control.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - participantID: Conference participant identifier.
//   - policy: Effective server-side media policy.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Controller) SetParticipantPolicy(ctx context.Context, conferenceID, participantID string, policy domain.ParticipantPolicy) error {
	if !validUUID(conferenceID) || !validUUID(participantID) || policy.Version < 0 {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.OperationTimeout)
	defer cancel()
	route, err := c.registry.GetOwner(ctx, conferenceID)
	if errors.Is(err, domain.ErrOwnership) {
		return nil
	}
	if err != nil {
		return domain.ErrUnavailable
	}
	_, err = c.transport.Call(ctx, "policy", domain.Command{RequestID: uuid.NewString(), ConferenceID: conferenceID, ParticipantID: participantID, Route: route, Policy: &policy})
	return err
}

// CloseConference closes the media peers and owned resources of one conference.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Controller) CloseConference(ctx context.Context, conferenceID string) error {
	if !validUUID(conferenceID) {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.OperationTimeout)
	defer cancel()
	route, err := c.registry.GetOwner(ctx, conferenceID)
	if errors.Is(err, domain.ErrOwnership) {
		return nil
	}
	if err != nil {
		return domain.ErrUnavailable
	}
	_, err = c.transport.Call(ctx, "close", domain.Command{RequestID: uuid.NewString(), ConferenceID: conferenceID, Route: route})
	return err
}

// NewController constructs the controller with the dependencies and limits required for
// authenticated media-room control.
//
// Parameters:
//   - registry: Dependency or metrics registry used to locate or register the component.
//   - tickets: One-use or signed media-ticket issuer and verifier.
//   - transport: Transport implementation or RTP receiver for this operation.
//   - sessions: Physical-session authorization dependency.
//   - publisher: Trusted event publisher.
//   - cfg: Operator-controlled configuration and processing limits.
//   - limits: Configured protocol, queue and execution limits.
//
// Returns:
//   - result 1: The constructed or loaded controller.
func NewController(registry Registry, tickets Tickets, transport Transport, sessions Sessions, publisher Publisher, cfg config.MediaConfig, limits config.RealtimeConfig) *Controller {
	limits.SDPBytes = min(limits.SDPBytes, domain.MaxSDPBytes)
	limits.ICEBytes = min(limits.ICEBytes, domain.MaxICEBytes)
	return &Controller{registry: registry, tickets: tickets, transport: transport, sessions: sessions, publisher: publisher, cfg: cfg, limits: limits, peers: map[string]endpoint{}}
}

// Handle dispatches the validated media-room connection operation to its application handler.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - session: Persistent or physical session state, as defined by its type.
//   - expiresAt: Absolute expiration of the verified access credential.
//   - event: Trusted event to publish or process.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Controller) Handle(ctx context.Context, session realtime.Session, expiresAt time.Time, event realtime.Envelope) error {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.OperationTimeout)
	defer cancel()
	if !expiresAt.After(time.Now()) || c.sessions.ValidateSession(ctx, session) != nil {
		return domain.ErrUnauthorized
	}
	signal, err := c.decode(event.Type, event.Data)
	if err != nil {
		return err
	}
	binding := domain.Binding{ConferenceID: session.ConferenceID, ParticipantID: session.ParticipantID, SessionID: session.ID, ConnectionID: session.ConnectionID, UserID: session.UserID, AuthorizationExpiresAt: expiresAt}
	if event.Type == "media.join" {
		return c.join(ctx, binding, event.ID)
	}
	c.mu.Lock()
	peer, exists := c.peers[session.ConnectionID]
	c.mu.Unlock()
	if !exists {
		if event.Type == "media.leave" {
			return c.emit(ctx, binding, "media.left", event.ID, map[string]string{"mediaPeerId": signal.MediaPeerID})
		}
		return domain.ErrPeerNotFound
	}
	// An empty media.leave cancels a queued join before the browser received its connection
	// identifier. The single WebSocket reader executes it after the join response; cleanup is
	// restricted to the sender's authorized session.
	if signal.MediaPeerID != peer.peerID && !(event.Type == "media.leave" && signal.MediaPeerID == "") {
		return domain.ErrUnauthorized
	}
	owner, err := c.registry.GetOwner(ctx, session.ConferenceID)
	if err != nil {
		return domain.ErrOwnership
	}
	if owner != peer.route {
		return domain.ErrOwnership
	}
	command := domain.Command{RequestID: event.ID, Binding: peer.binding, Route: peer.route, MediaPeerID: peer.peerID, NegotiationID: signal.NegotiationID, SDP: signal.SDP, Candidate: signal.Candidate, TrackID: signal.TrackID}
	command.Publications = signal.Publications
	if event.Type == "media.offer" {
		command.Policy, err = c.policy(ctx, binding)
		if err != nil {
			return err
		}
	}
	action := strings.TrimPrefix(event.Type, "media.")
	result, err := c.transport.Call(ctx, action, command)
	if err != nil {
		return err
	}
	if action == "offer" {
		if result.MediaPeerID != peer.peerID || result.SDP == "" || len(result.SDP) > c.limits.SDPBytes {
			return domain.ErrUnavailable
		}
		return c.emit(ctx, binding, "media.answer", event.ID, map[string]string{"mediaPeerId": peer.peerID, "negotiationId": signal.NegotiationID, "sdp": result.SDP})
	}
	if action == "leave" {
		c.mu.Lock()
		delete(c.peers, session.ConnectionID)
		c.mu.Unlock()
		return c.emit(ctx, binding, "media.left", event.ID, map[string]string{"mediaPeerId": peer.peerID})
	}
	return c.emit(ctx, binding, "ack", event.ID, map[string]string{"type": event.Type})
}

// join selects the fenced media-room owner and prepares a connection for the verified physical
// session.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - binding: Verified account, session and media-connection identity.
//   - requestID: Client-supplied idempotency key for a retriable operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Controller) join(ctx context.Context, binding domain.Binding, requestID string) error {
	policy, err := c.policy(ctx, binding)
	if err != nil {
		return err
	}
	if policy != nil && policy.Kicked {
		return domain.ErrPolicy
	}
	workers, err := c.registry.Workers(ctx)
	if err != nil || len(workers) == 0 {
		return domain.ErrUnavailable
	}
	// A small stable hash routes new conferences; Claim preserves a live existing owner. This is
	// routing, not a distributed scheduler.
	conf, _ := uuid.Parse(binding.ConferenceID)
	index := int(conf[0]) % len(workers)
	route, err := c.registry.Claim(ctx, binding.ConferenceID, workers[index].ID, c.cfg.OwnershipTTL)
	if err != nil {
		return domain.ErrUnavailable
	}
	ticket, err := c.tickets.Issue(binding, route)
	if err != nil {
		return domain.ErrUnauthorized
	}
	result, err := c.transport.Call(ctx, "join", domain.Command{RequestID: requestID, Binding: binding, Route: route, Ticket: ticket, Policy: policy})
	if err != nil {
		return err
	}
	if parsed, err := uuid.Parse(result.MediaPeerID); err != nil || parsed == uuid.Nil || result.MaxPeers < 2 || result.MaxPeers > 32 || result.WorkerID != route.WorkerID {
		return domain.ErrUnavailable
	}
	video := result.VideoCapture
	if video.MaxWidth < 1 || video.MaxWidth > 1280 || video.MaxHeight < 1 || video.MaxHeight > 720 || video.MaxFrameRate < 1 || video.MaxFrameRate > 30 {
		// The worker prepared a connection, but an invalid capture source may never reach the browser.
		// Release the prepared media connection rather than leaving it ownerless.
		cleanup, cancel := context.WithTimeout(context.Background(), min(c.cfg.OperationTimeout, 3*time.Second))
		_, _ = c.transport.Call(cleanup, "leave", domain.Command{RequestID: uuid.NewString(), Binding: binding, Route: route, MediaPeerID: result.MediaPeerID})
		cancel()
		return domain.ErrUnavailable
	}
	peer := endpoint{binding: binding, route: route, peerID: result.MediaPeerID}
	c.mu.Lock()
	c.peers[binding.ConnectionID] = peer
	c.mu.Unlock()
	if result.Tracks == nil {
		result.Tracks = []domain.Track{}
	}
	if result.ICEServers == nil {
		result.ICEServers = []realtime.ICEServer{}
	}
	err = c.emit(ctx, binding, "media.joined", requestID, map[string]any{"mediaPeerId": result.MediaPeerID, "workerId": result.WorkerID, "maxPeers": result.MaxPeers, "iceServers": result.ICEServers, "tracks": result.Tracks, "videoCapture": result.VideoCapture, "policy": result.Policy})
	if err != nil {
		c.Disconnected(context.Background(), realtime.Session{ConnectionID: binding.ConnectionID})
	}
	return err
}

// emit sends a trusted lifecycle event through the component-owned bounded delivery path.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - binding: Verified account, session and media-connection identity.
//   - kind: Validated event, job, media or resource category.
//   - replyTo: Validated client correlation identifier for the error response.
//   - data: Input bytes or structured event payload.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Controller) emit(ctx context.Context, binding domain.Binding, kind, replyTo string, data any) error {
	event := realtime.Event(kind, binding.ConferenceID, data)
	event.ReplyTo = replyTo
	if err := c.publisher.Publish(ctx, realtime.Bus{Kind: "event", ConferenceID: binding.ConferenceID, ConnectionID: binding.ConnectionID, Event: &event}); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}

// Disconnected handles closure of a physical connection and releases only the resources owned by
// that connection.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - session: Persistent or physical session state, as defined by its type.
func (c *Controller) Disconnected(ctx context.Context, session realtime.Session) {
	c.mu.Lock()
	peer, exists := c.peers[session.ConnectionID]
	delete(c.peers, session.ConnectionID)
	c.mu.Unlock()
	if !exists {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, min(c.cfg.OperationTimeout, 3*time.Second))
	defer cancel()
	_, _ = c.transport.Call(ctx, "leave", domain.Command{RequestID: uuid.NewString(), Binding: peer.binding, Route: peer.route, MediaPeerID: peer.peerID})
}

// strictDecode decodes exactly one JSON value and rejects unknown fields or trailing input.
//
// Parameters:
//   - raw: Untrusted serialized value to decode or validate.
//   - target: Target state, account or decoded value, as defined by its type.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func strictDecode(raw []byte, target any) error {
	if !utf8.Valid(raw) {
		return domain.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil {
		return domain.ErrInvalid
	}
	if err := d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.ErrInvalid
	}
	return nil
}

// validUUID checks that an identifier is a nonzero UUID before it is used in storage or routing.
//
// Parameters:
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func validUUID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed != uuid.Nil && parsed.String() == id
}

// decode strictly decodes the payload of an allowed media-signaling command.
//
// Parameters:
//   - kind: Validated event, job, media or resource category.
//   - raw: Untrusted serialized value to decode or validate.
//
// Returns:
//   - result 1: The resulting signal.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Controller) decode(kind string, raw json.RawMessage) (domain.Signal, error) {
	var signal domain.Signal
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return signal, domain.ErrInvalid
	}
	if strictDecode(raw, &signal) != nil {
		return signal, domain.ErrInvalid
	}
	if kind != "media.offer" && signal.Publications != nil {
		return signal, domain.ErrInvalid
	}
	if kind == "media.join" {
		if signal.MediaPeerID != "" || signal.NegotiationID != "" || signal.SDP != "" || len(signal.Candidate) != 0 || signal.TrackID != "" {
			return signal, domain.ErrInvalid
		}
		return signal, nil
	}
	if !validUUID(signal.MediaPeerID) && !(kind == "media.leave" && signal.MediaPeerID == "") {
		return signal, domain.ErrInvalid
	}
	switch kind {
	case "media.offer":
		if !validUUID(signal.NegotiationID) || signal.SDP == "" || len(signal.SDP) > c.limits.SDPBytes || len(signal.Candidate) != 0 || signal.TrackID != "" {
			return signal, domain.ErrInvalid
		}
		if len(signal.Publications) > 4 {
			return signal, domain.ErrLimit
		}
		mids := map[string]bool{}
		sources := map[domain.Source]bool{}
		for _, pub := range signal.Publications {
			if pub.MID == "" || len(pub.MID) > 64 || strings.ContainsAny(pub.MID, "\r\n\x00 ") || len(pub.TrackID) > 128 || strings.ContainsAny(pub.TrackID, "\r\n\x00") || domain.SourceKind(pub.Source) == "" || mids[pub.MID] || sources[pub.Source] {
				return signal, domain.ErrInvalid
			}
			mids[pub.MID] = true
			sources[pub.Source] = true
		}
	case "media.ready":
		if !validUUID(signal.NegotiationID) || signal.SDP != "" || len(signal.Candidate) != 0 || signal.TrackID != "" {
			return signal, domain.ErrInvalid
		}
	case "media.ice":
		if signal.NegotiationID != "" || signal.SDP != "" || signal.TrackID != "" || len(signal.Candidate) == 0 || len(signal.Candidate) > c.limits.ICEBytes {
			return signal, domain.ErrInvalid
		}
		if string(signal.Candidate) != "null" {
			var ice struct {
				Candidate        string  `json:"candidate"`
				SDPMid           *string `json:"sdpMid"`
				SDPMLineIndex    *uint16 `json:"sdpMLineIndex"`
				UsernameFragment *string `json:"usernameFragment"`
			}
			if strictDecode(signal.Candidate, &ice) != nil || strings.ContainsAny(ice.Candidate, "\r\n\x00") || (ice.SDPMid != nil && len(*ice.SDPMid) > 128) || (ice.UsernameFragment != nil && len(*ice.UsernameFragment) > 256) {
				return signal, domain.ErrInvalid
			}
		}
	case "media.leave":
		if signal.NegotiationID != "" || signal.SDP != "" || len(signal.Candidate) != 0 || signal.TrackID != "" {
			return signal, domain.ErrInvalid
		}
	case "media.unpublish":
		if signal.NegotiationID != "" || signal.SDP != "" || len(signal.Candidate) != 0 || len(signal.TrackID) < 1 || len(signal.TrackID) > 128 || strings.ContainsAny(signal.TrackID, "\r\n\x00") {
			return signal, domain.ErrInvalid
		}
	default:
		return signal, domain.ErrInvalid
	}
	return signal, nil
}
