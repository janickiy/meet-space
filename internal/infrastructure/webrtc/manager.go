package webrtc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/janickiy/meet-space/internal/domain/records"
	"github.com/janickiy/meet-space/internal/infrastructure/ffmpeg"
	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	pionrtp "github.com/pion/rtp"
	pionwebrtc "github.com/pion/webrtc/v4"
)

// ErrNoMedia is retained for compatibility with legacy ingest callers.
var ErrNoMedia = records.ErrNoMedia

// Options contains recording-ingestion dependencies, media settings and lifecycle limits.
type Options struct {
	StoragePath string
	FFmpegPath  string
	UDPPort     int
	TCPPort     int
	NATIPs      []string
	Logger      *log.Logger
	OnStarted   func(context.Context, string)
	OnFailed    func(context.Context, string, error)
	MaxSessions int // Maximum concurrent legacy recordings; zero selects the safe default of two.
}

// Manager owns recording-ingestion media resources and synchronizes their creation and teardown.
type Manager struct {
	api      *pionwebrtc.API
	storage  string
	recorder *ffmpeg.SegmentRecorder
	logger   *log.Logger
	iceConn  net.PacketConn
	iceTCP   net.Listener

	onStarted func(context.Context, string)
	onFailed  func(context.Context, string, error)

	mu           sync.Mutex
	sessions     map[string]*session
	closed       bool
	maxSessions  int
	shutdownDone chan struct{}
}

// Active returns the count of component-owned operations still requiring completion before
// shutdown.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
func (m *Manager) Active() int { m.mu.Lock(); defer m.mu.Unlock(); return len(m.sessions) }

// NewManager constructs the manager with the dependencies and limits required for WebRTC
// recording ingestion.
//
// Parameters:
//   - options: Validated component construction or execution options.
//
// Returns:
//   - result 1: The constructed or loaded manager.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NewManager(options Options) (*Manager, error) {
	if options.MaxSessions <= 0 {
		options.MaxSessions = 2
	}
	logger := options.Logger
	if logger == nil {
		logger = log.Default()
	}
	mediaEngine := &pionwebrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, fmt.Errorf("register codecs: %w", err)
	}
	interceptors := &interceptor.Registry{}
	if err := pionwebrtc.RegisterDefaultInterceptors(mediaEngine, interceptors); err != nil {
		return nil, fmt.Errorf("register interceptors: %w", err)
	}
	settingEngine := pionwebrtc.SettingEngine{}
	var iceConn net.PacketConn
	var iceTCP net.Listener
	networkTypes := make([]pionwebrtc.NetworkType, 0, 2)
	if options.UDPPort > 0 {
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: options.UDPPort})
		if err != nil {
			return nil, fmt.Errorf("listen WebRTC UDP mux: %w", err)
		}
		iceConn = conn
		networkTypes = append(networkTypes, pionwebrtc.NetworkTypeUDP4)
		settingEngine.SetICEUDPMux(pionwebrtc.NewICEUDPMux(nil, conn))
		logger.Printf("WebRTC ICE UDP mux is listening on :%d/udp", options.UDPPort)
	}
	if options.TCPPort > 0 {
		listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: options.TCPPort})
		if err != nil {
			if iceConn != nil {
				_ = iceConn.Close()
			}
			return nil, fmt.Errorf("listen WebRTC TCP mux: %w", err)
		}
		iceTCP = listener
		networkTypes = append(networkTypes, pionwebrtc.NetworkTypeTCP4)
		settingEngine.SetICETCPMux(pionwebrtc.NewICETCPMux(nil, listener, 8))
		logger.Printf("WebRTC ICE TCP mux is listening on :%d/tcp", options.TCPPort)
	}
	if len(networkTypes) > 0 {
		settingEngine.SetNetworkTypes(networkTypes)
	}
	if len(options.NATIPs) > 0 {
		// Accept ordinary public IPs and the legacy external/local form. Replace rewrites host addresses
		// only for the configured IP family.
		rules := make([]pionwebrtc.ICEAddressRewriteRule, 0, len(options.NATIPs)+1)
		externalIPs := make([]string, 0, len(options.NATIPs))
		for _, address := range options.NATIPs {
			external, local, mapped := strings.Cut(address, "/")
			externalIPs = append(externalIPs, external)
			if mapped {
				rules = append(rules, pionwebrtc.ICEAddressRewriteRule{
					External:        []string{external},
					Local:           local,
					AsCandidateType: pionwebrtc.ICECandidateTypeHost,
					Mode:            pionwebrtc.ICEAddressRewriteReplace,
				})
			}
		}
		rules = append(rules, pionwebrtc.ICEAddressRewriteRule{
			External:        externalIPs,
			AsCandidateType: pionwebrtc.ICECandidateTypeHost,
			Mode:            pionwebrtc.ICEAddressRewriteReplace,
		})
		if err := settingEngine.SetICEAddressRewriteRules(rules...); err != nil {
			if iceConn != nil {
				_ = iceConn.Close()
			}
			if iceTCP != nil {
				_ = iceTCP.Close()
			}
			return nil, fmt.Errorf("configure WebRTC ICE address rewriting: %w", err)
		}
		logger.Printf("WebRTC ICE NAT 1:1 IPs: %s", strings.Join(options.NATIPs, ","))
	}

	return &Manager{
		api: pionwebrtc.NewAPI(
			pionwebrtc.WithMediaEngine(mediaEngine),
			pionwebrtc.WithInterceptorRegistry(interceptors),
			pionwebrtc.WithSettingEngine(settingEngine),
		),
		storage:     options.StoragePath,
		recorder:    ffmpeg.NewSegmentRecorder(options.FFmpegPath, logger),
		logger:      logger,
		iceConn:     iceConn,
		iceTCP:      iceTCP,
		onStarted:   options.OnStarted,
		onFailed:    options.OnFailed,
		sessions:    make(map[string]*session),
		maxSessions: options.MaxSessions,
	}, nil
}

