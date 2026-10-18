package realtime

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	domain "github.com/janickiy/meet-space/internal/domain/realtime"
)

// Repository defines the replaceable dependency contract for physical sessions and realtime
// event delivery.
type Repository interface {
	// Authorize checks the current identity and saved access state before physical sessions and
	// realtime event delivery.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//   - argument 3: Acting account identifier used to scope access.
	//
	// Returns:
	//   - result 1: The resulting participant.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Authorize(context.Context, string, string) (conferences.Participant, error)
	// Open opens the scoped media, recording or session resource required by this component.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Persistent or physical session state, as defined by its type.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Open(context.Context, domain.Session) error
	// Close closes the component-owned resources and ends their lifecycle.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Physical media-connection identifier.
	//   - argument 3: Reference timestamp of confirmed transport activity.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Close(context.Context, string, time.Time) error
	// Stale returns historical physical sessions that remain open beyond the liveness boundary.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Boundary before which results or state are selected.
	//   - argument 3: Opaque continuation boundary from the previous page.
	//
	// Returns:
	//   - result 1: The collected session values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Stale(context.Context, time.Time, string) ([]domain.Session, error)
	// Roster loads the conference and memberships needed to authorize the current room snapshot.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//
	// Returns:
	//   - result 1: The resulting status.
	//   - result 2: The collected participant values.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Roster(context.Context, string) (conferences.Status, []conferences.Participant, error)
}

// Store defines the replaceable dependency contract for physical sessions and realtime event
// delivery.
type Store interface {
	// Register registers the physical connection and its bounded presence lifetime.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Persistent or physical session state, as defined by its type.
	//   - argument 3: Lifetime assigned to the temporary value or ownership record.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Register(context.Context, domain.Session, time.Duration) error
	// Unregister closes the physical session and updates the participant's distributed presence.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Unregister(context.Context, string) error
	// Touch renews presence using the supplied confirmed transport activity time.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Timestamp of a transport-confirmed pong or other liveness acknowledgement.
	//   - argument 4: Lifetime assigned to the temporary value or ownership record.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Touch(context.Context, string, time.Time, time.Duration) error
	// Get loads the requested physical session within the caller's authorized scope.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting session.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Get(context.Context, string) (domain.Session, error)
	// Missing returns connections whose route has expired. An error must not be
	// interpreted as absence: reconciliation then leaves SQL sessions intact.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Resource identifiers to process as a bounded batch.
	//
	// Returns:
	//   - result 1: The collected string values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Missing(context.Context, []string) ([]string, error)
	// Active returns physical sessions whose presence has not expired.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier whose active sessions should be loaded.
	//
	// Returns:
	//   - result 1: The collected session values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Active(context.Context, string) ([]domain.Session, error)
	// Prune removes expired physical sessions and returns the state needed to reconcile presence.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Prune(context.Context) error
	// Publish publishes the saved physical session change through its configured event or command
	// transport.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Scoped publication and subscription transport.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Publish(context.Context, domain.Bus) error
	// Subscribe opens a scoped event subscription whose lifetime is bounded by the supplied context.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: The resulting subscription.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Subscribe(context.Context) (domain.Subscription, error)
}

// Socket defines the replaceable dependency contract for physical sessions and realtime event
// delivery.
type Socket interface {
	// Offer enqueues the event in the socket's bounded queue according to its priority and reports
	// whether it was accepted.
	//
	// Parameters:
	//   - argument 1: Trusted event to publish or process.
	//
	// Returns:
	//   - result 1: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	Offer(domain.Envelope) bool
	// Stop stops the active physical session operation and releases its owned resources.
	//
	// Parameters:
	//   - argument 1: Bounded reason code sent when closing the socket.
	Stop(string)
}

