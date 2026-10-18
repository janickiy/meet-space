package sfu

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/pion/ice/v4"
	"github.com/pion/sdp/v3"
	pion "github.com/pion/webrtc/v4"
)

// outboundEvent carries a peer event through the bounded outgoing signaling queue.
type outboundEvent struct {
	kind string
	data any
}

// peer owns a physical WebRTC connection, signaling state, participant policy and subscriptions.
type peer struct {
	id      string
	binding media.Binding
	manager *Manager
	room    *room
	pc      *pion.PeerConnection
	ctx     context.Context
	cancel  context.CancelFunc
	// negotiation serializes AddTrack, RemoveTrack and SDP negotiation.
	negotiation sync.Mutex
	// offers protects publication cleanup without holding negotiation while touching other peers'
	// senders, preventing cross-peer deadlocks.
	offers               sync.Mutex
	mu                   sync.Mutex
	closing              bool
	publications         map[string]*publishedTrack
	subscriptions        map[string]*subscription
	events               chan outboundEvent
	notify               chan struct{}
	revision             atomic.Uint64
	wg                   sync.WaitGroup
	closeOnce            sync.Once
	stopRequested        atomic.Bool
	failed               atomic.Bool // Prevents the callback and watchdog from counting the same failure twice.
	classified           atomic.Bool // Count the ICE route once per physical connection.
	allowedSources       map[string]media.Source
	receivers            map[string]*receiver
	suspendedSources     map[string]string
	offeredTrackIDs      map[string]string
	pendingRemoteICE     []pion.ICECandidateInit // Protected by negotiation.
	remoteICECount       int                     // Protected by negotiation; bounded per PeerConnection.
	ready                atomic.Bool
	forwarding           sync.RWMutex    // Protects readiness changes against concurrent RTP writes.
	pendingSubscriptions []*subscription // Exact negotiated answer set; protected by negotiation.
	negotiationID        string          // Protected by mu.
	pendingReadyAt       time.Time       // Protected by mu.
}

// view builds the public, member-specific view of the stored resource.
//
// Returns:
//   - result 1: The resulting peer view.
func (p *peer) view() media.PeerView {
	return media.PeerView{MediaPeerID: p.id, ConferenceID: p.binding.ConferenceID, ParticipantID: p.binding.ParticipantID, SessionID: p.binding.SessionID, ConnectionID: p.binding.ConnectionID}
}

// start starts the peer-owned background processing and prepares its transport callbacks.
//
// Parameters:
//   - fn: Callback executed within the component-owned lifecycle.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (p *peer) start(fn func()) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closing {
		return false
	}
	p.wg.Add(1)
	go func() { defer p.wg.Done(); fn() }()
	return true
}

// install installs Pion callbacks for signaling, transport changes and incoming tracks.
func (p *peer) install() {
	p.start(p.emitLoop)
	p.start(p.negotiationNotifications)
	p.pc.OnTrack(func(t *pion.TrackRemote, r *pion.RTPReceiver) {
		p.start(func() { p.manager.receive(p, t, r) })
	})
	p.pc.OnICECandidate(func(candidate *pion.ICECandidate) {
		var value *pion.ICECandidateInit
		if candidate != nil {
			c := candidate.ToJSON()
			value = &c
		}
		p.emit("media.ice", map[string]any{"mediaPeerId": p.id, "candidate": value})
	})
	p.pc.OnConnectionStateChange(func(state pion.PeerConnectionState) {
		p.manager.log(p, "peer_connection_state", []any{"state", state.String()})
		p.emit("media.state", map[string]string{"mediaPeerId": p.id, "state": state.String()})
		if state == pion.PeerConnectionStateConnected {
			p.classifyTransport()
		}
		if state == pion.PeerConnectionStateFailed {
			p.markFailure()
			p.stopAsync()
		}
	})
	p.pc.OnICEConnectionStateChange(func(state pion.ICEConnectionState) {
		p.manager.log(p, "ice_state", []any{"state", state.String()})
	})
	p.start(func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		created := time.Now()
		var disconnectedAt time.Time
		for {
			select {
			case <-p.ctx.Done():
				return
			case <-ticker.C:
				p.mu.Lock()
				pendingReadyAt := p.pendingReadyAt
				p.mu.Unlock()
				if !pendingReadyAt.IsZero() && time.Since(pendingReadyAt) >= p.manager.opts.NegotiationTimeout {
					p.markFailure()
					p.stopAsync()
					return
				}
				state := p.pc.ConnectionState()
				if state == pion.PeerConnectionStateConnected {
					disconnectedAt = time.Time{}
					continue
				}
				if state == pion.PeerConnectionStateDisconnected {
					if disconnectedAt.IsZero() {
						disconnectedAt = time.Now()
					}
					if time.Since(disconnectedAt) < p.manager.opts.ICEDisconnectedTimeout {
						continue
					}
				} else if state != pion.PeerConnectionStateFailed && time.Since(created) < p.manager.opts.NegotiationTimeout {
					continue
				}
				if state != pion.PeerConnectionStateClosed {
					p.markFailure()
				}
				p.stopAsync()
				return
			}
		}
	})
}