// Prepare prepares the authorized recording ingestion session state before media negotiation
// begins.
//
// Parameters:
//   - recordID: External recording UUID.
//   - segmentDurationSec: Requested segment duration in seconds.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) Prepare(recordID string, segmentDurationSec int) error {
	if segmentDurationSec <= 0 {
		segmentDurationSec = 5
	}
	recordDir := filepath.Join(m.storage, "records", recordID)
	tempDir := filepath.Join(m.storage, "tmp", recordID)
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[recordID]; ok {
		// Repeated start commands do not change an already active session's configuration.
		return nil
	}
	if m.closed || (m.maxSessions > 0 && len(m.sessions) >= m.maxSessions) {
		return fmt.Errorf("recorder draining or session limit reached")
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.sessions[recordID] = &session{
		recordID:           recordID,
		ctx:                ctx,
		cancel:             cancel,
		manager:            m,
		recordDir:          recordDir,
		tempDir:            tempDir,
		segmentDurationSec: segmentDurationSec,
		processReady:       make(chan struct{}),
	}

	return nil
}

// HandleOffer passes a recording SDP offer to the ingestion manager and returns its negotiated
// answer.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - recordID: External recording UUID.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting web rtc answer response.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) HandleOffer(ctx context.Context, recordID string, request records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error) {
	s, err := m.session(recordID)
	if err != nil {
		return records.WebRTCAnswerResponse{}, err
	}

	return s.handleOffer(ctx, request)
}

// Stop stops the active recording ingestion session operation and releases its owned resources.
//
// Parameters:
//   - recordID: External recording UUID.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) Stop(recordID string) error {
	m.mu.Lock()
	s, ok := m.sessions[recordID]
	if ok {
		// Keep the session owned until its transport and process have stopped:
		// Shutdown and the admission limit must also see draining recordings.
		s.mu.Lock()
		s.stopRequested = true
		s.mu.Unlock()
	}
	m.mu.Unlock()
	if !ok {
		return ErrNoMedia
	}
	defer func() {
		m.mu.Lock()
		if m.sessions[recordID] == s {
			delete(m.sessions, recordID)
		}
		m.mu.Unlock()
	}()

	return s.stop()
}

