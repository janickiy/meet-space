// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

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

// ErrNoMedia сохранено для совместимости с устаревшими вызовами приема данных.
var ErrNoMedia = records.ErrNoMedia

// Options содержит зависимости, связанные с записью и приемом данных, настройки мультимедиа и
// ограничения жизненного цикла.
type Options struct {
	StoragePath string
	FFmpegPath  string
	UDPPort     int
	TCPPort     int
	NATIPs      []string
	Logger      *log.Logger
	OnStarted   func(context.Context, string)
	OnFailed    func(context.Context, string, error)
	MaxSessions int // Максимальное количество одновременных устаревших записей; при значении 0 по умолчанию
	// используется безопасное значение 2.
}

// Manager — менеджер управляет ресурсами мультимедиа для записи и приема данных и синхронизирует их
// создание и удаление.
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

// Active возвращает количество операций, принадлежащих компоненту, которые необходимо завершить
// до завершения работы.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
func (m *Manager) Active() int { m.mu.Lock(); defer m.mu.Unlock(); return len(m.sessions) }

// NewManager создает менеджер с зависимостями и ограничениями, необходимыми для приема данных
// при записи WebRTC.
//
// @Parameters:
//   - options: Проверенные параметры создания или выполнения компонентов.
//
// @returns:
//   - result1: Созданный или загруженный менеджер.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
		// Принимает обычные общедоступные IPs и устаревшие внешние/локальные форматы.  Заменяет
		// перезаписанные адреса хостов только для настроенного семейства IP.
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

// Prepare — подготавливает авторизованное состояние сеанса приема данных при записи до начала
// согласования мультимедиа.
//
// @Parameters:
//   - recordID: Внешняя запись UUID.
//   - segmentDurationSec: Запрошенная продолжительность сегмента в секундах.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
		// Repeated Команды start не изменяют конфигурацию уже активного сеанса.
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

// HandleOffer передает предложение SDP для записи менеджеру приема данных и возвращает
// согласованный ответ.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - recordID: Внешняя запись UUID.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Итоговый ответ веб-RTC.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (m *Manager) HandleOffer(ctx context.Context, recordID string, request records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error) {
	s, err := m.session(recordID)
	if err != nil {
		return records.WebRTCAnswerResponse{}, err
	}

	return s.handleOffer(ctx, request)
}

// Stop останавливает активный сеанс приема данных с записывающего устройства и освобождает
// принадлежащие ему ресурсы.
//
// @Parameters:
//   - recordID: Внешняя запись UUID.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (m *Manager) Stop(recordID string) error {
	m.mu.Lock()
	s, ok := m.sessions[recordID]
	if ok {
		// Сессия остается в собственности до тех пор, пока не будут остановлены ее транспортный
		// и процессный компоненты: при завершении работы и достижении лимита приема данных
		// также должны учитываться записи, которые не были приняты.
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

// session — сессия находит или создает состояние приема данных с записывающего устройства, связанное с
// запрошенной записью.
//
// @Parameters:
//   - recordID: Внешняя запись UUID.
//
// @returns:
//   - result1: Сессия, созданная или загруженная.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (m *Manager) session(recordID string) (*session, error) {
	m.mu.Lock()
	s, ok := m.sessions[recordID]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("worker session is not prepared")
	}

	return s, nil
}

// session — сессия хранит данные, используемые для приема данных с записывающего устройства WebRTC.
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

// handleOffer согласовывает предложение SDP записывающего устройства и настраивает ожидаемые
// медиапотоки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Итоговый ответ веб-RTC.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// handleTrack начинает прием данных с одного согласованного входящего записывающего устройства.
//
// @Parameters:
//   - pc: Одноранговое соединение, к которому подключено записывающее устройство.
//   - track: Метаданные медиадорожки или согласованный экземпляр дорожки.
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

// startFFmpeg запускает ограниченный процесс записи FFmpeg после получения необходимых
// медиапотоков.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// requestVideoKeyframes запрашивает периодические ключевые кадры видео для сеанса приема данных
// с записывающего устройства.
//
// @Parameters:
//   - track: Метаданные медиадорожки или согласованный экземпляр дорожки.
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

// writePLI записывает информацию о потере изображения, чтобы запросить у отправителя видео
// новый ключевой кадр.
//
// @Parameters:
//   - track: Метаданные медиадорожки или согласованный экземпляр дорожки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// forwardRTP пересылает закодированный RTP с согласованной дорожки на локальный транспортный
// канал записи.
//
// @Parameters:
//   - track: Метаданные медиадорожки или согласованный экземпляр дорожки.
//   - port: Настроенный порт прослушивателя TCP или UDP.
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

// minStopDelay вычисляет минимальную задержку, необходимую для прекращения приема данных при
// записи.
//
// @returns:
//   - result1: Продолжительность или ограниченный интервал ожидания, вычисленный в ходе
//     операции.
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

// waitBeforeStop ожидает окончания задержки перед прекращением приема данных, если только его
// контекст не будет отменен.
//
// @Parameters:
//   - startedAt: Отметка времени начала используется для расчета минимального времени записи.
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

// stop останавливает активный сеанс приема данных при записи и освобождает принадлежащие ему
// ресурсы.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
		// PeerConnection, который уже был принят, может все равно передать свой первый трек.
		// Блокирует новые предложения, сохраняя исходные ограничения на передачу медиаданных.
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
		// Startup владеет незарегистрированным дочерним процессом до тех пор, пока не
		// завершится его завершение.
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

// fail фиксирует сбой безопасного сеанса приема данных при записи и запускает процесс очистки
// принадлежащих ресурсов.
//
// @Parameters:
//   - err: Ошибка классификации, инкапсуляции или безопасного оповещения.
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
			// В случае сбоя остановите воспроизведение мультимедиа, прежде чем снять блокировку
			// записи конференции.
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

// armTrackWaitTimeout запускает ограниченное ожидание медиапотоков, обещанных предложением SDP.
//
// @Parameters:
//   - timeout: Максимальное время, отведенное на выполнение этой операции.
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

// hasExpectedTracksLocked проверяет, все ли ожидаемые медиапотоки получены, пока состояние
// сеанса заблокировано.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
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

// offerMediaKinds извлекает ожидаемые типы аудио- и видеопотоков из предложения SDP.
//
// @Parameters:
//   - sdp: Описание удаленного сеанса для проверки.
//
// @returns:
//   - result1: Результирующая карта [строка] int.
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

// codecKind сопоставляет согласованный кодек с типом аудио- или видеопотока.
//
// @Parameters:
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//
// @returns:
//   - result1: Нормализованный тип аудио- или видеоданных.
func codecKind(kind pionwebrtc.RTPCodecType) string {
	if kind == pionwebrtc.RTPCodecTypeAudio {
		return "audio"
	}

	return "video"
}

// reserveUDPPort резервирует доступный локальный порт UDP для входа RTP-to-FFmpeg.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// writeRTPPacket записывает один полный пакет RTP в выбранный локальный транспорт.
//
// @Parameters:
//   - conn: Подключение, принадлежащее текущей операции.
//   - packet: Закодированный RTP или медиапакет.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func writeRTPPacket(conn *net.UDPConn, packet *pionrtp.Packet) error {
	raw, err := packet.Marshal()
	if err != nil {
		return err
	}
	_, err = conn.Write(raw)

	return err
}
