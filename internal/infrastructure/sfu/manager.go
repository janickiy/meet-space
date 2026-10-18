// Пакет sfu отвечает за передачу медиаданных.  Внутренние обработчики управления медиа-воркера
// проверяют права доступа приложения, билеты и распределенное владение.
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

// Options содержит зависимости SFU, ограничения на передачу медиаданных и обратные вызовы
// жизненного цикла.
type Options struct {
	WorkerID                                                                           string
	ICE                                                                                []pion.ICEServer
	UDPPort, UDPMinPort, UDPMaxPort, TCPPort                                           int
	NATIPs                                                                             []string
	MaxPeers, MaxRooms, MaxPublishedTracks, MaxAudioTracks, MaxVideoTracks, QueueSize  int
	MaxScreenSharers, EgressQueueSize                                                  int
	ICEDisconnectedTimeout, ICEFailedTimeout, ICEKeepaliveInterval, NegotiationTimeout time.Duration
	Logger                                                                             *slog.Logger
	// Emit должен быть ограничен. Ограниченный обработчик событий, связанный с соединением,
	// вызывает его без блокировок SFU и без переадресации RTP. Операции жизненного цикла,
	// запрашиваемые Emit, выполняются асинхронно: синхронный Leave будет ожидать обработчик
	// событий, который в данный момент вызывает Emit.
	Emit func(media.Binding, string, any)
}

// Stats содержит счетчики соединений, треков и ограниченных очередей, используемые для
// диагностики.
type Stats struct {
	Rooms             int    `json:"roomsActive"`
	Peers             int    `json:"mediaPeersActive"`
	Tracks            int    `json:"tracksPublished"`
	Subscriptions     int    `json:"subscriptions"`
	PeerConnections   uint64 `json:"peerConnectionTotal"`
	Failures          uint64 `json:"peerConnectionFailures"`
	RelayConnections  uint64 `json:"relayConnections"`  // Суммарное количество выбранных пар кандидатов с использованием TURN.
	DirectConnections uint64 `json:"directConnections"` // Совокупное количество выбранных пар кандидатов, не использующих TURN.
	Packets           uint64 `json:"packetsForwarded"`
	Bytes             uint64 `json:"bytesForwarded"`
	Dropped           uint64 `json:"packetsDropped"`
	RecordingOutputs  int    `json:"recordingOutputs"`
	RecordingDrops    uint64 `json:"recordingDrops"`
	AudioTaps         int    `json:"audioTaps"`
	AudioTapDrops     uint64 `json:"audioTapDrops"`
}

// Manager — менеджер управляет локальными медиаресурсами и синхронизирует их создание, использование и
// удаление.
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

// room группирует одноранговые узлы, публикации и состояние жизненного цикла локальной комнаты
// SFU.
type room struct {
	id       string
	mu       sync.Mutex
	peers    map[string]*peer
	tracks   map[string]*publishedTrack
	policies map[string]media.ParticipantPolicy
	screens  map[string]bool
	egresses map[string]*egress
}

// defaults заполняет отсутствующие настройки компонентов поддерживаемыми стандартными
// значениями по умолчанию.
//
// @Parameters:
//   - o: Зависимости компонентов и параметры создания.
//
// @returns:
//   - result1: Результирующие параметры.
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

// NewManager создаёт менеджер с зависимостями и ограничениями, необходимыми для одноранговых
// узлов SFU и пересылки медиаданных.
//
// @Parameters:
//   - options: Проверенные параметры создания или выполнения компонентов.
//
// @returns:
//   - result1: Созданный или загруженный менеджер.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
		// Replace сохраняет соответствие 1:1 между NAT и настроенными хостами SFU для
		// кандидатов IPv4.
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
	// Страж защищает добавления WaitGroup до тех пор, пока Shutdown не разорвёт все
	// зарегистрированные соединения. Добавьте соединение в closeWG, прежде чем удалить его с
	// карты.
	m.closeWG.Add(1)
	return m, nil
}

// Join создает или возобновляет авторизованное членство или подключение к медиакомнате,
// сохраняя текущие идентификационные данные.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - binding: Подтвержденная идентификация учетной записи, сеанса и медиа-подключения.
//
// @returns:
//   - result1: Полученное представление одноранговых узлов.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
	// Создайте соединение вне общей блокировки, но сделайте так, чтобы функция Shutdown
	// дождалась завершения регистрации или закрытия NewPeerConnection, находящегося в процессе
	// обработки.
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
			// До регистрации функция Join владеет одноранговым узлом; все пути с отказом
			// освобождают его через того же владельца жизненного цикла, вне блокировок
			// менеджера/комнаты.
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
	registered = true // Теперь менеджер владеет функциями Leave/Shutdown и peer.close.  Устанавливайте обратные
	// вызовы и запускайте фоновые задачи только после того, как регистрация будет защищена от
	// функции Shutdown.
	p.install()
	m.mu.Unlock()
	m.syncPeer(p)
	m.log(p, "media_join", nil)
	return p.view(), nil
}