// session finds or creates the recording-ingestion state associated with the requested
// recording.
//
// Parameters:
//   - recordID: External recording UUID.
//
// Returns:
//   - result 1: The constructed or loaded session.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (m *Manager) session(recordID string) (*session, error) {
	m.mu.Lock()
	s, ok := m.sessions[recordID]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("worker session is not prepared")
	}

	return s, nil
}

// session holds the session data used by WebRTC recording ingestion.
type session struct {
	recordID           string
	ctx                context.Context
	cancel             context.CancelFunc
	manager            *Manager
	recordDir          string
	tempDir            string
	segmentDurationSec int

	mu            sync.Mutex
	pc            *pionwebrtc.PeerConnection
	process       *ffmpeg.SegmentProcess
	tracks        []ffmpeg.RTPTrack
	expected      map[string]int
	received      map[string]int
	processSince  time.Time
	waitTimer     *time.Timer
	processReady  chan struct{}
	startDone     chan struct{}
	startOnce     sync.Once
	failOnce      sync.Once
	failed        bool
	stopping      bool
	stopRequested bool
}

// handleOffer negotiates the recording peer's SDP offer and configures its expected media
// tracks.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting web rtc answer response.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *session) handleOffer(ctx context.Context, request records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error) {
	if request.Type == "" {
		request.Type = "offer"
	}
	if strings.TrimSpace(request.SDP) == "" {
		return records.WebRTCAnswerResponse{}, fmt.Errorf("sdp is required")
	}
	expected := offerMediaKinds(request.SDP)
	if len(expected) == 0 {
		return records.WebRTCAnswerResponse{}, fmt.Errorf("offer must contain audio or video media section")
	}
	pc, err := s.manager.api.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		return records.WebRTCAnswerResponse{}, fmt.Errorf("create peer connection: %w", err)
	}

	s.mu.Lock()
	if s.failed || s.stopRequested || s.stopping || s.ctx.Err() != nil {
		s.mu.Unlock()
		_ = pc.Close()
		return records.WebRTCAnswerResponse{}, fmt.Errorf("WebRTC session has failed")
	}
	previous := s.pc
	s.pc = pc
	s.expected = expected
	s.received = make(map[string]int, len(expected))
	s.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}

	pc.OnTrack(

		func(track *pionwebrtc.TrackRemote, _ *pionwebrtc.RTPReceiver) {
			s.handleTrack(pc, track)
		})
	pc.OnConnectionStateChange(

		func(state pionwebrtc.PeerConnectionState) {
			s.manager.logger.Printf("record %s WebRTC state: %s", s.recordID, state.String())
			if state == pionwebrtc.PeerConnectionStateFailed {
				s.mu.Lock()
				current := s.pc == pc
				s.mu.Unlock()
				if current {
					s.fail(fmt.Errorf("webrtc connection failed"))
				}
			}
		})

	offer := pionwebrtc.SessionDescription{Type: pionwebrtc.SDPTypeOffer, SDP: request.SDP}
	if err := pc.SetRemoteDescription(offer); err != nil {
		_ = pc.Close()
		return records.WebRTCAnswerResponse{}, fmt.Errorf("set remote description: %w", err)
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		_ = pc.Close()
		return records.WebRTCAnswerResponse{}, fmt.Errorf("create answer: %w", err)
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		_ = pc.Close()
		return records.WebRTCAnswerResponse{}, fmt.Errorf("set local description: %w", err)
	}

	select {
	case <-pionwebrtc.GatheringCompletePromise(pc):
	case <-time.After(3 * time.Second):
		s.manager.logger.Printf("record %s WebRTC answer ICE gathering timeout; returning partial answer", s.recordID)
	case <-ctx.Done():
		_ = pc.Close()
		return records.WebRTCAnswerResponse{}, ctx.Err()
	case <-s.ctx.Done():
		_ = pc.Close()
		return records.WebRTCAnswerResponse{}, s.ctx.Err()
	}
	s.mu.Lock()
	active := !s.failed && !s.stopRequested && !s.stopping && s.ctx.Err() == nil && s.pc == pc
	s.mu.Unlock()
	if !active {
		_ = pc.Close()
		return records.WebRTCAnswerResponse{}, fmt.Errorf("WebRTC session stopped during offer")
	}
	local := pc.LocalDescription()
	if local == nil {
		return records.WebRTCAnswerResponse{}, fmt.Errorf("local SDP answer is empty")
	}
	s.armTrackWaitTimeout(20 * time.Second)

	return records.WebRTCAnswerResponse{Type: "answer", SDP: local.SDP}, nil
}