// DisconnectObserver defines the replaceable dependency contract for physical sessions and
// realtime event delivery.
type DisconnectObserver interface {
	// Disconnected handles closure of a physical connection and releases only the resources owned by
	// that connection.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Persistent or physical session state, as defined by its type.
	Disconnected(context.Context, domain.Session)
}

// DisconnectObservers combines bounded cleanup observers while preserving independent subsystem
// lifecycles.
type DisconnectObservers []DisconnectObserver

// Disconnected handles closure of a physical connection and releases only the resources owned by
// that connection.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - session: Persistent or physical session state, as defined by its type.
func (observers DisconnectObservers) Disconnected(ctx context.Context, session domain.Session) {
	for _, observer := range observers {
		if observer != nil {
			observer.Disconnected(ctx, session)
		}
	}
}

// localSocket binds a local transport to its physical session and lifetime state.
type localSocket struct {
	session domain.Session
	socket  Socket
	ready   bool
}

// Hub coordinates local sockets, distributed presence and access-filtered event delivery with
// independent priorities.
type Hub struct {
	repo               Repository
	store              Store
	ttl                time.Duration
	logger             *slog.Logger
	ctx                context.Context
	cancel             context.CancelFunc
	sub                domain.Subscription
	mu                 sync.Mutex
	closing            bool
	unavailable        bool
	generation         uint64
	local              map[string]*localSocket
	sockets            sync.WaitGroup
	workers            sync.WaitGroup
	disconnectObserver DisconnectObserver
	lowEvents          chan domain.Bus
	shutdownOnce       sync.Once // Start shutdown only once, even when callers repeat the request.
	shutdownDone       chan struct{}
}

// NewHub constructs the hub with the dependencies and limits required for physical sessions and
// realtime event delivery.
//
// Parameters:
//   - repo: Persistence dependency used by the application operation.
//   - store: Distributed physical-session and event store.
//   - ttl: Lifetime assigned to the temporary value or ownership record.
//   - logger: Bounded diagnostic logger.
//
// Returns:
//   - result 1: The constructed or loaded hub.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NewHub(repo Repository, store Store, ttl time.Duration, logger *slog.Logger) (*Hub, error) {
	ctx, cancel := context.WithCancel(context.Background())
	if logger == nil {
		logger = slog.Default()
	}
	h := &Hub{repo: repo, store: store, ttl: ttl, logger: logger, ctx: ctx, cancel: cancel, local: map[string]*localSocket{}, lowEvents: make(chan domain.Bus, 128)}
	op, c := context.WithTimeout(ctx, 5*time.Second)
	defer c()
	sub, err := store.Subscribe(op)
	if err != nil {
		cancel()
		return nil, err
	}
	h.sub = sub
	h.generation = 1
	h.workers.Add(3)
	go h.receive()
	go h.receiveLowPriority()
	go h.janitor()
	return h, nil
}

// Authorize checks the current identity and saved access state before physical sessions and
// realtime event delivery.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: The resulting participant.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Hub) Authorize(ctx context.Context, conferenceID, userID string) (conferences.Participant, error) {
	return h.repo.Authorize(ctx, conferenceID, userID)
}

// AuthorizeClient additionally scopes private calls to the initiating/accepting
// client. Ordinary conference repositories preserve their existing contract.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - userID: Acting account identifier used to scope access.
//   - clientID: Physical browser or socket client identifier.
//
// Returns:
//   - result 1: The resulting participant.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Hub) AuthorizeClient(ctx context.Context, conferenceID, userID, clientID string) (conferences.Participant, error) {
	if repo, ok := h.repo.(interface {
		// AuthorizeClient validates conference access for the specified account and physical client
		// identity.
		//
		// Parameters:
		//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
		//   - argument 2: Conference identifier limiting the operation scope.
		//   - argument 3: Acting account identifier used to scope access.
		//   - argument 4: Physical browser or socket client identifier.
		//
		// Returns:
		//   - result 1: The resulting participant.
		//   - result 2: nil on success; otherwise the validation, access, transport or persistence
		//     failure.
		AuthorizeClient(context.Context, string, string, string) (conferences.Participant, error)
	}); ok {
		return repo.AuthorizeClient(ctx, conferenceID, userID, clientID)
	}
	return h.repo.Authorize(ctx, conferenceID, userID)
}

