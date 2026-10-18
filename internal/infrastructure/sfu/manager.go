// Package sfu handles media transport. The media worker's internal control handlers validate
// application permissions, tickets and distributed ownership.
package sfu

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/pion/interceptor"
	pion "github.com/pion/webrtc/v4"
)

// Options contains the SFU's dependencies, media limits and lifecycle callbacks.
type Options struct {
	WorkerID                                                                           string
	ICE                                                                                []pion.ICEServer
	UDPPort, UDPMinPort, UDPMaxPort, TCPPort                                           int
	NATIPs                                                                             []string
	MaxPeers, MaxRooms, MaxPublishedTracks, MaxAudioTracks, MaxVideoTracks, QueueSize  int
	MaxScreenSharers, EgressQueueSize                                                  int
	ICEDisconnectedTimeout, ICEFailedTimeout, ICEKeepaliveInterval, NegotiationTimeout time.Duration
	Logger                                                                             *slog.Logger
	// Emit must be bounded. A connection-owned bounded event worker invokes it without SFU locks and
	// outside RTP forwarding. Lifecycle operations requested by Emit run asynchronously: synchronous
	// Leave would wait for the event worker currently calling Emit.
	Emit func(media.Binding, string, any)
}

// Stats contains connection, track and bounded-queue counters used for diagnostics.
type Stats struct {
	Rooms             int    `json:"roomsActive"`
	Peers             int    `json:"mediaPeersActive"`
	Tracks            int    `json:"tracksPublished"`
	Subscriptions     int    `json:"subscriptions"`
	PeerConnections   uint64 `json:"peerConnectionTotal"`
	Failures          uint64 `json:"peerConnectionFailures"`
	RelayConnections  uint64 `json:"relayConnections"`  // Cumulative count of selected candidate pairs using TURN.
	DirectConnections uint64 `json:"directConnections"` // Cumulative count of selected candidate pairs not using TURN.
	Packets           uint64 `json:"packetsForwarded"`
	Bytes             uint64 `json:"bytesForwarded"`
	Dropped           uint64 `json:"packetsDropped"`
	RecordingOutputs  int    `json:"recordingOutputs"`
	RecordingDrops    uint64 `json:"recordingDrops"`
	AudioTaps         int    `json:"audioTaps"`
	AudioTapDrops     uint64 `json:"audioTapDrops"`
}

// Manager owns local media resources and synchronizes their creation, use and teardown.
type Manager struct {
	opts                                     Options
	api                                      *pion.API
	mu                                       sync.Mutex
	rooms                                    map[string]*room
	peers                                    map[string]*peer
	connections                              map[string]*peer
	roomClosures                             map[string]chan struct{}
	closing                                  bool
	closed                                   chan struct{}
	shutdownOnce                             sync.Once
	closeWG                                  sync.WaitGroup
	udp                                      net.PacketConn
	tcp                                      net.Listener
	total, failures, packets, bytes, dropped atomic.Uint64
	egressDropped                            atomic.Uint64
	audioTapDropped                          atomic.Uint64
	relay, direct                            atomic.Uint64
}

// room groups one local SFU room's peers, publications and lifetime state.
type room struct {
	id       string
	mu       sync.Mutex
	peers    map[string]*peer
	tracks   map[string]*publishedTrack
	policies map[string]media.ParticipantPolicy
	screens  map[string]bool
	egresses map[string]*egress
}