// handleTrack starts ingestion of one negotiated incoming recording track.
//
// Parameters:
//   - pc: Peer connection owning the recording track.
//   - track: Media-track metadata or negotiated track instance.
func (s *session) handleTrack(pc *pionwebrtc.PeerConnection, track *pionwebrtc.TrackRemote) {
	port, err := reserveUDPPort()
	if err != nil {
		s.fail(fmt.Errorf("reserve RTP port: %w", err))
		return
	}
	codec := track.Codec()
	ffTrack := ffmpeg.RTPTrack{
		Kind:        codecKind(track.Kind()),
		MimeType:    codec.MimeType,
		PayloadType: uint8(codec.PayloadType),
		ClockRate:   codec.ClockRate,
		Channels:    uint16(codec.Channels),
		Port:        port,
		FmtpLine:    codec.SDPFmtpLine,
	}

	s.mu.Lock()
	if s.failed || s.stopping || s.ctx.Err() != nil || s.pc != pc {
		s.mu.Unlock()
		return
	}
	s.tracks = append(s.tracks, ffTrack)
	if s.received == nil {
		s.received = make(map[string]int)
	}
	s.received[ffTrack.Kind]++
	ready := s.hasExpectedTracksLocked()
	s.mu.Unlock()

	s.manager.logger.Printf("record %s received %s track codec=%s port=%d", s.recordID, ffTrack.Kind, ffTrack.MimeType, port)
	go s.forwardRTP(track, port)
	s.requestVideoKeyframes(track)

	if ready {
		s.startOnce.Do(

			func() {
				if err := s.startFFmpeg(); err != nil {
					s.fail(err)
				}
			})
	}
}

// startFFmpeg starts the bounded FFmpeg recording process after the required media tracks are
// available.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *session) startFFmpeg() error {
	s.mu.Lock()
	if s.failed || s.stopping || s.ctx.Err() != nil {
		s.mu.Unlock()
		return fmt.Errorf("WebRTC session has failed")
	}
	if s.process != nil {
		s.mu.Unlock()
		return nil
	}
	if s.waitTimer != nil {
		s.waitTimer.Stop()
		s.waitTimer = nil
	}
	tracks := append([]ffmpeg.RTPTrack(nil), s.tracks...)
	s.startDone = make(chan struct{})
	startDone := s.startDone
	s.mu.Unlock()

	process, err := s.manager.recorder.Start(s.ctx, s.recordID, tracks, s.tempDir, s.recordDir, s.segmentDurationSec)
	if err != nil {
		close(startDone)
		return err
	}
	s.mu.Lock()
	if s.failed || s.stopping || s.ctx.Err() != nil {
		s.mu.Unlock()
		_ = process.Stop(time.Second)
		close(startDone)
		return fmt.Errorf("WebRTC session has failed")
	}
	s.process = process
	s.processSince = time.Now()
	processReady := s.processReady
	if processReady != nil {
		close(processReady)
	}
	close(startDone)
	s.mu.Unlock()
	go func() {
		<-process.Done()
		err := process.WaitErr()
		if err == nil {
			err = fmt.Errorf("FFmpeg exited before recording stop")
		}
		s.fail(fmt.Errorf("ffmpeg recording terminated: %w; stderr=%s", err, process.Stderr()))
	}()
	if s.manager.onStarted != nil {
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		defer cancel()
		s.manager.onStarted(ctx, s.recordID)
	}

	return nil
}