// sameBinding сравнивает полные идентификационные данные на стороне сервера для двух привязок
// мультимедиа.
//
// @Parameters:
//   - a: Сравниваемое значение привязки.
//   - b: Сравниваемое значение привязки.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func sameBinding(a, b media.Binding) bool {
	return a.ConferenceID == b.ConferenceID && a.ParticipantID == b.ParticipantID && a.SessionID == b.SessionID && a.ConnectionID == b.ConnectionID && a.UserID == b.UserID
}

// get ищет текущее медиасоединение, принадлежащее компоненту, под блокировкой состояния
// менеджера.
//
// @Parameters:
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Созданный или загруженный одноранговый узел.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (m *Manager) get(id string) (*peer, error) {
	m.mu.Lock()
	p := m.peers[id]
	m.mu.Unlock()
	if p == nil {
		return nil, media.ErrPeerNotFound
	}
	return p, nil
}

// PeerBinding возвращает идентификационные данные сеанса на стороне сервера, привязанные к
// одноранговому соединению.
//
// @Parameters:
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Полученная привязка.
//   - result2: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (m *Manager) PeerBinding(id string) (media.Binding, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.peers[id]
	if p == nil {
		return media.Binding{}, false
	}
	return p.binding, true
}

// Bindings возвращает текущие идентификационные данные однорангового узла для запрошенной
// медиакомнаты.
//
// @returns:
//   - result1: Собранные значения привязок.
func (m *Manager) Bindings() []media.Binding {
	m.mu.Lock()
	defer m.mu.Unlock()
	bindings := make([]media.Binding, 0, len(m.peers))
	for _, p := range m.peers {
		bindings = append(bindings, p.binding)
	}
	return bindings
}

// Leave отменяет активное членство или подключение к мультимедиа, сохраняя при этом сохранённую
// историю разговоров или конференций.
//
// @Parameters:
//   - _: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// detachLocked выводит одноранговый узел из активного состояния менеджера перед выполнением
// очистки сети вне блокировки.
//
// @Parameters:
//   - p: Состояние однорангового узла, обрабатываемое в ходе этой операции.
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

// finishDetached закрывает медиаресурсы отсоединенного однорангового узла вне блокировки
// менеджера.
//
// @Parameters:
//   - p: Состояние однорангового узла, обрабатываемое в ходе этой операции.
func (m *Manager) finishDetached(p *peer) {
	defer m.closeWG.Done()
	p.close()
	m.log(p, "media_leave", nil)
}

// LeaveConnection отключает только указанное физическое медиасоединение.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - connectionID: Идентификатор физического подключения к медиа.
func (m *Manager) LeaveConnection(ctx context.Context, connectionID string) {
	m.mu.Lock()
	p := m.connections[connectionID]
	m.mu.Unlock()
	if p != nil {
		_ = m.Leave(ctx, p.id)
	}
}

// CloseConference закрывает медиаузлы и принадлежащие ресурсы одной конференции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
	// Учитываются очистка координатора и очистка соединения.  При завершении работы ожидается
	// закрытие всех конференций, начатое до установки глобального флага завершения работы.
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

// Shutdown останавливает выполнение новых задач и ожидает завершения закрытия ресурсов,
// принадлежащих компоненту.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// waitClosed ожидает завершения закрытия ресурсов отсоединенных одноранговых узлов менеджера.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - done: Канал закрывается после завершения работы принадлежащего фонового ресурса.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func waitClosed(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Snapshot возвращает согласованный снимок текущего состояния медиаресурсов и счетчиков
// менеджера.
//
// @returns:
//   - result1: Итоговая статистика.
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

// newRoom создает локальные для комнаты реестры треков и одноранговых подключений.
//
// @Parameters:
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Созданная или загруженная комната.
func newRoom(id string) *room {
	return &room{id: id, peers: map[string]*peer{}, tracks: map[string]*publishedTrack{}, policies: map[string]media.ParticipantPolicy{}, screens: map[string]bool{}, egresses: map[string]*egress{}}
}

// HasConference сообщает, является ли данный менеджер владельцем запрашиваемого конференц-зала.
//
// @Parameters:
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (m *Manager) HasConference(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rooms[id] != nil
}

// log — записывает в журнал ограниченную диагностику компонента без использования приватных
// сигнальных данных.
//
// @Parameters:
//   - p: Состояние однорангового узла, обрабатываемое в ходе этой операции.
//   - event: Доверенное событие для публикации или обработки.
//   - fields: Ограниченные структурированные диагностические поля.
func (m *Manager) log(p *peer, event string, fields []any) {
	args := []any{"worker_id", m.opts.WorkerID, "conference_id", p.binding.ConferenceID, "participant_id", p.binding.ParticipantID, "session_id", p.binding.SessionID, "connection_id", p.binding.ConnectionID, "media_peer_id", p.id, "event_type", event}
	args = append(args, fields...)
	m.opts.Logger.Info("media event", args...)
}

// supported — поддерживает проверку того, поддерживается ли предоставленный медиакодек в SFU.
//
// @Parameters:
//   - track: Метаданные медиадорожки или согласованный экземпляр дорожки.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func supported(track *pion.TrackRemote) bool {
	return (track.Kind() == pion.RTPCodecTypeAudio && strings.EqualFold(track.Codec().MimeType, pion.MimeTypeOpus)) ||
		(track.Kind() == pion.RTPCodecTypeVideo && strings.EqualFold(track.Codec().MimeType, pion.MimeTypeVP8))
}