// markFailure records a terminal peer failure and starts the peer cleanup path.
func (p *peer) markFailure() {
	if !p.stopRequested.Load() && p.failed.CompareAndSwap(false, true) {
		p.manager.failures.Add(1)
	}
}

// classifyTransport maps a transport error to the bounded media error classification.
func (p *peer) classifyTransport() {
	for _, receiver := range p.pc.GetReceivers() {
		transport := receiver.Transport()
		if transport == nil || transport.ICETransport() == nil {
			continue
		}
		pair, err := transport.ICETransport().GetSelectedCandidatePair()
		if err != nil || pair == nil || pair.Local == nil || pair.Remote == nil {
			continue
		}
		if p.classified.CompareAndSwap(false, true) {
			if pair.Local.Typ == pion.ICECandidateTypeRelay || pair.Remote.Typ == pion.ICECandidateTypeRelay {
				p.manager.relay.Add(1)
			} else {
				p.manager.direct.Add(1)
			}
		}
		return
	}
}

// emit sends a trusted lifecycle event through the component-owned bounded delivery path.
//
// Parameters:
//   - kind: Validated event, job, media or resource category.
//   - data: Input bytes or structured event payload.
func (p *peer) emit(kind string, data any) {
	select {
	case <-p.ctx.Done():
		return
	default:
	}
	select {
	case p.events <- outboundEvent{kind, data}:
	default:
		p.stopAsync()
	}
}

// stopAsync starts peer teardown without blocking a transport callback.
func (p *peer) stopAsync() {
	if p.stopRequested.CompareAndSwap(false, true) {
		go func() { _ = p.manager.Leave(context.Background(), p.id) }()
	}
}

// emitLoop delivers queued peer events without blocking the media callback that produced them.
func (p *peer) emitLoop() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case event := <-p.events:
			if p.manager.opts.Emit != nil {
				p.manager.opts.Emit(p.binding, event.kind, event.data)
			}
		}
	}
}

// changed notifies the component observer after local or persisted state changes.
func (p *peer) changed() {
	p.revision.Add(1)
	select {
	case p.notify <- struct{}{}:
	default:
	}
}

// tracks builds a snapshot of the media tracks visible to the current peer.
//
// Returns:
//   - result 1: The collected track values.
func (p *peer) tracks() []media.Track {
	p.mu.Lock()
	defer p.mu.Unlock()
	tracks := make([]media.Track, 0, len(p.subscriptions))
	for _, sub := range p.subscriptions {
		tracks = append(tracks, sub.source.metadata)
	}
	return tracks
}

// negotiationNotifications coalesces track changes into bounded peer-negotiation notifications.
func (p *peer) negotiationNotifications() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.notify:
			// Notification coalescing is only an optimization. Revision increases monotonically; every
			// change during a pending offer leaves a new notification pending.
			timer := time.NewTimer(30 * time.Millisecond)
			select {
			case <-p.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			select {
			case <-p.notify:
			default:
			}
			revision := p.revision.Load()
			p.emit("media.tracks", map[string]any{"mediaPeerId": p.id, "revision": revision, "tracks": p.tracks()})
			p.emit("media.renegotiate", map[string]any{"mediaPeerId": p.id, "revision": revision})
		}
	}
}

// close closes the component-owned resources and ends their lifecycle.
func (p *peer) close() {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closing = true
		pubs := make([]*publishedTrack, 0, len(p.publications))
		for _, t := range p.publications {
			pubs = append(pubs, t)
		}
		subs := make([]*subscription, 0, len(p.subscriptions))
		for _, s := range p.subscriptions {
			subs = append(subs, s)
		}
		p.mu.Unlock()
		p.cancel()
		// Close the transport first to unblock RTP and RTCP reads. Never hold peer or room locks while
		// closing Pion network transports.
		_ = p.pc.Close()
		for _, t := range pubs {
			p.manager.unpublish(t)
		}
		for _, s := range subs {
			p.manager.unsubscribe(s)
		}
		p.wg.Wait()
	})
}

