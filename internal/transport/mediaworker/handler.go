// Package mediaworker exposes only authenticated internal signaling. RTP never passes through
// these HTTP handlers.
package mediaworker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/janickiy/meet-space/internal/domain/realtime"
	"github.com/pion/webrtc/v4"
)

// Engine defines the replaceable dependency contract for mediaworker.
type Engine interface {
	// Join creates or resumes the authorized membership or media-room connection, preserving its
	// current identity.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Verified account, session and media-connection identity.
	//
	// Returns:
	//   - result 1: The resulting peer view.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Join(context.Context, media.Binding) (media.PeerView, error)
	// Offer processes an SDP offer for the currently bound WebRTC connection.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Identifier correlating this offer with its negotiation cycle.
	//   - argument 4: Untrusted serialized value to decode or validate.
	//
	// Returns:
	//   - result 1: The resulting session description.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Offer(context.Context, string, string, string) (webrtc.SessionDescription, error)
	// Ready accepts confirmation that the client applied the SDP answer and enables tracks only for
	// the matching current negotiation.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Identifier correlating this offer with its negotiation cycle.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Ready(context.Context, string, string) error
	// ICE applies a validated ICE candidate to the current media connection.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Candidate session or ICE record to validate before use.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ICE(context.Context, string, *webrtc.ICECandidateInit) error
	// Unpublish stops publishing the specified authorized media source.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Identifier of the published media track.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Unpublish(context.Context, string, string) error
	// Leave removes the active membership or media connection while preserving the saved
	// conversation or conference history.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Leave(context.Context, string) error
	// LeaveConnection disconnects only the specified physical media connection.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Physical media-connection identifier.
	LeaveConnection(context.Context, string)
	// PeerBinding returns the server-side session identity bound to a peer connection.
	//
	// Parameters:
	//   - argument 1: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting binding.
	//   - result 2: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	PeerBinding(string) (media.Binding, bool)
	// Bindings returns the current peer identity bindings for the requested media room.
	//
	// Returns:
	//   - result 1: The collected binding values.
	Bindings() []media.Binding
	// Tracks returns a snapshot of the requested room's visible published tracks.
	//
	// Parameters:
	//   - argument 1: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The collected track values.
	Tracks(string) []media.Track
	// CloseConference closes the media peers and owned resources of one conference.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	CloseConference(context.Context, string) error
	// Shutdown stops new work and waits for the component-owned resources to finish closing.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Shutdown(context.Context) error
}

// Registry defines the replaceable dependency contract for mediaworker.
type Registry interface {
	// RegisterWorker saves the media worker's current address, capacity and bounded presence
	// lifetime.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Live media-worker metadata and ownership identity.
	//   - argument 3: Lifetime assigned to the temporary value or ownership record.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	RegisterWorker(context.Context, media.Worker, time.Duration) error
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
	GetOwner(context.Context, string) (media.Route, error)
	// Renew extends the mediaworker lease only while its owner and fencing identity still match.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//   - argument 3: Current media-room owner and fencing version.
	//   - argument 4: Lifetime assigned to the temporary value or ownership record.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Renew(context.Context, string, media.Route, time.Duration) error
	// Release releases only the mediaworker resource owned by the matching operation or token.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//   - argument 3: Current media-room owner and fencing version.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Release(context.Context, string, media.Route) error
	// RemoveWorker removes the specified media worker's presence from the registry.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identity of the worker owning this operation.
	//   - argument 3: Operator-configured service endpoint.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	RemoveWorker(context.Context, string, string) error
}

// Sessions defines the replaceable dependency contract for mediaworker.
type Sessions interface {
	// Get loads the requested mediaworker within the caller's authorized scope.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting session.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Get(context.Context, string) (realtime.Session, error)
}

// Tickets defines the replaceable dependency contract for mediaworker.
type Tickets interface {
	// Verify validates the supplied credentials or signed binding before accepting the caller's
	// identity.
	//
	// Parameters:
	//   - argument 1: Untrusted serialized value to decode or validate.
	//
	// Returns:
	//   - result 1: The resulting binding.
	//   - result 2: The resulting route.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Verify(string) (media.Binding, media.Route, error)
}

// offerCache retains a negotiated SDP result so repeated offers can be processed safely.
type offerCache struct {
	mu     sync.Mutex
	id     string
	hash   [32]byte
	answer string
}

// roomGate serializes control operations for one media room.
type roomGate struct {
	token chan struct{}
	refs  int
}