// defaults fills missing component settings with their supported bounded defaults.
//
// Parameters:
//   - o: Component dependencies and construction options.
//
// Returns:
//   - result 1: The resulting options.
func defaults(o Options) Options {
	if o.MaxPeers == 0 {
		o.MaxPeers = 10
	}
	if o.MaxRooms == 0 {
		o.MaxRooms = 100
	}
	if o.MaxPublishedTracks == 0 {
		o.MaxPublishedTracks = 4
	}
	if o.MaxAudioTracks == 0 {
		o.MaxAudioTracks = 2
	}
	if o.MaxVideoTracks == 0 {
		o.MaxVideoTracks = 2
	}
	if o.MaxScreenSharers == 0 {
		o.MaxScreenSharers = 1
	}
	if o.EgressQueueSize == 0 {
		o.EgressQueueSize = 2048
	}
	if o.QueueSize == 0 {
		o.QueueSize = 128
	}
	if o.ICEDisconnectedTimeout == 0 {
		o.ICEDisconnectedTimeout = 10 * time.Second
	}
	if o.ICEFailedTimeout == 0 {
		o.ICEFailedTimeout = 20 * time.Second
	}
	if o.ICEKeepaliveInterval == 0 {
		o.ICEKeepaliveInterval = 2 * time.Second
	}
	if o.NegotiationTimeout == 0 {
		o.NegotiationTimeout = 15 * time.Second
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return o
}

// NewManager constructs the manager with the dependencies and limits required for SFU peers and
// media forwarding.
//
// Parameters:
//   - options: Validated component construction or execution options.
//
// Returns:
//   - result 1: The constructed or loaded manager.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NewManager(options Options) (*Manager, error) {
	o := defaults(options)
	if o.MaxPeers < 2 || o.MaxPeers > 100 || o.MaxRooms < 1 || o.MaxRooms > 10000 ||
		o.MaxPublishedTracks < 1 || o.MaxPublishedTracks > 4 || o.MaxAudioTracks < 1 || o.MaxAudioTracks > 2 || o.MaxVideoTracks < 1 || o.MaxVideoTracks > 2 ||
		o.MaxScreenSharers < 1 || o.MaxScreenSharers > 4 || o.EgressQueueSize < 1 || o.EgressQueueSize > 8192 ||
		o.QueueSize < 1 || o.QueueSize > 4096 || o.NegotiationTimeout < 100*time.Millisecond ||
		o.UDPPort < 0 || o.UDPPort > 65535 || o.TCPPort < 0 || o.TCPPort > 65535 ||
		o.UDPMinPort < 0 || o.UDPMaxPort > 65535 || o.UDPMinPort > o.UDPMaxPort ||
		(o.UDPMinPort == 0) != (o.UDPMaxPort == 0) || (o.UDPPort > 0 && o.UDPMinPort > 0) ||
		o.ICEDisconnectedTimeout <= 0 || o.ICEFailedTimeout <= 0 || o.ICEKeepaliveInterval <= 0 {
		return nil, media.ErrInvalid
	}
	engine := &pion.MediaEngine{}
	if err := engine.RegisterCodec(pion.RTPCodecParameters{RTPCodecCapability: pion.RTPCodecCapability{
		MimeType: pion.MimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: "minptime=10;useinbandfec=1",
	}, PayloadType: 111}, pion.RTPCodecTypeAudio); err != nil {
		return nil, err
	}
	feedback := []pion.RTCPFeedback{{Type: "nack"}, {Type: "nack", Parameter: "pli"}, {Type: "ccm", Parameter: "fir"}}
	if err := engine.RegisterCodec(pion.RTPCodecParameters{RTPCodecCapability: pion.RTPCodecCapability{
		MimeType: pion.MimeTypeVP8, ClockRate: 90000, RTCPFeedback: feedback,
	}, PayloadType: 96}, pion.RTPCodecTypeVideo); err != nil {
		return nil, err
	}
	registry := &interceptor.Registry{}
	loggerFactory := safePionLoggerFactory{logger: o.Logger}
	if err := pion.RegisterDefaultInterceptorsWithOptions(engine, registry, pion.WithInterceptorLoggerFactory(loggerFactory)); err != nil {
		return nil, err
	}
	settings := pion.SettingEngine{LoggerFactory: loggerFactory}
	settings.SetICETimeouts(o.ICEDisconnectedTimeout, o.ICEFailedTimeout, o.ICEKeepaliveInterval)
	m := &Manager{opts: o, rooms: map[string]*room{}, peers: map[string]*peer{}, connections: map[string]*peer{}, roomClosures: map[string]chan struct{}{}, closed: make(chan struct{})}
	cleanup := func() {
		if m.udp != nil {
			_ = m.udp.Close()
		}
		if m.tcp != nil {
			_ = m.tcp.Close()
		}
	}
	networks := []pion.NetworkType{pion.NetworkTypeUDP4}
	if o.UDPPort > 0 {
		conn, err := net.ListenPacket("udp4", net.JoinHostPort("0.0.0.0", strconv.Itoa(o.UDPPort)))
		if err != nil {
			return nil, err
		}
		m.udp = conn
		settings.SetICEUDPMux(pion.NewICEUDPMux(loggerFactory.NewLogger("ice"), conn))
	} else if o.UDPMinPort > 0 {
		if err := settings.SetEphemeralUDPPortRange(uint16(o.UDPMinPort), uint16(o.UDPMaxPort)); err != nil {
			return nil, err
		}
	}
	if o.TCPPort > 0 {
		listener, err := net.Listen("tcp4", net.JoinHostPort("0.0.0.0", strconv.Itoa(o.TCPPort)))
		if err != nil {
			cleanup()
			return nil, err
		}
		m.tcp = listener
		settings.SetICETCPMux(pion.NewICETCPMux(loggerFactory.NewLogger("ice"), listener, 8))
		networks = append(networks, pion.NetworkTypeTCP4)
	}
	settings.SetNetworkTypes(networks)
	if len(o.NATIPs) > 0 {
		for _, ip := range o.NATIPs {
			if net.ParseIP(ip) == nil {
				cleanup()
				return nil, media.ErrInvalid
			}
		}
		// Replace preserves NAT 1:1 mapping for the SFU's configured IPv4 host candidates.
		if err := settings.SetICEAddressRewriteRules(pion.ICEAddressRewriteRule{
			External:        o.NATIPs,
			AsCandidateType: pion.ICECandidateTypeHost,
			Mode:            pion.ICEAddressRewriteReplace,
		}); err != nil {
			cleanup()
			return nil, fmt.Errorf("configure SFU ICE address rewriting: %w", err)
		}
	}
	m.api = pion.NewAPI(pion.WithMediaEngine(engine), pion.WithInterceptorRegistry(registry), pion.WithSettingEngine(settings))
	// The sentinel protects WaitGroup additions until Shutdown detaches every registered connection.
	// Add a connection to closeWG before removing it from the map.
	m.closeWG.Add(1)
	return m, nil
}