// Offer processes an SDP offer for the currently bound WebRTC connection.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//   - negotiationID: Identifier correlating this offer with its negotiation cycle.
//   - raw: Untrusted serialized value to decode or validate.
//
// Returns:
//   - result 1: The resulting session description.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) Offer(ctx context.Context, id, negotiationID, raw string) (pion.SessionDescription, error) {
	return m.OfferSources(ctx, id, negotiationID, raw, nil)
}

// OfferSources processes an SDP offer with its explicit media-source declarations.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//   - negotiationID: Identifier correlating this offer with its negotiation cycle.
//   - raw: Untrusted serialized value to decode or validate.
//   - publications: Validated declarations of the peer's media sources.
//
// Returns:
//   - result 1: The resulting session description.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) OfferSources(ctx context.Context, id, negotiationID, raw string, publications []media.Publication) (pion.SessionDescription, error) {
	if parsed, err := uuid.Parse(negotiationID); err != nil || parsed == uuid.Nil {
		return pion.SessionDescription{}, media.ErrInvalid
	}
	p, err := m.get(id)
	if err != nil {
		return pion.SessionDescription{}, err
	}
	sources, err := validateSourceOffer(raw, m.opts.MaxPeers, publications, m.opts.MaxPublishedTracks, m.opts.MaxAudioTracks, m.opts.MaxVideoTracks)
	if err != nil {
		return pion.SessionDescription{}, err
	}
	receiveVideo, err := receiveVideoPreference(raw)
	if err != nil {
		return pion.SessionDescription{}, err
	}
	if err = ctx.Err(); err != nil {
		return pion.SessionDescription{}, err
	}
	p.offers.Lock()
	defer p.offers.Unlock()
	p.negotiation.Lock()
	p.mu.Lock()
	closed := p.closing
	trackIDs := map[string]string{}
	for _, publication := range publications {
		trackIDs[publication.MID] = publication.TrackID
	}
	for mid, previousID := range p.suspendedSources {
		if sources[mid] == "" || (trackIDs[mid] != "" && trackIDs[mid] != previousID) {
			delete(p.suspendedSources, mid)
		} else {
			delete(sources, mid)
		}
	}
	p.mu.Unlock()
	if closed {
		p.negotiation.Unlock()
		return pion.SessionDescription{}, media.ErrPeerNotFound
	}
	if p.pc.SignalingState() != pion.SignalingStateStable {
		p.negotiation.Unlock()
		return pion.SessionDescription{}, media.ErrNegotiation
	}
	if err = p.reserveSources(sources); err != nil {
		p.negotiation.Unlock()
		return pion.SessionDescription{}, err
	}
	p.forwarding.Lock()
	p.mu.Lock()
	// Sending an answer does not prove the receiver applied it. Block new SSRCs until the matching
	// Ready sent after SetRemoteDescription.
	p.ready.Store(false)
	p.pendingSubscriptions = nil
	for _, sub := range p.subscriptions {
		sub.ready.Store(false)
	}
	p.negotiationID = negotiationID
	p.pendingReadyAt = time.Now()
	p.allowedSources = sources
	p.offeredTrackIDs = trackIDs
	p.mu.Unlock()
	p.forwarding.Unlock()
	if err = p.pc.SetRemoteDescription(pion.SessionDescription{Type: pion.SDPTypeOffer, SDP: raw}); err != nil {
		p.negotiation.Unlock()
		// Failed SetRemoteDescription may partially change Pion signaling or ICE state. Do not retain a
		// transport whose negotiation state is uncertain.
		p.stopAsync()
		return pion.SessionDescription{}, media.ErrInvalid
	}
	pending := p.pendingRemoteICE
	p.pendingRemoteICE = nil
	for _, candidate := range pending {
		if err = p.pc.AddICECandidate(candidate); err != nil {
			p.negotiation.Unlock()
			p.stopAsync()
			return pion.SessionDescription{}, media.ErrInvalid
		}
	}
	answer, err := p.pc.CreateAnswer(nil)
	if err == nil {
		err = p.pc.SetLocalDescription(answer)
	}
	if err != nil {
		p.negotiation.Unlock()
		p.stopAsync()
		return pion.SessionDescription{}, media.ErrNegotiation
	}
	local := p.pc.LocalDescription()
	if local == nil {
		p.negotiation.Unlock()
		p.stopAsync()
		return pion.SessionDescription{}, media.ErrNegotiation
	}
	result := *local
	declared, parseErr := answeredSSRCs(result.SDP)
	if parseErr != nil {
		p.negotiation.Unlock()
		p.stopAsync()
		return pion.SessionDescription{}, media.ErrNegotiation
	}
	p.mu.Lock()
	for _, sub := range p.subscriptions {
		// Sender parameters allocate SSRCs before binding and do not prove negotiation. Only the primary
		// SSRC explicitly declared in an active sending section of the applied answer passes the
		// answer-applied gate.
		parameters := sub.sender.GetParameters()
		if len(parameters.Encodings) > 0 && declared[uint32(parameters.Encodings[0].SSRC)] && (receiveVideo || sub.source.metadata.Kind != "video") {
			p.pendingSubscriptions = append(p.pendingSubscriptions, sub)
		}
	}
	p.mu.Unlock()
	p.negotiation.Unlock()
	// Never reconcile while holding publisher.negotiation: publication removal also locks
	// subscribers. Parallel offers must not create A-to-B and B-to-A lock cycles.
	p.mu.Lock()
	retire := []*publishedTrack{}
	for _, t := range p.publications {
		if p.allowedSources[t.receiver.mid] != t.metadata.Source {
			retire = append(retire, t)
		}
	}
	p.mu.Unlock()
	for _, t := range retire {
		m.unpublish(t)
	}
	return result, nil
}