// roomLease contains the worker's local ownership confirmation and its expiration boundary.
type roomLease struct {
	route    media.Route
	deadline time.Time
}

// Handler adapts HTTP requests to mediaworker, validates input and writes the response.
type Handler struct {
	registrationMu sync.Mutex
	draining       bool
	cfg            config.MediaConfig
	registry       Registry
	sessions       Sessions
	tickets        Tickets
	engine         Engine
	logger         *slog.Logger
	metrics        func() any
	ready          atomic.Bool
	gate           chan struct{}
	mu             sync.Mutex
	leases         map[string]roomLease
	offers         map[string]*offerCache
	roomGates      map[string]*roomGate
	workerDeadline time.Time
	fencing        bool
	closing        bool
}

// NewHandler constructs the handler with the dependencies and limits required for mediaworker.
//
// Parameters:
//   - cfg: Operator-controlled configuration and processing limits.
//   - registry: Dependency or metrics registry used to locate or register the component.
//   - sessions: Physical-session authorization dependency.
//   - tickets: One-use or signed media-ticket issuer and verifier.
//   - engine: Media engine that owns the room and peer resources.
//   - metrics: Callback returning bounded operational metrics.
//   - logger: Bounded diagnostic logger.
//
// Returns:
//   - result 1: The constructed or loaded handler.
func NewHandler(cfg config.MediaConfig, registry Registry, sessions Sessions, tickets Tickets, engine Engine, metrics func() any, logger *slog.Logger) *Handler {
	return &Handler{cfg: cfg, registry: registry, sessions: sessions, tickets: tickets, engine: engine, metrics: metrics, logger: logger, gate: make(chan struct{}, 128), leases: make(map[string]roomLease), offers: make(map[string]*offerCache), roomGates: make(map[string]*roomGate)}
}

// lockRoom serializes a control operation for one media room and returns its unlock callback.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The callback used to complete or release the operation.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Handler) lockRoom(ctx context.Context, id string) (func(), error) {
	if ctx.Err() != nil {
		return nil, media.ErrUnavailable
	}
	h.mu.Lock()
	gate := h.roomGates[id]
	if gate == nil {
		gate = &roomGate{token: make(chan struct{}, 1)}
		gate.token <- struct{}{}
		h.roomGates[id] = gate
	}
	gate.refs++
	h.mu.Unlock()

	releaseRef := func() {
		h.mu.Lock()
		gate.refs--
		if gate.refs == 0 {
			delete(h.roomGates, id)
		}
		h.mu.Unlock()
	}
	select {
	case <-ctx.Done():
		releaseRef()
		return nil, media.ErrUnavailable
	case <-gate.token:

		return func() { gate.token <- struct{}{}; releaseRef() }, nil
	}
}

// Routes builds the media worker's internal HTTP router with authentication and readiness
// checks.
//
// Returns:
//   - result 1: The resulting handler.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz",

		func(w http.ResponseWriter, _ *http.Request) {
			if !h.Ready() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	mux.HandleFunc("GET /internal/media/metrics", h.auth(

		func(w http.ResponseWriter, _ *http.Request) { h.json(w, http.StatusOK, h.metrics()) }))
	for _, operation := range []string{"join", "offer", "ready", "ice", "leave", "unpublish", "policy", "close"} {
		op := operation
		mux.HandleFunc("POST /internal/media/"+op, h.auth(

			func(w http.ResponseWriter, r *http.Request) { h.command(w, r, op) }))
	}
	mux.HandleFunc("POST /internal/media/egress", h.auth(h.egress))
	return mux
}

// auth verifies the protected internal API-to-media-worker request credential.
//
// Parameters:
//   - next: Next HTTP handler or newer event, as defined by its type.
//
// Returns:
//   - result 1: The HTTP handler to register for this operation.
func (h *Handler) auth(next http.HandlerFunc) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if provided == r.Header.Get("Authorization") || subtle.ConstantTimeCompare([]byte(provided), []byte(h.cfg.InternalSecret)) != 1 {
			h.fail(w, media.ErrUnauthorized)
			return
		}
		next(w, r)
	}
}