// Check includes the live subscription in API readiness; a healthy Redis Ping
// alone does not establish that this Hub can receive conference events.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Hub) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closing || h.unavailable {
		return apperrors.ErrUnavailable
	}
	return nil
}

// SetDisconnectObserver updates disconnect observer for physical sessions and realtime event
// delivery.
//
// Parameters:
//   - observer: Observer notified after a persisted lifecycle change.
func (h *Hub) SetDisconnectObserver(observer DisconnectObserver) {
	h.mu.Lock()
	h.disconnectObserver = observer
	h.mu.Unlock()
}

// ValidateSession binds the media operation to a current, authorized physical WebSocket session.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - session: Persistent or physical session state, as defined by its type.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Hub) ValidateSession(ctx context.Context, session domain.Session) error {
	p, err := h.AuthorizeClient(ctx, session.ConferenceID, session.UserID, session.ClientID)
	if err != nil {
		return err
	}
	live, err := h.store.Get(ctx, session.ConnectionID)
	if err != nil {
		return err
	}
	if p.ID != session.ParticipantID || live.ID != session.ID || live.ConferenceID != session.ConferenceID || live.ParticipantID != session.ParticipantID || live.UserID != session.UserID || live.ConnectionID != session.ConnectionID || live.Status != "connected" {
		return apperrors.ErrForbidden
	}
	return nil
}

// Prepare prepares the authorized physical session state before media negotiation begins.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: The resulting session.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Hub) Prepare(ctx context.Context, conferenceID, userID string) (domain.Session, error) {
	return h.PrepareClient(ctx, conferenceID, userID, "")
}

// PrepareClient authorizes and prepares the physical client session before socket registration.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - userID: Acting account identifier used to scope access.
//   - clientID: Physical browser or socket client identifier.
//
// Returns:
//   - result 1: The resulting session.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Hub) PrepareClient(ctx context.Context, conferenceID, userID, clientID string) (domain.Session, error) {
	h.mu.Lock()
	closing := h.closing
	unavailable := h.unavailable
	h.mu.Unlock()
	if closing {
		return domain.Session{}, apperrors.ErrConflict
	}
	if unavailable {
		return domain.Session{}, apperrors.ErrUnavailable
	}
	p, err := h.AuthorizeClient(ctx, conferenceID, userID, clientID)
	if err != nil {
		return domain.Session{}, err
	}
	now := time.Now().UTC()
	s := domain.Session{ID: uuid.NewString(), ConferenceID: conferenceID, ParticipantID: p.ID, UserID: userID, ClientID: clientID, ConnectionID: uuid.NewString(), Status: "connected", ConnectedAt: now, LastSeenAt: now}
	return s, h.repo.Open(ctx, s)
}

// Abort closes a physical session whose registration failed and removes its distributed
// presence.
//
// Parameters:
//   - session: Persistent or physical session state, as defined by its type.
func (h *Hub) Abort(session domain.Session) {
	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	_ = h.repo.Close(ctx, session.ConnectionID, session.LastSeenAt)
}