// Ready accepts confirmation that the client applied the SDP answer and enables tracks only for
// the matching current negotiation.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//   - negotiationID: Identifier correlating this offer with its negotiation cycle.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) Ready(ctx context.Context, id, negotiationID string) error {
	p, err := m.get(id)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	p.negotiation.Lock()
	p.forwarding.Lock()
	p.mu.Lock()
	if p.closing {
		p.mu.Unlock()
		p.forwarding.Unlock()
		p.negotiation.Unlock()
		return media.ErrPeerNotFound
	}
	if p.negotiationID != negotiationID || negotiationID == "" || p.pc.SignalingState() != pion.SignalingStateStable {
		p.mu.Unlock()
		p.forwarding.Unlock()
		p.negotiation.Unlock()
		return media.ErrNegotiation
	}
	if p.ready.Load() {
		p.mu.Unlock()
		p.forwarding.Unlock()
		p.negotiation.Unlock()
		return nil
	}
	p.ready.Store(true)
	p.pendingReadyAt = time.Time{}
	subs := make([]*subscription, 0, len(p.pendingSubscriptions))
	for _, sub := range p.pendingSubscriptions {
		if p.subscriptions[sub.source.metadata.ID] == sub {
			sub.ready.Store(true)
			subs = append(subs, sub)
		}
	}
	p.pendingSubscriptions = nil
	p.mu.Unlock()
	p.forwarding.Unlock()
	p.negotiation.Unlock()
	for _, sub := range subs {
		sub.source.requestPLI()
	}
	return nil
}

// ICE applies a validated ICE candidate to the current media connection.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//   - candidate: Candidate session or ICE record to validate before use.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) ICE(ctx context.Context, id string, candidate *pion.ICECandidateInit) error {
	p, err := m.get(id)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if candidate == nil {
		candidate = &pion.ICECandidateInit{}
	}
	if len(candidate.Candidate) > 4096 || strings.ContainsAny(candidate.Candidate, "\r\n") {
		return media.ErrInvalid
	}
	if candidate.Candidate != "" {
		parsed, parseErr := ice.UnmarshalCandidate(strings.TrimPrefix(candidate.Candidate, "candidate:"))
		if parseErr != nil {
			return media.ErrInvalid
		}
		if parsed.Component() != 1 {
			return media.ErrInvalid
		}
	}
	if candidate.SDPMid != nil && len(*candidate.SDPMid) > 64 {
		return media.ErrInvalid
	}
	if candidate.UsernameFragment != nil && len(*candidate.UsernameFragment) > 256 {
		return media.ErrInvalid
	}
	p.negotiation.Lock()
	defer p.negotiation.Unlock()
	p.mu.Lock()
	closed := p.closing
	p.mu.Unlock()
	if closed {
		return media.ErrPeerNotFound
	}
	if p.remoteICECount >= 128 {
		return media.ErrLimit
	}
	if p.pc.RemoteDescription() == nil {
		if len(p.pendingRemoteICE) >= 64 {
			return media.ErrLimit
		}
		// Copy the structure and caller-owned optional pointers so the buffer cannot retain mutable HTTP
		// decoder objects between requests.
		buffered := *candidate
		if candidate.SDPMid != nil {
			value := *candidate.SDPMid
			buffered.SDPMid = &value
		}
		if candidate.SDPMLineIndex != nil {
			value := *candidate.SDPMLineIndex
			buffered.SDPMLineIndex = &value
		}
		if candidate.UsernameFragment != nil {
			value := *candidate.UsernameFragment
			buffered.UsernameFragment = &value
		}
		p.pendingRemoteICE = append(p.pendingRemoteICE, buffered)
		p.remoteICECount++
		return nil
	}
	if err = p.pc.AddICECandidate(*candidate); err != nil {
		return media.ErrInvalid
	}
	p.remoteICECount++
	return nil
}