// strictJSON decodes bounded JSON with unknown-field and trailing-data rejection.
//
// Parameters:
//   - raw: Untrusted serialized value to decode or validate.
//   - target: Target state, account or decoded value, as defined by its type.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func strictJSON(raw []byte, target any) error {
	if !utf8.Valid(raw) {
		return media.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return media.ErrInvalid
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return media.ErrInvalid
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

// sameBinding compares the full server-side identity of two media bindings.
//
// Parameters:
//   - a: The binding value being compared.
//   - b: The binding value being compared.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func sameBinding(a, b media.Binding) bool {
	return a.ConferenceID == b.ConferenceID && a.ParticipantID == b.ParticipantID && a.SessionID == b.SessionID && a.ConnectionID == b.ConnectionID && a.UserID == b.UserID && a.AuthorizationExpiresAt.Equal(b.AuthorizationExpiresAt)
}

// sameRoute compares the media-room owner and fencing version of two routing records.
//
// Parameters:
//   - a: The route value being compared.
//   - b: The route value being compared.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func sameRoute(a, b media.Route) bool {
	return a.WorkerID == b.WorkerID && a.Endpoint == b.Endpoint && a.LeaseID == b.LeaseID
}

// active checks whether the active resource still has the required session or ownership
// identity.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - binding: Verified account, session and media-connection identity.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Handler) active(ctx context.Context, binding media.Binding) error {
	if !binding.AuthorizationExpiresAt.After(time.Now()) {
		return media.ErrUnauthorized
	}
	s, err := h.sessions.Get(ctx, binding.ConnectionID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, media.ErrUnauthorized) || errors.Is(err, media.ErrPeerNotFound) {
			return media.ErrUnauthorized
		}
		return media.ErrUnavailable
	}
	if s.ID != binding.SessionID || s.ConferenceID != binding.ConferenceID || s.ParticipantID != binding.ParticipantID || s.ConnectionID != binding.ConnectionID || s.UserID != binding.UserID || s.Status != "connected" {
		return media.ErrUnauthorized
	}
	return nil
}