// Register registers the physical connection and its bounded presence lifetime.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - session: Persistent or physical session state, as defined by its type.
//   - socket: Physical socket transport registered with its verified session.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Hub) Register(ctx context.Context, session domain.Session, socket Socket) error {
	h.mu.Lock()
	if h.closing || h.unavailable {
		err := apperrors.ErrConflict
		if h.unavailable && !h.closing {
			err = apperrors.ErrUnavailable
		}
		h.mu.Unlock()
		h.Abort(session)
		return err
	}
	h.sockets.Add(1)
	h.local[session.ConnectionID] = &localSocket{session: session, socket: socket}
	h.mu.Unlock()
	if err := h.store.Register(ctx, session, h.ttl); err != nil {
		h.Unregister(session)
		return err
	}
	// Handle conference completion or member departure racing the socket upgrade and Redis
	// registration.
	if _, err := h.AuthorizeClient(ctx, session.ConferenceID, session.UserID, session.ClientID); err != nil {
		h.Unregister(session)
		return err
	}
	state, err := h.state(ctx, session)
	if err == nil && !stateAllows(state, session.ParticipantID) {
		err = apperrors.ErrForbidden
	}
	if err != nil || !socket.Offer(domain.Event("conference.state", session.ConferenceID, stateFor(state, session))) {
		h.Unregister(session)
		if err != nil {
			return err
		}
		return apperrors.ErrConflict
	}
	h.mu.Lock()
	if entry := h.local[session.ConnectionID]; entry != nil {
		entry.ready = true
	}
	h.mu.Unlock()
	h.log(session, "connected")
	return nil
}

// Unregister closes the physical session and updates the participant's distributed presence.
//
// Parameters:
//   - session: Persistent or physical session state, as defined by its type.
func (h *Hub) Unregister(session domain.Session) {
	h.mu.Lock()
	_, exists := h.local[session.ConnectionID]
	delete(h.local, session.ConnectionID)
	observer := h.disconnectObserver
	h.mu.Unlock()
	if !exists {
		return
	}
	defer h.sockets.Done()
	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	seen := session.LastSeenAt
	if live, err := h.store.Get(ctx, session.ConnectionID); err == nil {
		seen = live.LastSeenAt
	}
	if err := h.store.Unregister(ctx, session.ConnectionID); err != nil {
		h.log(session, "redis_cleanup_failed")
	}
	if err := h.repo.Close(ctx, session.ConnectionID, seen); err != nil {
		h.log(session, "history_cleanup_failed")
	}
	// Observers see the session as already closed. Two tabs closing together cannot both mistake the
	// other for the last active session.
	if observer != nil {
		observer.Disconnected(ctx, session)
	}
	h.log(session, "disconnected")
}

// Touch renews presence using the supplied confirmed transport activity time.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - session: Persistent or physical session state, as defined by its type.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Hub) Touch(ctx context.Context, session domain.Session) error {
	if _, err := h.AuthorizeClient(ctx, session.ConferenceID, session.UserID, session.ClientID); err != nil {
		return err
	}
	// Membership checks must not add time to the five-second liveness window. LastSeenAt comes only
	// from a transport-confirmed received pong.
	remaining := h.ttl - time.Since(session.LastSeenAt)
	if session.LastSeenAt.IsZero() || remaining <= 0 || remaining > h.ttl {
		return apperrors.ErrNotFound
	}
	return h.store.Touch(ctx, session.ConnectionID, session.LastSeenAt, h.ttl)
}

// GetActiveSessions returns the current physical sessions used to authorize media commands.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//
// Returns:
//   - result 1: The collected session values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Hub) GetActiveSessions(ctx context.Context, conferenceID string) ([]domain.Session, error) {
	return h.store.Active(ctx, conferenceID)
}

// Broadcast publishes a trusted event only to the currently authorized conference or
// conversation recipients.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - event: Trusted event to publish or process.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Hub) Broadcast(ctx context.Context, event domain.Envelope) error {
	return h.store.Publish(ctx, domain.Bus{Kind: "event", ConferenceID: event.ConferenceID, Event: &event})
}

// SendToParticipant publishes a trusted event to the physical sessions of the selected
// authorized participant.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - participantID: Conference participant identifier.
//   - event: Trusted event to publish or process.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Hub) SendToParticipant(ctx context.Context, conferenceID, participantID string, event domain.Envelope) error {
	return h.store.Publish(ctx, domain.Bus{Kind: "event", ConferenceID: conferenceID, ParticipantID: participantID, Event: &event})
}