// requestVideoKeyframes requests periodic video keyframes for the recording ingestion session.
//
// Parameters:
//   - track: Media-track metadata or negotiated track instance.
func (s *session) requestVideoKeyframes(track *pionwebrtc.TrackRemote) {
	if track.Kind() != pionwebrtc.RTPCodecTypeVideo {
		return
	}
	go func() {
		select {
		case <-s.processReady:
		case <-s.ctx.Done():
			return
		}
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		for {
			if err := s.writePLI(track); err != nil {
				s.manager.logger.Printf("record %s request video keyframe failed: %v", s.recordID, err)
			}
			select {
			case <-ticker.C:
			case <-timer.C:
				return
			case <-s.ctx.Done():
				return
			}
		}
	}()
}

// writePLI writes a picture-loss indication to request a fresh keyframe from the video sender.
//
// Parameters:
//   - track: Media-track metadata or negotiated track instance.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *session) writePLI(track *pionwebrtc.TrackRemote) error {
	s.mu.Lock()
	pc := s.pc
	s.mu.Unlock()
	if pc == nil {
		return nil
	}

	return pc.WriteRTCP([]rtcp.Packet{
		&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())},
	})
}

// forwardRTP forwards encoded RTP from the negotiated track to the local recorder transport.
//
// Parameters:
//   - track: Media-track metadata or negotiated track instance.
//   - port: Configured TCP or UDP listener port.
func (s *session) forwardRTP(track *pionwebrtc.TrackRemote, port int) {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		s.fail(fmt.Errorf("resolve RTP UDP addr: %w", err))
		return
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		s.fail(fmt.Errorf("dial RTP UDP addr: %w", err))
		return
	}
	defer conn.Close()

	for {
		packet, _, err := track.ReadRTP()
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
				s.manager.logger.Printf("record %s track read stopped: %v", s.recordID, err)
			}
			return
		}
		if err := writeRTPPacket(conn, packet); err != nil {
			if errors.Is(err, syscall.ECONNREFUSED) {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			s.fail(fmt.Errorf("write RTP packet: %w", err))
			return
		}
	}
}

// minStopDelay computes the minimum delay needed before stopping recording ingestion.
//
// Returns:
//   - result 1: The duration or bounded waiting interval computed by the operation.
func (s *session) minStopDelay() time.Duration {
	duration := time.Duration(s.segmentDurationSec) * time.Second
	if duration < 2*time.Second {
		duration = 2 * time.Second
	}
	if duration > 8*time.Second {
		duration = 8 * time.Second
	}

	return duration
}

// waitBeforeStop waits for the ingestion stop delay unless its context is canceled.
//
// Parameters:
//   - startedAt: Start timestamp used to calculate the minimum recording lifetime.
func (s *session) waitBeforeStop(startedAt time.Time) {
	if startedAt.IsZero() {
		return
	}
	wait := time.Until(startedAt.Add(s.minStopDelay()))
	if wait <= 0 {
		return
	}
	s.manager.logger.Printf("record %s stop requested before first segment window; waiting %s before FFmpeg stop", s.recordID, wait.Round(100*time.Millisecond))
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-s.ctx.Done():
	}
}

// stop stops the active recording ingestion session operation and releases its owned resources.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *session) stop() error {
	s.mu.Lock()
	s.stopRequested = true
	hasProcess := s.process != nil
	if hasProcess {
		s.stopping = true
	}
	if s.waitTimer != nil {
		s.waitTimer.Stop()
		s.waitTimer = nil
	}
	s.mu.Unlock()
	if !hasProcess {
		// An already accepted PeerConnection may still deliver its first track.
		// Fence new offers while preserving the original bounded media grace.
		timer := time.NewTimer(s.minStopDelay())
		select {
		case <-s.processReady:
		case <-timer.C:
		case <-s.ctx.Done():
		}
		timer.Stop()
	}
	s.mu.Lock()
	s.stopping = true
	pc, process, processSince, startDone := s.pc, s.process, s.processSince, s.startDone
	s.mu.Unlock()
	if process == nil {
		s.cancel()
		// Startup owns an unregistered child until its completion is joined.
		if startDone != nil {
			<-startDone
		}
		if pc != nil {
			_ = pc.Close()
		}
		return ErrNoMedia
	}
	s.waitBeforeStop(processSince)
	if err := process.Stop(10 * time.Second); err != nil {
		if pc != nil {
			_ = pc.Close()
		}
		s.cancel()
		return err
	}
	if pc != nil {
		_ = pc.Close()
	}
	s.cancel()

	return nil
}