// command strictly decodes and executes an allowed internal media control command.
//
// Parameters:
//   - w: Destination HTTP response writer.
//   - r: Incoming HTTP request.
//   - operation: Fixed allowlisted provider or protocol operation.
func (h *Handler) command(w http.ResponseWriter, r *http.Request, operation string) {
	select {
	case h.gate <- struct{}{}:
		defer func() { <-h.gate }()
	default:
		h.fail(w, media.ErrLimit)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, media.MaxCommandBytes)
	raw, err := io.ReadAll(r.Body)
	var cmd media.Command
	if err != nil || strictJSON(raw, &cmd) != nil || !validUUID(cmd.RequestID) || r.Header.Get("X-Request-ID") != cmd.RequestID {
		h.fail(w, media.ErrInvalid)
		return
	}
	w.Header().Set("X-Request-ID", cmd.RequestID)
	if operation == "policy" || operation == "close" {
		h.serviceCommand(w, r, operation, cmd)
		return
	}
	for _, id := range []string{cmd.Binding.ConferenceID, cmd.Binding.ParticipantID, cmd.Binding.SessionID, cmd.Binding.ConnectionID, cmd.Binding.UserID, cmd.Route.LeaseID} {
		if !validUUID(id) {
			h.fail(w, media.ErrInvalid)
			return
		}
	}
	if cmd.Route.WorkerID != h.cfg.WorkerID || cmd.Route.Endpoint != h.cfg.WorkerInternalURL {
		h.fail(w, media.ErrOwnership)
		return
	}
	if operation != "join" && !validUUID(cmd.MediaPeerID) {
		h.fail(w, media.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.OperationTimeout)
	defer cancel()
	if operation == "join" || operation == "leave" {
		unlock, err := h.lockRoom(ctx, cmd.Binding.ConferenceID)
		if err != nil {
			h.fail(w, err)
			return
		}
		defer unlock()
	}
	if operation == "leave" {
		// Expired authorization or lost ownership must not prevent resource cleanup.
		if binding, ok := h.engine.PeerBinding(cmd.MediaPeerID); ok && !sameBinding(binding, cmd.Binding) {
			h.fail(w, media.ErrUnauthorized)
			return
		}
		err = h.engine.Leave(ctx, cmd.MediaPeerID)
		h.mu.Lock()
		delete(h.offers, cmd.MediaPeerID)
		h.mu.Unlock()
		if err != nil {
			h.fail(w, err)
			return
		}
		// Periodic cleanup releases an empty room under the same room lock.
		h.json(w, http.StatusOK, media.Result{MediaPeerID: cmd.MediaPeerID})
		return
	}
	if !h.ready.Load() {
		h.fail(w, media.ErrUnavailable)
		return
	}
	owner, err := h.registry.GetOwner(ctx, cmd.Binding.ConferenceID)
	if err != nil || !sameRoute(owner, cmd.Route) {
		h.fail(w, media.ErrOwnership)
		return
	}
	if err := h.active(ctx, cmd.Binding); err != nil {
		h.fail(w, err)
		return
	}
	if operation == "join" {
		if cmd.Policy != nil && (cmd.Policy.Version < 0 || cmd.Policy.Kicked) {
			h.fail(w, media.ErrPolicy)
			return
		}
		binding, route, err := h.tickets.Verify(cmd.Ticket)
		if err != nil || !sameBinding(binding, cmd.Binding) || !sameRoute(route, cmd.Route) {
			h.fail(w, media.ErrUnauthorized)
			return
		}
		renewedAt := time.Now()
		if err := h.registry.Renew(ctx, binding.ConferenceID, route, h.cfg.OwnershipTTL); err != nil {
			h.fail(w, media.ErrOwnership)
			return
		}
		// Reassigning a room to this process must not reuse the previous lease's state. Serialize join
		// and leave operations for that room.
		h.mu.Lock()
		old, existed := h.leases[binding.ConferenceID]
		h.mu.Unlock()
		if existed && !sameRoute(old.route, route) {
			_ = h.engine.CloseConference(ctx, binding.ConferenceID)
		}
		h.mu.Lock()
		admitted := !h.draining && !h.fencing && !h.closing && h.workerDeadline.After(time.Now()) && renewedAt.Add(h.cfg.OwnershipTTL).After(time.Now())
		if admitted {
			h.leases[binding.ConferenceID] = roomLease{route: route, deadline: renewedAt.Add(h.cfg.OwnershipTTL)}
		}
		h.mu.Unlock()
		if !admitted {
			h.fail(w, media.ErrUnavailable)
			return
		}
		// A valid leave and rejoin carries a newly saved policy. Apply it to an existing room before
		// admission and again to a newly created room.
		if cmd.Policy != nil {
			engine, ok := h.engine.(interface {
				// SetPolicy updates policy for mediaworker.
				//
				// Parameters:
				//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
				//   - argument 2: Conference identifier limiting the operation scope.
				//   - argument 3: Conference participant identifier.
				//   - argument 4: Effective server-side media policy.
				//
				// Returns:
				//   - result 1: nil on success; otherwise the validation, access, transport or persistence
				//     failure.
				SetPolicy(context.Context, string, string, media.ParticipantPolicy) error
			})
			if !ok {
				h.fail(w, media.ErrUnavailable)
				return
			}
			if err = engine.SetPolicy(ctx, binding.ConferenceID, binding.ParticipantID, *cmd.Policy); err != nil {
				h.fail(w, err)
				return
			}
		}
		peer, err := h.engine.Join(ctx, binding)
		if err != nil {
			h.fail(w, err)
			return
		}
		if cmd.Policy != nil {
			if policyEngine, ok := h.engine.(interface {
				// SetPolicy updates policy for mediaworker.
				//
				// Parameters:
				//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
				//   - argument 2: Conference identifier limiting the operation scope.
				//   - argument 3: Conference participant identifier.
				//   - argument 4: Effective server-side media policy.
				//
				// Returns:
				//   - result 1: nil on success; otherwise the validation, access, transport or persistence
				//     failure.
				SetPolicy(context.Context, string, string, media.ParticipantPolicy) error
			}); ok {
				if err = policyEngine.SetPolicy(ctx, binding.ConferenceID, binding.ParticipantID, *cmd.Policy); err != nil {
					_ = h.engine.Leave(ctx, peer.MediaPeerID)
					h.fail(w, err)
					return
				}
			} else {
				_ = h.engine.Leave(ctx, peer.MediaPeerID)
				h.fail(w, media.ErrUnavailable)
				return
			}
		}
		if !h.leaseValid(binding.ConferenceID, route) {
			_ = h.engine.Leave(ctx, peer.MediaPeerID)
			h.fail(w, media.ErrOwnership)
			return
		}
		h.logger.Info("media peer admitted", "conference_id", binding.ConferenceID, "participant_id", binding.ParticipantID, "session_id", binding.SessionID, "media_peer_id", peer.MediaPeerID, "worker_id", h.cfg.WorkerID, "request_id", cmd.RequestID)
		policy := cmd.Policy
		if provider, ok := h.engine.(interface {
			// ParticipantPolicy returns the participant's current effective media restrictions.
			//
			// Parameters:
			//   - argument 1: Conference identifier limiting the operation scope.
			//   - argument 2: Conference participant identifier.
			//
			// Returns:
			//   - result 1: The resulting participant policy.
			//   - result 2: Whether the operation's documented condition holds or its conditional state
			//     change succeeded.
			ParticipantPolicy(string, string) (media.ParticipantPolicy, bool)
		}); ok {
			if current, exists := provider.ParticipantPolicy(binding.ConferenceID, binding.ParticipantID); exists {
				policy = &current
			}
		}
		ice := h.cfg.TURN.ClientICE(h.cfg.ICE, time.Now())
		h.json(w, http.StatusOK, media.Result{MediaPeerID: peer.MediaPeerID, WorkerID: h.cfg.WorkerID, MaxPeers: h.cfg.MaxPeers, ICEServers: ice.ICEServers, ICETransportPolicy: ice.ICETransportPolicy, ICEExpiresAt: ice.ExpiresAt, Tracks: h.engine.Tracks(peer.MediaPeerID), Policy: policy, VideoCapture: media.VideoCaptureTarget{MaxWidth: h.cfg.VideoMaxWidth, MaxHeight: h.cfg.VideoMaxHeight, MaxFrameRate: h.cfg.VideoMaxFPS}})
		return
	}
	binding, ok := h.engine.PeerBinding(cmd.MediaPeerID)
	if !ok {
		h.fail(w, media.ErrPeerNotFound)
		return
	}
	if !sameBinding(binding, cmd.Binding) {
		h.fail(w, media.ErrUnauthorized)
		return
	}
	result := media.Result{MediaPeerID: cmd.MediaPeerID}
	switch operation {
	case "offer":
		if !validUUID(cmd.NegotiationID) || len(cmd.SDP) == 0 || len(cmd.SDP) > media.MaxSDPBytes {
			h.fail(w, media.ErrInvalid)
			return
		}
		if cmd.Policy != nil {
			policyEngine, ok := h.engine.(interface {
				// SetPolicy updates policy for mediaworker.
				//
				// Parameters:
				//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
				//   - argument 2: Conference identifier limiting the operation scope.
				//   - argument 3: Conference participant identifier.
				//   - argument 4: Effective server-side media policy.
				//
				// Returns:
				//   - result 1: nil on success; otherwise the validation, access, transport or persistence
				//     failure.
				SetPolicy(context.Context, string, string, media.ParticipantPolicy) error
			})
			if !ok {
				h.fail(w, media.ErrUnavailable)
				return
			}
			if err = policyEngine.SetPolicy(ctx, binding.ConferenceID, binding.ParticipantID, *cmd.Policy); err != nil {
				h.fail(w, err)
				return
			}
			if cmd.Policy.Kicked {
				h.fail(w, media.ErrPolicy)
				return
			}
		}
		h.mu.Lock()
		cache := h.offers[cmd.MediaPeerID]
		if cache == nil {
			cache = &offerCache{}
			h.offers[cmd.MediaPeerID] = cache
		}
		h.mu.Unlock()
		cache.mu.Lock()
		metadata, _ := json.Marshal(cmd.Publications)
		hash := sha256.Sum256(append(append([]byte(cmd.SDP), 0), metadata...))
		if cache.id == cmd.NegotiationID {
			if cache.hash != hash {
				err = media.ErrNegotiation
			} else {
				result.SDP = cache.answer
			}
		} else {
			var answer webrtc.SessionDescription
			if engine, ok := h.engine.(interface {
				// OfferSources processes an SDP offer with its explicit media-source declarations.
				//
				// Parameters:
				//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
				//   - argument 2: Identifier of the resource addressed by this operation.
				//   - argument 3: Identifier correlating this offer with its negotiation cycle.
				//   - argument 4: Untrusted serialized value to decode or validate.
				//   - argument 5: Validated declarations of the peer's media sources.
				//
				// Returns:
				//   - result 1: The resulting session description.
				//   - result 2: nil on success; otherwise the validation, access, transport or persistence
				//     failure.
				OfferSources(context.Context, string, string, string, []media.Publication) (webrtc.SessionDescription, error)
			}); ok {
				answer, err = engine.OfferSources(ctx, cmd.MediaPeerID, cmd.NegotiationID, cmd.SDP, cmd.Publications)
			} else if cmd.Publications != nil {
				err = media.ErrInvalid
			} else {
				answer, err = h.engine.Offer(ctx, cmd.MediaPeerID, cmd.NegotiationID, cmd.SDP)
			}
			if err == nil {
				result.SDP = answer.SDP
				cache.id, cache.hash, cache.answer = cmd.NegotiationID, hash, answer.SDP
			}
		}
		cache.mu.Unlock()
		result.NegotiationID = cmd.NegotiationID
	case "ready":
		if !validUUID(cmd.NegotiationID) || cmd.SDP != "" || len(cmd.Candidate) != 0 || cmd.TrackID != "" {
			h.fail(w, media.ErrInvalid)
			return
		}
		h.mu.Lock()
		cache := h.offers[cmd.MediaPeerID]
		h.mu.Unlock()
		if cache == nil {
			h.fail(w, media.ErrNegotiation)
			return
		}
		cache.mu.Lock()
		if cache.id != cmd.NegotiationID {
			err = media.ErrNegotiation
		} else {
			err = h.engine.Ready(ctx, cmd.MediaPeerID, cmd.NegotiationID)
		}
		cache.mu.Unlock()
	case "ice":
		if len(cmd.Candidate) == 0 || len(cmd.Candidate) > media.MaxICEBytes {
			h.fail(w, media.ErrInvalid)
			return
		}
		var candidate *webrtc.ICECandidateInit
		if string(cmd.Candidate) != "null" {
			candidate = &webrtc.ICECandidateInit{}
			if strictJSON(cmd.Candidate, candidate) != nil {
				h.fail(w, media.ErrInvalid)
				return
			}
		}
		err = h.engine.ICE(ctx, cmd.MediaPeerID, candidate)
	case "unpublish":
		if !validUUID(cmd.TrackID) {
			h.fail(w, media.ErrInvalid)
			return
		}
		err = h.engine.Unpublish(ctx, cmd.MediaPeerID, cmd.TrackID)
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	h.json(w, http.StatusOK, result)
}

// fail records a safe mediaworker failure and starts the owned-resource cleanup path.
//
// Parameters:
//   - w: Destination HTTP response writer.
//   - err: Error to classify, wrap or report safely.
func (h *Handler) fail(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, media.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, media.ErrLimit):
		status = http.StatusTooManyRequests
	case errors.Is(err, media.ErrPeerNotFound):
		status = http.StatusNotFound
	case errors.Is(err, media.ErrOwnership), errors.Is(err, media.ErrNegotiation), errors.Is(err, media.ErrScreenConflict):
		status = http.StatusConflict
	case errors.Is(err, media.ErrPolicy):
		status = http.StatusForbidden
	case errors.Is(err, media.ErrUnavailable):
		status = http.StatusServiceUnavailable
	}
	h.json(w, status, map[string]string{"code": media.ErrorCode(err)})
}