// SendToConnection publishes a trusted event to one authorized physical socket connection.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - session: Persistent or physical session state, as defined by its type.
//   - targetID: Identifier of the target account or resource.
//   - event: Trusted event to publish or process.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Hub) SendToConnection(ctx context.Context, session domain.Session, targetID string, event domain.Envelope) error {
	if _, err := h.AuthorizeClient(ctx, session.ConferenceID, session.UserID, session.ClientID); err != nil {
		return err
	}
	sender, err := h.store.Get(ctx, session.ConnectionID)
	if err != nil || sender.ConferenceID != session.ConferenceID || sender.UserID != session.UserID {
		return apperrors.ErrForbidden
	}
	target, err := h.store.Get(ctx, targetID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return apperrors.New(apperrors.ErrNotFound, "target connection is offline")
	}
	if err != nil {
		return err
	}
	if target.ConferenceID != session.ConferenceID {
		return apperrors.ErrForbidden
	}
	if _, err := h.AuthorizeClient(ctx, target.ConferenceID, target.UserID, target.ClientID); err != nil {
		return apperrors.ErrForbidden
	}
	err = h.store.Publish(ctx, domain.Bus{Kind: "event", ConferenceID: session.ConferenceID, ConnectionID: targetID, Event: &event})
	if err == nil {
		h.log(session, event.Type)
	}
	return err
}

// ConferenceChanged notifies connected sessions or observers of the conference's committed state
// change.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
func (h *Hub) ConferenceChanged(ctx context.Context, conferenceID string) {
	ctx, c := context.WithTimeout(ctx, 3*time.Second)
	defer c()
	if err := h.store.Publish(ctx, domain.Bus{Kind: "changed", ConferenceID: conferenceID}); err != nil {
		h.logger.Warn("realtime event", "conference_id", conferenceID, "event_type", "mutation_publish_failed")
	}
}

// state aggregates physical connections by persisted participant membership.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - session: Persistent or physical session state, as defined by its type.
//
// Returns:
//   - result 1: The resulting state.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Hub) state(ctx context.Context, session domain.Session) (domain.State, error) {
	status, roster, err := h.repo.Roster(ctx, session.ConferenceID)
	if err != nil {
		return domain.State{}, err
	}
	activeSessions, err := h.store.Active(ctx, session.ConferenceID)
	if err != nil {
		return domain.State{}, err
	}
	connectionsByParticipant := map[string][]string{}
	for _, activeSession := range activeSessions {
		connectionsByParticipant[activeSession.ParticipantID] = append(connectionsByParticipant[activeSession.ParticipantID], activeSession.ConnectionID)
	}
	state := domain.State{ConnectionID: session.ConnectionID, ParticipantID: session.ParticipantID, Status: status, Participants: make([]domain.Presence, 0, len(roster))}
	for _, participant := range roster {
		connectionIDs := connectionsByParticipant[participant.ID]
		if !participant.CanParticipate() || connectionIDs == nil {
			connectionIDs = []string{}
		}
		sort.Strings(connectionIDs)
		state.Participants = append(state.Participants, domain.Presence{ParticipantView: participant.View(), Online: len(connectionIDs) > 0, Connections: len(connectionIDs), ConnectionIDs: connectionIDs})
	}
	return state, nil
}

// stateFor hides the waiting roster from non-moderators and sets the recipient identity.
//
// Parameters:
//   - state: Current state or validated state update.
//   - session: Persistent or physical session state, as defined by its type.
//
// Returns:
//   - result 1: The resulting state.
func stateFor(state domain.State, session domain.Session) domain.State {
	state.ConnectionID, state.ParticipantID = session.ConnectionID, session.ParticipantID
	isModerator := false
	for _, participant := range state.Participants {
		if participant.ID == session.ParticipantID {
			isModerator = participant.Role == conferences.Owner || participant.Role == conferences.CoHost
			break
		}
	}
	if isModerator {
		return state
	}
	visible := make([]domain.Presence, 0, len(state.Participants))
	for _, participant := range state.Participants {
		if participant.CanReadHistory() {
			visible = append(visible, participant)
		}
	}
	state.Participants = visible
	return state
}