// Unpublish stops publishing the specified authorized media source.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//   - trackID: Identifier of the published media track.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) Unpublish(ctx context.Context, id, trackID string) error {
	p, err := m.get(id)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	track := p.publications[trackID]
	if track != nil {
		delete(p.allowedSources, track.receiver.mid)
		if p.suspendedSources == nil {
			p.suspendedSources = map[string]string{}
		}
		p.suspendedSources[track.receiver.mid] = p.offeredTrackIDs[track.receiver.mid]
	}
	p.mu.Unlock()
	if track != nil {
		m.unpublish(track)
	}
	return nil
}

// validateSourceOffer validates that declared sources match the negotiated SDP and publication
// limits.
//
// Parameters:
//   - raw: Untrusted serialized value to decode or validate.
//   - maxPeers: Maximum peers permitted by the room budget.
//   - publications: Validated declarations of the peer's media sources.
//   - maxTracks: Maximum media tracks permitted by the publication budget.
//   - maxAudio: Maximum published audio sources.
//   - maxVideo: Maximum published video sources.
//
// Returns:
//   - result 1: The resulting source.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func validateSourceOffer(raw string, maxPeers int, publications []media.Publication, maxTracks, maxAudio, maxVideo int) (map[string]media.Source, error) {
	if len(raw) == 0 || len(raw) > media.MaxSDPBytes {
		return nil, media.ErrInvalid
	}
	var description sdp.SessionDescription
	if err := description.UnmarshalString(raw); err != nil {
		return nil, media.ErrInvalid
	}
	slotLimit := 4 * maxPeers
	if publications == nil {
		slotLimit = 2 * maxPeers
	}
	if len(description.MediaDescriptions) == 0 || len(description.MediaDescriptions) > slotLimit {
		return nil, media.ErrLimit
	}
	bundled, err := bundleOnlySections(&description)
	if err != nil {
		return nil, err
	}
	// Validate embedded candidates before passing untrusted SDP to Pion, which might only warn and
	// ignore invalid values. Full offers may include redundant component-2 candidates with rtcp-mux.
	// Trickle input permits only component-1; full SDP permits component-1 or component-2.
	attributes := [][]sdp.Attribute{description.Attributes}
	for _, section := range description.MediaDescriptions {
		attributes = append(attributes, section.Attributes)
	}
	candidates := 0
	for _, attrs := range attributes {
		for _, attr := range attrs {
			if attr.Key != "candidate" {
				continue
			}
			candidates++
			if candidates > 128 {
				return nil, media.ErrLimit
			}
			if len(attr.Value) == 0 || len(attr.Value) > 4096 {
				return nil, media.ErrInvalid
			}
			parsed, err := ice.UnmarshalCandidate(attr.Value)
			if err != nil || (parsed.Component() != 1 && parsed.Component() != 2) {
				return nil, media.ErrInvalid
			}
		}
	}
	declared := map[string]media.Source{}
	uniqueSources := map[media.Source]bool{}
	for _, publication := range publications {
		if publication.MID == "" || len(publication.MID) > 64 || strings.ContainsAny(publication.MID, "\r\n\x00 ") || len(publication.TrackID) > 128 || strings.ContainsAny(publication.TrackID, "\r\n\x00") || media.SourceKind(publication.Source) == "" || declared[publication.MID] != "" || uniqueSources[publication.Source] {
			return nil, media.ErrInvalid
		}
		declared[publication.MID] = publication.Source
		uniqueSources[publication.Source] = true
	}
	if len(publications) > maxTracks || (uniqueSources[media.SourceAudioScreen] && !uniqueSources[media.SourceVideoScreen]) {
		return nil, media.ErrLimit
	}
	active := map[string]media.Source{}
	kinds := map[string]int{}
	for _, section := range description.MediaDescriptions {
		kind := section.MediaName.Media
		if kind != "audio" && kind != "video" {
			return nil, media.ErrInvalid
		}
		if section.MediaName.Port.Value == 0 && !bundled[section] {
			continue
		}
		direction := "sendrecv"
		for _, attr := range description.Attributes {
			if attr.Key == "sendonly" || attr.Key == "sendrecv" || attr.Key == "recvonly" || attr.Key == "inactive" {
				direction = attr.Key
			}
		}
		for _, attr := range section.Attributes {
			if attr.Key == "sendonly" || attr.Key == "sendrecv" || attr.Key == "recvonly" || attr.Key == "inactive" {
				direction = attr.Key
			}
		}
		if direction != "sendonly" && direction != "sendrecv" {
			continue
		}
		mid, _ := section.Attribute("mid")
		if mid == "" || len(mid) > 64 || active[mid] != "" {
			return nil, media.ErrInvalid
		}
		source := declared[mid]
		if publications == nil {
			if kinds[kind] > 0 {
				return nil, media.ErrLimit
			}
			source = media.SourceMicrophone
			if kind == "video" {
				source = media.SourceCamera
			}
		} else if source == "" || string(media.SourceKind(source)) != kind {
			return nil, media.ErrInvalid
		}
		kinds[kind]++
		if (kind == "audio" && kinds[kind] > maxAudio) || (kind == "video" && kinds[kind] > maxVideo) || len(active) >= maxTracks {
			return nil, media.ErrLimit
		}
		supported := false
		for _, attr := range section.Attributes {
			if attr.Key == "rtpmap" {
				fields := strings.Fields(attr.Value)
				if len(fields) == 2 && ((kind == "audio" && strings.EqualFold(fields[1], "opus/48000/2")) || (kind == "video" && strings.EqualFold(fields[1], "VP8/90000"))) {
					supported = true
				}
			}
		}
		if !supported {
			return nil, media.ErrInvalid
		}
		active[mid] = source
	}
	if publications != nil && len(active) != len(publications) {
		return nil, media.ErrInvalid
	}
	return active, nil
}