// json serializes the payload as a JSON response with the selected HTTP status.
//
// Parameters:
//   - w: Destination HTTP response writer.
//   - status: Lifecycle state or response status to record.
//   - data: Input bytes or structured event payload.
func (h *Handler) json(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// Start starts mediaworker and prepares the resources owned by that operation.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: The bounded event or media channel owned by this operation.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Handler) Start(ctx context.Context) (<-chan struct{}, error) {
	startedAt := time.Now()
	operation, cancel := context.WithTimeout(ctx, h.cfg.OperationTimeout)
	err := h.registry.RegisterWorker(operation, media.Worker{ID: h.cfg.WorkerID, Endpoint: h.cfg.WorkerInternalURL}, h.cfg.WorkerTTL)
	cancel()
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	h.workerDeadline = startedAt.Add(h.cfg.WorkerTTL)
	h.updateReadyLocked()
	h.mu.Unlock()
	done := make(chan struct{})
	var workers sync.WaitGroup

	run := func(interval time.Duration, tick func(context.Context)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					h.ready.Store(false)
					return
				case <-ticker.C:
					tick(ctx)
				}
			}
		}()
	}
	run(h.cfg.HeartbeatInterval, h.heartbeat)
	run(h.cfg.SessionCheckInterval, h.sweep)
	fenceInterval := min(h.cfg.WorkerTTL, h.cfg.OwnershipTTL) / 4
	fenceInterval = max(10*time.Millisecond, min(time.Second, fenceInterval))
	run(fenceInterval, h.watchdog)
	go func() { workers.Wait(); close(done) }()
	return done, nil
}