// stateAllows checks whether the current conference snapshot still permits the physical session
// to participate.
//
// Parameters:
//   - state: Current state or validated state update.
//   - participantID: Conference participant identifier.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func stateAllows(state domain.State, participantID string) bool {
	if state.Status != conferences.Created && state.Status != conferences.Active {
		return false
	}
	for _, p := range state.Participants {
		if p.ID == participantID {
			return p.CanParticipate() && !p.ChatLeft
		}
	}
	return false
}

// receive reads incoming physical session events and forwards them to the corresponding bounded
// delivery path.
func (h *Hub) receive() {
	defer h.workers.Done()
	sub, generation := h.sub, h.generation // Set before this worker starts.
	for {
		bus, err := sub.Receive(h.ctx)
		if err != nil {
			if h.ctx.Err() != nil {
				return
			}
			h.logger.Warn("realtime event", "event_type", "broker_unavailable")
			h.breakBroker(generation)
			_ = sub.Close()
			var ok bool
			sub, generation, ok = h.recoverSubscription()
			if !ok {
				return
			}
			continue
		}
		if bus.Kind == "event" && bus.Event != nil && domain.LowPriorityEvent(bus.Event.Type) {
			select {
			case h.lowEvents <- bus:
			default:
			} // Persisted history recovers events lost to queue overflow.
		} else {
			h.deliver(bus)
		}
	}
}

// breakBroker invalidates only the generation whose operation failed. A slow
// Prune result from an old connection must not close its healthy replacement.
// Network close and socket notifications run outside the registry lock.
//
// Parameters:
//   - generation: Content generation used to reject stale results.
func (h *Hub) breakBroker(generation uint64) {
	h.mu.Lock()
	if h.closing || h.unavailable || h.generation != generation {
		h.mu.Unlock()
		return
	}
	h.unavailable = true
	sub := h.sub
	entries := make([]*localSocket, 0, len(h.local))
	for _, entry := range h.local {
		entries = append(entries, entry)
	}
	h.mu.Unlock()
	for _, entry := range entries {
		entry.socket.Stop("broker_unavailable")
	}
	_ = sub.Close() // Interrupt Receive so its sole owner starts recovery.
}

// recoverSubscription runs in the existing receive worker. Each attempt owns
// its timeout and the next subscription is acknowledged before admission opens.
//
// Returns:
//   - result 1: The resulting subscription.
//   - result 2: The resulting count, size or numeric value.
//   - result 3: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (h *Hub) recoverSubscription() (domain.Subscription, uint64, bool) {
	backoff := 100 * time.Millisecond
	for {
		timer := time.NewTimer(backoff)
		select {
		case <-h.ctx.Done():
			timer.Stop()
			return nil, 0, false
		case <-timer.C:
		}
		timer.Stop()
		ctx, cancel := context.WithTimeout(h.ctx, 5*time.Second)
		sub, err := h.store.Subscribe(ctx)
		cancel()
		if err != nil {
			backoff = min(backoff*2, 2*time.Second)
			continue
		}
		h.mu.Lock()
		if h.closing || h.ctx.Err() != nil {
			h.mu.Unlock()
			_ = sub.Close()
			return nil, 0, false
		}
		h.sub = sub
		h.generation++
		generation := h.generation
		h.unavailable = false
		h.mu.Unlock()
		h.logger.Info("realtime event", "event_type", "broker_recovered")
		return sub, generation, true
	}
}