// fail records a safe recording ingestion session failure and starts the owned-resource cleanup
// path.
//
// Parameters:
//   - err: Error to classify, wrap or report safely.
func (s *session) fail(err error) {
	s.failOnce.Do(

		func() {
			s.mu.Lock()
			if s.failed || s.stopping || s.ctx.Err() != nil {
				s.mu.Unlock()
				return
			}
			s.failed = true
			pc := s.pc
			process := s.process
			startDone := s.startDone
			if s.waitTimer != nil {
				s.waitTimer.Stop()
				s.waitTimer = nil
			}
			s.mu.Unlock()
			// On failure, stop media before releasing the conference recording lock.
			s.cancel()
			if pc != nil {
				_ = pc.Close()
			}
			if process != nil {
				_ = process.Stop(time.Second)
			} else if startDone != nil {
				<-startDone
			}
			s.manager.mu.Lock()
			if s.manager.sessions[s.recordID] == s {
				delete(s.manager.sessions, s.recordID)
			}
			s.manager.mu.Unlock()
			s.manager.logger.Printf("record %s ingest failed: %v", s.recordID, err)
			if s.manager.onFailed != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				s.manager.onFailed(ctx, s.recordID, err)
			}
		})
}

// armTrackWaitTimeout starts the bounded wait for the media tracks promised by the SDP offer.
//
// Parameters:
//   - timeout: Maximum time allowed for this operation.
func (s *session) armTrackWaitTimeout(timeout time.Duration) {
	s.mu.Lock()
	if s.failed || s.stopRequested || s.stopping || s.ctx.Err() != nil || s.waitTimer != nil || s.process != nil {
		s.mu.Unlock()
		return
	}
	s.waitTimer = time.AfterFunc(timeout,

		func() {
			s.mu.Lock()
			hasProcess := s.process != nil
			s.mu.Unlock()
			if !hasProcess {
				s.fail(fmt.Errorf("%w: expected media tracks were not received", ErrNoMedia))
			}
		})
	s.mu.Unlock()
}

// hasExpectedTracksLocked checks whether every expected track has arrived while session state is
// locked.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (s *session) hasExpectedTracksLocked() bool {
	if len(s.expected) == 0 {
		return len(s.tracks) > 0
	}
	for kind, expectedCount := range s.expected {
		if expectedCount > 0 && s.received[kind] == 0 {
			return false
		}
	}

	return true
}

// offerMediaKinds extracts the expected audio and video kinds from an SDP offer.
//
// Parameters:
//   - sdp: Remote session description to validate.
//
// Returns:
//   - result 1: The resulting map[string]int.
func offerMediaKinds(sdp string) map[string]int {
	result := make(map[string]int)
	for _, rawLine := range strings.Split(sdp, "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, "m=") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "m="))
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "audio" || fields[0] == "video" {
			result[fields[0]]++
		}
	}

	return result
}

// codecKind maps a negotiated codec to its audio or video media kind.
//
// Parameters:
//   - kind: Validated event, job, media or resource category.
//
// Returns:
//   - result 1: Normalized audio or video media kind.
func codecKind(kind pionwebrtc.RTPCodecType) string {
	if kind == pionwebrtc.RTPCodecTypeAudio {
		return "audio"
	}

	return "video"
}

// reserveUDPPort reserves an available local UDP port for the RTP-to-FFmpeg input.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func reserveUDPPort() (int, error) {
	addr, err := net.ResolveUDPAddr("udp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	return conn.LocalAddr().(*net.UDPAddr).Port, nil
}

// writeRTPPacket writes one complete RTP packet to the selected local transport.
//
// Parameters:
//   - conn: Connection owned by the current operation.
//   - packet: Encoded RTP or media packet.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func writeRTPPacket(conn *net.UDPConn, packet *pionrtp.Packet) error {
	raw, err := packet.Marshal()
	if err != nil {
		return err
	}
	_, err = conn.Write(raw)

	return err
}