// updateReadyLocked refreshes the worker readiness flag while its state lock is held.
func (h *Handler) updateReadyLocked() {
	now := time.Now()
	valid := !h.fencing && !h.closing && h.workerDeadline.After(now)
	for _, lease := range h.leases {
		if !lease.deadline.After(now) {
			valid = false
			break
		}
	}
	h.ready.Store(valid)
}

// leaseValid checks that the room still belongs to this worker with the expected fencing
// version.
//
// Parameters:
//   - conferenceID: Conference identifier limiting the operation scope.
//   - route: Current media-room owner and fencing version.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (h *Handler) leaseValid(conferenceID string, route media.Route) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	lease, ok := h.leases[conferenceID]
	return ok && !h.fencing && !h.closing && h.workerDeadline.After(time.Now()) && lease.deadline.After(time.Now()) && sameRoute(lease.route, route)
}

// watchdog closes media resources that have lost their authorization or distributed ownership.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
func (h *Handler) watchdog(ctx context.Context) {
	h.mu.Lock()
	now := time.Now()
	expired := !h.fencing && !h.closing && !h.workerDeadline.After(now)
	for _, lease := range h.leases {
		if !lease.deadline.After(now) {
			expired = true
			break
		}
	}
	h.mu.Unlock()
	if expired {
		h.fence(ctx, "local_lease_expired")
	}
}