// Join creates or resumes the authorized membership or media-room connection, preserving its
// current identity.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - binding: Verified account, session and media-connection identity.
//
// Returns:
//   - result 1: The resulting peer view.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) Join(ctx context.Context, binding media.Binding) (media.PeerView, error) {
	for _, id := range []string{binding.ConferenceID, binding.ParticipantID, binding.SessionID, binding.ConnectionID} {
		if parsed, err := uuid.Parse(id); err != nil || parsed == uuid.Nil {
			return media.PeerView{}, media.ErrInvalid
		}
	}
	if err := ctx.Err(); err != nil {
		return media.PeerView{}, err
	}
	m.mu.Lock()
	if m.closing || m.roomClosures[binding.ConferenceID] != nil {
		m.mu.Unlock()
		return media.PeerView{}, media.ErrUnavailable
	}
	if old := m.connections[binding.ConnectionID]; old != nil {
		m.mu.Unlock()
		if sameBinding(old.binding, binding) {
			return old.view(), nil
		}
		return media.PeerView{}, media.ErrUnauthorized
	}
	// Create the connection outside the shared lock, but make Shutdown wait until an in-flight
	// NewPeerConnection has been registered or closed.
	m.closeWG.Add(1)
	m.mu.Unlock()
	defer m.closeWG.Done()
	pc, err := m.api.NewPeerConnection(pion.Configuration{ICEServers: m.opts.ICE})
	if err != nil {
		return media.PeerView{}, media.ErrUnavailable
	}
	ctxPeer, cancel := context.WithCancel(context.Background())
	p := &peer{id: uuid.NewString(), binding: binding, manager: m, pc: pc, ctx: ctxPeer, cancel: cancel,
		publications: map[string]*publishedTrack{}, subscriptions: map[string]*subscription{}, receivers: map[string]*receiver{},
		events: make(chan outboundEvent, 128), notify: make(chan struct{}, 1)}
	registered := false
	defer func() {
		if !registered {
			// Before registration Join owns the peer; all rejection paths release
			// it through the same lifecycle owner, outside manager/room locks.
			p.close()
		}
	}()
	m.mu.Lock()
	if old := m.connections[binding.ConnectionID]; old != nil {
		m.mu.Unlock()
		if sameBinding(old.binding, binding) {
			return old.view(), nil
		}
		return media.PeerView{}, media.ErrUnauthorized
	}
	if m.closing || ctx.Err() != nil || m.roomClosures[binding.ConferenceID] != nil {
		m.mu.Unlock()
		return media.PeerView{}, media.ErrUnavailable
	}
	r := m.rooms[binding.ConferenceID]
	if r == nil {
		if len(m.rooms) >= m.opts.MaxRooms {
			m.mu.Unlock()
			return media.PeerView{}, media.ErrLimit
		}
		r = newRoom(binding.ConferenceID)
		m.rooms[r.id] = r
	}
	r.mu.Lock()
	if r.policies[binding.ParticipantID].Kicked {
		r.mu.Unlock()
		m.mu.Unlock()
		return media.PeerView{}, media.ErrPolicy
	}
	if len(r.peers) >= m.opts.MaxPeers {
		r.mu.Unlock()
		m.mu.Unlock()
		return media.PeerView{}, media.ErrLimit
	}
	p.room = r
	r.peers[p.id] = p
	r.mu.Unlock()
	m.peers[p.id] = p
	m.connections[binding.ConnectionID] = p
	m.total.Add(1)
	registered = true // The manager now owns Leave/Shutdown and peer.close.
	// Install callbacks and start background tasks only after registration is protected against
	// Shutdown.
	p.install()
	m.mu.Unlock()
	m.syncPeer(p)
	m.log(p, "media_join", nil)
	return p.view(), nil
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
	return a.ConferenceID == b.ConferenceID && a.ParticipantID == b.ParticipantID && a.SessionID == b.SessionID && a.ConnectionID == b.ConnectionID && a.UserID == b.UserID
}