// receiveLowPriority delivers chat and reaction events through the independent bounded
// low-priority queue.
func (h *Hub) receiveLowPriority() {
	defer h.workers.Done()
	for {
		select {
		case <-h.ctx.Done():
			return
		case bus := <-h.lowEvents:
			h.deliver(bus)
		}
	}
}

// entries copies the local socket entries while holding the registry lock.
//
// Parameters:
//   - conferenceID: Conference identifier limiting the operation scope.
//
// Returns:
//   - result 1: The collected *local socket values.
func (h *Hub) entries(conferenceID string) []*localSocket {
	h.mu.Lock()
	defer h.mu.Unlock()
	rows := make([]*localSocket, 0, len(h.local))
	for _, entry := range h.local {
		if entry.ready && (conferenceID == "" || entry.session.ConferenceID == conferenceID) {
			rows = append(rows, entry)
		}
	}
	return rows
}

// deliver checks the current recipient authorization immediately before the saved event or
// command is delivered.
//
// Parameters:
//   - bus: Scoped publication and subscription transport.
func (h *Hub) deliver(bus domain.Bus) {
	ctx, c := context.WithTimeout(h.ctx, 5*time.Second)
	defer c()
	if bus.Kind == "expired" && bus.Session != nil {
		_ = h.repo.Close(ctx, bus.Session.ConnectionID, bus.Session.LastSeenAt)
		h.mu.Lock()
		observer := h.disconnectObserver
		h.mu.Unlock()
		if observer != nil {
			observer.Disconnected(ctx, *bus.Session)
		}
	}
	entries := h.entries(bus.ConferenceID)
	if len(entries) == 0 {
		return
	}
	if bus.Kind == "event" && bus.Event != nil {
		// Recheck recipients against current persisted membership, not the socket cache. Removal or
		// completion may commit before the state-change event is delivered.
		kind := bus.Event.Type
		restricted := strings.HasPrefix(kind, "caption.") || strings.HasPrefix(kind, "chat.") || strings.HasPrefix(kind, "recording.") || strings.HasPrefix(kind, "reaction.") || strings.HasPrefix(kind, "participant.")
		allowed := map[string]bool{}
		if restricted {
			status, roster, err := h.repo.Roster(ctx, bus.ConferenceID)
			if err != nil {
				return
			}
			for _, p := range roster {
				ok := p.CanParticipate() && !p.ChatLeft
				if status == conferences.Finished || status == conferences.Cancelled {
					ok = false
				}
				if kind == "participant.waiting" || kind == "participant.rejected" {
					ok = ok && p.CanAdmit()
				}
				allowed[p.ID] = ok
			}
		}
		for _, entry := range entries {
			s := entry.session
			if restricted && !allowed[s.ParticipantID] {
				continue
			}
			if (bus.ConnectionID == "" || bus.ConnectionID == s.ConnectionID) && (bus.ParticipantID == "" || bus.ParticipantID == s.ParticipantID) {
				if !entry.socket.Offer(*bus.Event) {
					entry.socket.Stop("slow_client")
				}
			}
		}
		return
	}
	if len(entries) == 0 {
		return
	}
	// Use one conference snapshot per event rather than a database and Redis lookup per socket.
	state, err := h.state(ctx, entries[0].session)
	if err != nil {
		for _, e := range entries {
			e.socket.Stop("state_unavailable")
		}
		return
	}
	joined := map[string]bool{}
	for _, p := range state.Participants {
		joined[p.ID] = p.CanParticipate() && !p.ChatLeft
	}
	for _, entry := range entries {
		if state.Status == conferences.Finished || state.Status == conferences.Cancelled || !joined[entry.session.ParticipantID] {
			entry.socket.Stop("membership_closed")
			continue
		}
		kind := "conference.state"
		if bus.Kind == "connected" {
			kind = "participant.connected"
		} else if bus.Kind == "disconnected" || bus.Kind == "expired" {
			kind = "participant.disconnected"
		}
		if !entry.socket.Offer(domain.Event(kind, bus.ConferenceID, stateFor(state, entry.session))) {
			entry.socket.Stop("slow_client")
		}
	}
}