// fence stops a stale worker from continuing media processing after ownership is lost.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - reason: Bounded lifecycle or failure reason.
func (h *Handler) fence(ctx context.Context, reason string) {
	h.mu.Lock()
	if h.fencing || h.closing {
		h.mu.Unlock()
		return
	}
	if reason == "local_lease_expired" {
		now := time.Now()
		expired := !h.workerDeadline.After(now)
		for _, lease := range h.leases {
			if !lease.deadline.After(now) {
				expired = true
				break
			}
		}
		if !expired {
			h.mu.Unlock()
			return
		} // A concurrent renewal completed first.
	}
	h.fencing = true
	h.ready.Store(false)
	h.workerDeadline = time.Time{}
	rooms := make(map[string]bool)
	for id := range h.leases {
		rooms[id] = true
	}
	h.leases = make(map[string]roomLease)
	h.mu.Unlock()
	h.logger.Warn("media worker fenced", "worker_id", h.cfg.WorkerID, "event_type", reason)
	for _, binding := range h.engine.Bindings() {
		rooms[binding.ConferenceID] = true
	}
	var closed sync.WaitGroup
	for id := range rooms {
		closed.Add(1)
		go func(id string) { defer closed.Done(); _ = h.engine.CloseConference(ctx, id) }(id)
	}
	closed.Wait()
	h.mu.Lock()
	h.offers = make(map[string]*offerCache)
	h.fencing = false
	// The worker becomes ready again only after a successful heartbeat beyond the previous lease
	// boundary.
	h.ready.Store(false)
	h.mu.Unlock()
}

// heartbeat periodically renews the worker's distributed presence.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
func (h *Handler) heartbeat(ctx context.Context) {
	h.registrationMu.Lock()
	defer h.registrationMu.Unlock()
	h.mu.Lock()
	draining := h.draining
	h.mu.Unlock()
	operation, cancel := context.WithTimeout(ctx, h.cfg.OperationTimeout)
	defer cancel()
	startedAt := time.Now()
	if err := h.registry.RegisterWorker(operation, media.Worker{ID: h.cfg.WorkerID, Endpoint: h.cfg.WorkerInternalURL, Draining: draining}, h.cfg.WorkerTTL); err != nil {
		h.fence(operation, "registry_unavailable")
		return
	}
	h.mu.Lock()
	h.workerDeadline = startedAt.Add(h.cfg.WorkerTTL)
	leases := make(map[string]roomLease, len(h.leases))
	for id, lease := range h.leases {
		leases[id] = lease
	}
	h.mu.Unlock()
	for id, lease := range leases {
		renewedAt := time.Now()
		// Presence renewal must not wait for a slow room lock held during join or leave.
		if err := h.registry.Renew(operation, id, lease.route, h.cfg.OwnershipTTL); err != nil {
			if !errors.Is(err, media.ErrOwnership) {
				h.fence(operation, "registry_unavailable")
				return
			}
			unlock, err := h.lockRoom(operation, id)
			if err != nil {
				h.fence(operation, "lease_cleanup_timeout")
				return
			}
			h.mu.Lock()
			current, ok := h.leases[id]
			matches := ok && sameRoute(current.route, lease.route)
			if matches {
				delete(h.leases, id)
			}
			h.mu.Unlock()
			if matches {
				_ = h.engine.CloseConference(operation, id)
				h.logger.Warn("media room ownership lost", "conference_id", id, "worker_id", h.cfg.WorkerID)
			}
			unlock()
			continue
		}
		h.mu.Lock()
		if current, ok := h.leases[id]; ok && sameRoute(current.route, lease.route) {
			current.deadline = renewedAt.Add(h.cfg.OwnershipTTL)
			h.leases[id] = current
		}
		h.mu.Unlock()
	}
	h.releaseEmpty(operation)
	h.mu.Lock()
	h.updateReadyLocked()
	h.mu.Unlock()
}