// answeredSSRCs extracts the negotiated media SSRCs, including valid BUNDLE-only sections.
//
// Parameters:
//   - raw: Untrusted serialized value to decode or validate.
//
// Returns:
//   - result 1: The resulting map[uint32]bool.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func answeredSSRCs(raw string) (map[uint32]bool, error) {
	var description sdp.SessionDescription
	if err := description.UnmarshalString(raw); err != nil {
		return nil, media.ErrNegotiation
	}
	bundled, err := bundleOnlySections(&description)
	if err != nil {
		return nil, media.ErrNegotiation
	}
	result := map[uint32]bool{}
	for _, section := range description.MediaDescriptions {
		if (section.MediaName.Port.Value == 0 && !bundled[section]) || (section.MediaName.Media != "audio" && section.MediaName.Media != "video") {
			continue
		}
		direction := "sendrecv"
		for _, attrs := range [][]sdp.Attribute{description.Attributes, section.Attributes} {
			for _, attr := range attrs {
				switch attr.Key {
				case "sendonly", "sendrecv", "recvonly", "inactive":
					direction = attr.Key
				}
			}
		}
		if direction != "sendonly" && direction != "sendrecv" {
			continue
		}
		declared := map[uint32]bool{}
		repair := map[uint32]bool{}
		for _, attr := range section.Attributes {
			fields := strings.Fields(attr.Value)
			switch attr.Key {
			case "ssrc":
				if len(fields) < 2 {
					return nil, media.ErrNegotiation
				}
				id, err := strconv.ParseUint(fields[0], 10, 32)
				if err != nil || id == 0 {
					return nil, media.ErrNegotiation
				}
				declared[uint32(id)] = true
			case "ssrc-group":
				if len(fields) < 3 {
					return nil, media.ErrNegotiation
				}
				if fields[0] == "FID" || fields[0] == "FEC" || fields[0] == "FEC-FR" {
					for _, value := range fields[2:] {
						id, err := strconv.ParseUint(value, 10, 32)
						if err != nil || id == 0 {
							return nil, media.ErrNegotiation
						}
						repair[uint32(id)] = true
					}
				}
			}
		}
		for id := range declared {
			if !repair[id] {
				result[id] = true
			}
		}
	}
	return result, nil
}