// janitor periodically prunes expired presence and closes sockets whose authorization has been
// revoked.
func (h *Hub) janitor() {
	defer h.workers.Done()
	interval := h.ttl / 4
	if interval > 250*time.Millisecond {
		interval = 250 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	cursor := ""
	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			ctx, c := context.WithTimeout(h.ctx, 5*time.Second)
			h.mu.Lock()
			generation := h.generation
			h.mu.Unlock()
			if err := h.store.Prune(ctx); err != nil {
				h.breakBroker(generation)
			}
			// Also recover failures between opening the SQL session and registering its Redis presence.
			rows, err := h.repo.Stale(ctx, time.Now().Add(-2*h.ttl), cursor)
			if err == nil {
				h.closeMissingSessions(ctx, rows)
				if len(rows) < 500 {
					cursor = ""
				} else {
					cursor = rows[len(rows)-1].ID
				}
			}
			c()
		}
	}
}

// closeMissingSessions checks one bounded SQL page in a Redis request batch.
// Live routes need no JSON decoding; SQL is changed only after a successful
// presence check, preserving the failure behavior of the former GET loop.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - rows: Persisted or local state snapshot to reconcile.
func (h *Hub) closeMissingSessions(ctx context.Context, rows []domain.Session) {
	if len(rows) == 0 {
		return
	}
	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].ConnectionID
	}
	missing, err := h.store.Missing(ctx, ids)
	if err != nil || len(missing) == 0 {
		return
	}
	absent := make(map[string]struct{}, len(missing))
	for _, id := range missing {
		absent[id] = struct{}{}
	}
	for _, s := range rows {
		if _, ok := absent[s.ConnectionID]; ok {
			_ = h.repo.Close(ctx, s.ConnectionID, s.LastSeenAt)
		}
	}
}

// stopSockets closes the selected physical sockets after their access or conference state
// changes.
//
// Parameters:
//   - reason: Bounded lifecycle or failure reason.
func (h *Hub) stopSockets(reason string) {
	h.mu.Lock()
	h.closing = true
	entries := make([]*localSocket, 0, len(h.local))
	for _, e := range h.local {
		entries = append(entries, e)
	}
	h.mu.Unlock()
	for _, e := range entries {
		e.socket.Stop(reason)
	}
}

// Shutdown stops new work and waits for the component-owned resources to finish closing.
func (h *Hub) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = h.ShutdownContext(ctx)
}

// ShutdownContext stops accepting new sockets and waits for owned subscriptions and tasks until
// the shutdown context expires.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (h *Hub) ShutdownContext(ctx context.Context) error {
	h.shutdownOnce.Do(func() {
		h.shutdownDone = make(chan struct{})
		go func() {
			defer close(h.shutdownDone)
			h.stopSockets("server_shutdown")
			h.cancel()
			h.mu.Lock()
			sub := h.sub
			h.mu.Unlock()
			_ = sub.Close()
			h.sockets.Wait()
			h.workers.Wait()
		}()
	})
	select {
	case <-h.shutdownDone:
		return nil
	case <-ctx.Done():
		h.cancel()
		return ctx.Err()
	}
}

// LocalCount returns the number of locally registered sockets under the registry lock.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
func (h *Hub) LocalCount() int { h.mu.Lock(); defer h.mu.Unlock(); return len(h.local) }

// log records a bounded component diagnostic without private signaling content.
//
// Parameters:
//   - s: The session state processed by this operation.
//   - kind: Validated event, job, media or resource category.
func (h *Hub) log(s domain.Session, kind string) {
	h.logger.Info("realtime event", "conference_id", s.ConferenceID, "participant_id", s.ParticipantID, "user_id", s.UserID, "connection_id", s.ConnectionID, "event_type", kind)
}