// sweep removes expired sessions and closes resources that no longer have valid access.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
func (h *Handler) sweep(ctx context.Context) {
	operation, cancel := context.WithTimeout(ctx, h.cfg.OperationTimeout)
	defer cancel()
	for _, binding := range h.engine.Bindings() {
		if operation.Err() != nil {
			h.fence(operation, "session_sweep_timeout")
			return
		}
		err := h.active(operation, binding)
		if errors.Is(err, media.ErrUnavailable) {
			h.fence(operation, "session_store_unavailable")
			return
		}
		if err != nil {
			unlock, err := h.lockRoom(operation, binding.ConferenceID)
			if err != nil {
				h.fence(operation, "session_cleanup_timeout")
				return
			}
			h.engine.LeaveConnection(operation, binding.ConnectionID)
			unlock()
		}
	}
	h.releaseEmpty(operation)
}

// releaseEmpty releases ownership of media rooms that have no active connections.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
func (h *Handler) releaseEmpty(ctx context.Context) {
	h.mu.Lock()
	leases := make(map[string]roomLease, len(h.leases))
	for id, lease := range h.leases {
		leases[id] = lease
	}
	for id := range h.offers {
		if _, ok := h.engine.PeerBinding(id); !ok {
			delete(h.offers, id)
		}
	}
	h.mu.Unlock()
	for id, lease := range leases {
		unlock, err := h.lockRoom(ctx, id)
		if err != nil {
			return
		}
		active := false
		if engine, ok := h.engine.(interface {
			// HasConference reports whether this manager currently owns the requested conference room.
			//
			// Parameters:
			//   - argument 1: Identifier of the resource addressed by this operation.
			//
			// Returns:
			//   - result 1: Whether the operation's documented condition holds or its conditional state
			//     change succeeded.
			HasConference(string) bool
		}); ok {
			active = engine.HasConference(id)
		}
		for _, binding := range h.engine.Bindings() {
			if binding.ConferenceID == id {
				active = true
				break
			}
		}
		if !active {
			h.mu.Lock()
			if current, ok := h.leases[id]; ok && sameRoute(current.route, lease.route) {
				delete(h.leases, id)
			}
			h.mu.Unlock()
			_ = h.registry.Release(ctx, id, lease.route)
		}
		unlock()
	}
}

// Stop stops the active mediaworker operation and releases its owned resources.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Handler) Stop(ctx context.Context) error {
	h.mu.Lock()
	h.closing = true
	h.ready.Store(false)
	h.mu.Unlock()
	err := h.engine.Shutdown(ctx)
	h.releaseEmpty(ctx)
	_ = h.registry.RemoveWorker(ctx, h.cfg.WorkerID, h.cfg.WorkerInternalURL)
	return err
}

// Ready reports whether the component or configured dependency is currently usable.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (h *Handler) Ready() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ready.Load() && !h.draining
}

// BeginDrain stops accepting new work while existing operations finish.
func (h *Handler) BeginDrain() {
	h.mu.Lock()
	h.draining = true
	h.mu.Unlock()
	h.heartbeat(context.Background())
}

// ActiveRooms counts rooms with active participants so deployment can detect media that would be
// interrupted.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
func (h *Handler) ActiveRooms() int {
	rooms := make(map[string]struct{})
	for _, binding := range h.engine.Bindings() {
		rooms[binding.ConferenceID] = struct{}{}
	}
	return len(rooms) + len(h.gate)
}