// get looks up the current component-owned media connection under the manager state lock.
//
// Parameters:
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The constructed or loaded peer.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) get(id string) (*peer, error) {
	m.mu.Lock()
	p := m.peers[id]
	m.mu.Unlock()
	if p == nil {
		return nil, media.ErrPeerNotFound
	}
	return p, nil
}

// PeerBinding returns the server-side session identity bound to a peer connection.
//
// Parameters:
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting binding.
//   - result 2: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (m *Manager) PeerBinding(id string) (media.Binding, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.peers[id]
	if p == nil {
		return media.Binding{}, false
	}
	return p.binding, true
}

// Bindings returns the current peer identity bindings for the requested media room.
//
// Returns:
//   - result 1: The collected binding values.
func (m *Manager) Bindings() []media.Binding {
	m.mu.Lock()
	defer m.mu.Unlock()
	bindings := make([]media.Binding, 0, len(m.peers))
	for _, p := range m.peers {
		bindings = append(bindings, p.binding)
	}
	return bindings
}

// Leave removes the active membership or media connection while preserving the saved
// conversation or conference history.
//
// Parameters:
//   - _: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) Leave(_ context.Context, id string) error {
	m.mu.Lock()
	p := m.peers[id]
	if p == nil {
		m.mu.Unlock()
		return nil
	}
	m.detachLocked(p)
	m.mu.Unlock()
	m.finishDetached(p)
	return nil
}

// detachLocked removes the peer from active manager state before performing network cleanup
// outside the lock.
//
// Parameters:
//   - p: The peer state processed by this operation.
func (m *Manager) detachLocked(p *peer) {
	m.closeWG.Add(1)
	p.mu.Lock()
	p.closing = true
	p.cancel()
	delete(m.peers, p.id)
	delete(m.connections, p.binding.ConnectionID)
	p.room.mu.Lock()
	delete(p.room.peers, p.id)
	delete(p.room.screens, p.id)
	empty := len(p.room.peers) == 0 && len(p.room.egresses) == 0
	p.room.mu.Unlock()
	p.mu.Unlock()
	if empty {
		delete(m.rooms, p.room.id)
	}
}

// finishDetached closes a detached peer's media resources outside the manager lock.
//
// Parameters:
//   - p: The peer state processed by this operation.
func (m *Manager) finishDetached(p *peer) {
	defer m.closeWG.Done()
	p.close()
	m.log(p, "media_leave", nil)
}

// LeaveConnection disconnects only the specified physical media connection.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - connectionID: Physical media-connection identifier.
func (m *Manager) LeaveConnection(ctx context.Context, connectionID string) {
	m.mu.Lock()
	p := m.connections[connectionID]
	m.mu.Unlock()
	if p != nil {
		_ = m.Leave(ctx, p.id)
	}
}

// CloseConference closes the media peers and owned resources of one conference.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) CloseConference(ctx context.Context, id string) error {
	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return waitClosed(ctx, m.closed)
	}
	if done := m.roomClosures[id]; done != nil {
		m.mu.Unlock()
		return waitClosed(ctx, done)
	}
	peers := []*peer{}
	if r := m.rooms[id]; r != nil {
		r.mu.Lock()
		for _, e := range r.egresses {
			e.fail(media.ErrUnavailable)
		}
		r.egresses = map[string]*egress{}
		if len(r.peers) == 0 {
			delete(m.rooms, id)
		}
		r.mu.Unlock()
	}
	for _, p := range m.peers {
		if p.binding.ConferenceID == id {
			peers = append(peers, p)
		}
	}
	if len(peers) == 0 {
		m.mu.Unlock()
		return nil
	}
	done := make(chan struct{})
	m.roomClosures[id] = done
	// Account for the coordinator and connection cleanup. Shutdown waits for every conference close
	// started before the global shutdown flag was set.
	m.closeWG.Add(1)
	for _, p := range peers {
		m.detachLocked(p)
	}
	m.mu.Unlock()
	go func() {
		defer m.closeWG.Done()
		var wg sync.WaitGroup
		for _, p := range peers {
			wg.Add(1)
			go func(p *peer) { defer wg.Done(); m.finishDetached(p) }(p)
		}
		wg.Wait()
		m.mu.Lock()
		delete(m.roomClosures, id)
		m.mu.Unlock()
		close(done)
	}()
	return waitClosed(ctx, done)
}

// Shutdown stops new work and waits for the component-owned resources to finish closing.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.shutdownOnce.Do(func() {
		m.mu.Lock()
		m.closing = true
		for _, r := range m.rooms {
			r.mu.Lock()
			for _, e := range r.egresses {
				e.fail(media.ErrUnavailable)
			}
			r.egresses = map[string]*egress{}
			if len(r.peers) == 0 {
				delete(m.rooms, r.id)
			}
			r.mu.Unlock()
		}
		peers := make([]*peer, 0, len(m.peers))
		for _, p := range m.peers {
			peers = append(peers, p)
		}
		for _, p := range peers {
			m.detachLocked(p)
		}
		m.mu.Unlock()
		go func() {
			for _, p := range peers {
				go m.finishDetached(p)
			}
			m.closeWG.Done()
			m.closeWG.Wait()
			if m.udp != nil {
				_ = m.udp.Close()
			}
			if m.tcp != nil {
				_ = m.tcp.Close()
			}
			close(m.closed)
		}()
	})
	return waitClosed(ctx, m.closed)
}

// waitClosed waits for the manager's detached peer resources to finish closing.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - done: Channel closed when the owned background resource terminates.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func waitClosed(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Snapshot returns a consistent snapshot of the manager's current media state and counters.
//
// Returns:
//   - result 1: The resulting stats.
func (m *Manager) Snapshot() Stats {
	m.mu.Lock()
	rooms := make([]*room, 0, len(m.rooms))
	for _, r := range m.rooms {
		rooms = append(rooms, r)
	}
	s := Stats{Rooms: len(m.rooms), Peers: len(m.peers)}
	m.mu.Unlock()
	for _, r := range rooms {
		r.mu.Lock()
		for _, e := range r.egresses {
			if e.audioOnly {
				s.AudioTaps++
			} else {
				s.RecordingOutputs++
			}
		}
		sources := make([]*publishedTrack, 0, len(r.tracks))
		for _, t := range r.tracks {
			sources = append(sources, t)
		}
		r.mu.Unlock()
		s.Tracks += len(sources)
		for _, t := range sources {
			t.mu.Lock()
			s.Subscriptions += len(t.subscribers)
			t.mu.Unlock()
		}
	}
	s.PeerConnections = m.total.Load()
	s.Failures = m.failures.Load()
	s.RelayConnections = m.relay.Load()
	s.DirectConnections = m.direct.Load()
	s.Packets = m.packets.Load()
	s.Bytes = m.bytes.Load()
	s.Dropped = m.dropped.Load()
	s.RecordingDrops = m.egressDropped.Load()
	s.AudioTapDrops = m.audioTapDropped.Load()
	return s
}

// newRoom creates the room-local track and peer registries.
//
// Parameters:
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The constructed or loaded room.
func newRoom(id string) *room {
	return &room{id: id, peers: map[string]*peer{}, tracks: map[string]*publishedTrack{}, policies: map[string]media.ParticipantPolicy{}, screens: map[string]bool{}, egresses: map[string]*egress{}}
}

// HasConference reports whether this manager currently owns the requested conference room.
//
// Parameters:
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (m *Manager) HasConference(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rooms[id] != nil
}

// log records a bounded component diagnostic without private signaling content.
//
// Parameters:
//   - p: The peer state processed by this operation.
//   - event: Trusted event to publish or process.
//   - fields: Bounded structured diagnostic fields.
func (m *Manager) log(p *peer, event string, fields []any) {
	args := []any{"worker_id", m.opts.WorkerID, "conference_id", p.binding.ConferenceID, "participant_id", p.binding.ParticipantID, "session_id", p.binding.SessionID, "connection_id", p.binding.ConnectionID, "media_peer_id", p.id, "event_type", event}
	args = append(args, fields...)
	m.opts.Logger.Info("media event", args...)
}

// supported checks whether the supplied media codec is supported by the SFU.
//
// Parameters:
//   - track: Media-track metadata or negotiated track instance.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func supported(track *pion.TrackRemote) bool {
	return (track.Kind() == pion.RTPCodecTypeAudio && strings.EqualFold(track.Codec().MimeType, pion.MimeTypeOpus)) ||
		(track.Kind() == pion.RTPCodecTypeVideo && strings.EqualFold(track.Codec().MimeType, pion.MimeTypeVP8))
}
