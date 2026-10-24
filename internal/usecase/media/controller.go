// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

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

// Registry — реестр определяет заменяемый контракт зависимостей для аутентифицированного управления
// медиакомнатами.
type Registry interface {
	// Workers возвращает список активных медиа-воркеров, которые могут владеть комнатой.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//
	// @returns:
	//   - result1: Собранные значения для воркера.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Workers(context.Context) ([]domain.Worker, error)
	// Claim — требование атомарно получает текущую аренду подключения к медиакомнате или запись о
	// владельце.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg3: Идентификатор работника, которому принадлежит эта операция.
	//   - arg4: Срок действия, назначенный для временного значения или записи о
	//   владельце.
	//
	// @returns:
	//   - result1: Результирующий маршрут.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Claim(context.Context, string, string, time.Duration) (domain.Route, error)
	// GetOwner загружает текущую версию медиакомнаты с распределенным владельцем и защитой от
	// несанкционированного доступа.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//
	// @returns:
	//   - result1: Результирующий маршрут.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	GetOwner(context.Context, string) (domain.Route, error)
}

// Tickets — билеты определяют заменяемый контракт зависимостей для аутентифицированного управления
// медиакомнатами.
type Tickets interface {
	// Issue создает подписанный токен, привязанный к предоставленным идентификационным данным и
	// сроку действия.
	//
	// @Parameters:
	//   - arg1: Подтвержденная идентификация учетной записи, сеанса и медиа-подключения.
	//   - arg2: Текущий владелец медиа-комнаты и версия ограждения.
	//
	// @returns:
	//   - result1: Подписанные краткосрочные учетные данные для подтвержденной личности.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Issue(domain.Binding, domain.Route) (string, error)
}

// Transport — транспорт определяет заменяемый контракт зависимостей для аутентифицированного управления
// медиакомнатами.
type Transport interface {
	// Call — вызов выполняет аутентифицированную внутреннюю операцию HTTP с выбранным медиа-воркером.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Запрошенное действие по управлению жизненным циклом или модерации.
	//   - arg3: Подтвержденная команда внутренней записи или передачи медиаданных.
	//
	// @returns:
	//   - result1: Результирующий результат.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Call(context.Context, string, domain.Command) (domain.Result, error)
}

// Sessions — сеансы определяют заменяемый контракт зависимостей для аутентифицированного управления
// медиакомнатами.
type Sessions interface {
	// ValidateSession привязывает медиаоперацию к текущему авторизованному физическому сеансу
	// WebSocket.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Постоянное или физическое состояние сессии, определяемое ее типом.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ValidateSession(context.Context, realtime.Session) error
}

// Publisher — издатель определяет заменяемый контракт зависимости для управления медиакомнатами с
// аутентификацией.
type Publisher interface {
	// Publish — издатель публикует сохраненные данные об изменении подключения к медиакомнате через
	// настроенный транспорт событий или команд.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Публикация и подписка с ограничением области видимости.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Publish(context.Context, realtime.Bus) error
}

// PolicyProvider определяет заменяемый контракт зависимости для управления медиакомнатой с
// аутентификацией.
type PolicyProvider interface {
	// MediaPolicy загружает действующие сохранённые медиа-ограничения для участника.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg3: Идентификатор участника конференции.
	//
	// @returns:
	//   - result1: Результирующая политика в отношении участника.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	MediaPolicy(context.Context, string, string) (domain.ParticipantPolicy, error)
}

// endpoint содержит данные об эндпоинте, используемые для управления медиакомнатами с
// аутентификацией.
type endpoint struct {
	binding domain.Binding
	route   domain.Route
	peerID  string
}

// Controller — контроллер привязывает клиентские сигналы к авторизации физического сеанса, владению комнатой
// и внутренней передаче медиаданных.
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

// SetPolicyProvider обновляет поставщика политик для управления медиакомнатами с
// аутентификацией.
//
// @Parameters:
//   - provider: Настроенный адаптер внешнего провайдера.
func (c *Controller) SetPolicyProvider(provider PolicyProvider) { c.policyProvider = provider }

// policy загружает действующие ограничения на использование медиа на стороне сервера для
// участника.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - binding: Подтвержденная идентификация учетной записи, сеанса и медиа-подключения.
//
// @returns:
//   - result1: Сконструированная или загруженная политика участника.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// SetParticipantPolicy обновляет политику участника для управления медиакомнатами с
// аутентификацией.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - participantID: Идентификатор участника конференции.
//   - policy: Эффективная политика в отношении медиаданных на стороне сервера.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// CloseConference закрывает медиаузлы и принадлежащие ресурсы одной конференции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// NewController создает контроллер с зависимостями и ограничениями, необходимыми для управления
// медиакомнатами с аутентификацией.
//
// @Parameters:
//   - registry: Реестр зависимостей или метрик, используемый для поиска или регистрации
//     компонента.
//   - tickets: Одноразовый или подписанный эмитент и верификатор медиа-билетов.
//   - transport: Реализация транспорта или приемник RTP для этой операции.
//   - sessions: Зависимость авторизации физического сеанса.
//   - publisher: Доверенный издатель событий.
//   - cfg: Ограничения конфигурации и обработки, контролируемые оператором.
//   - limits: Настроенные ограничения протокола, очереди и выполнения.
//
// @returns:
//   - result1: Создан или загружен контроллер.
func NewController(registry Registry, tickets Tickets, transport Transport, sessions Sessions, publisher Publisher, cfg config.MediaConfig, limits config.RealtimeConfig) *Controller {
	limits.SDPBytes = min(limits.SDPBytes, domain.MaxSDPBytes)
	limits.ICEBytes = min(limits.ICEBytes, domain.MaxICEBytes)
	return &Controller{registry: registry, tickets: tickets, transport: transport, sessions: sessions, publisher: publisher, cfg: cfg, limits: limits, peers: map[string]endpoint{}}
}

// Handle — обработчик перенаправляет проверенную операцию подключения к медиакомнате в обработчик
// приложения.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - session: Постоянное или физическое состояние сессии, определяемое ее типом.
//   - expiresAt: Абсолютный срок действия подтвержденных учетных данных для доступа.
//   - event: Доверенное событие для публикации или обработки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
	// Пустой media.leave отменяет поставленное в очередь подключение до того, как браузер
	// получит идентификатор подключения.  Единственный считыватель WebSocket выполняет его
	// после получения ответа о подключении; очистка выполняется только в рамках авторизованного
	// сеанса отправителя.
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

// join выбирает владельца медиакомнаты и подготавливает подключение для проверенного
// физического сеанса.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - binding: Подтвержденная идентификация учетной записи, сеанса и медиа-подключения.
//   - requestID: Предоставленный клиентом ключ идемпотентности для операции с возможностью
//     повторных попыток.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
	// Небольшая стабильная хэш-функция направляет запросы на новые конференции; Claim сохраняет
	// активного существующего владельца.  Это маршрутизация, а не распределённый планировщик.
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
		// Воркер подготовил подключение, но недопустимый источник захвата может так и не дойти
		// до браузера.  Освободите подготовленное медиаподключение, не оставляя его без
		// владельца.
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

// emit отправляет доверенное событие жизненного цикла по ограниченному пути доставки,
// принадлежащему компоненту.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - binding: Подтвержденная идентификация учетной записи, сеанса и медиа-подключения.
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - replyTo: Подтвержденный корреляционный идентификатор клиента для ответа об ошибке.
//   - data: Входные байты или структурированная полезная информация о событии.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Controller) emit(ctx context.Context, binding domain.Binding, kind, replyTo string, data any) error {
	event := realtime.Event(kind, binding.ConferenceID, data)
	event.ReplyTo = replyTo
	if err := c.publisher.Publish(ctx, realtime.Bus{Kind: "event", ConferenceID: binding.ConferenceID, ConnectionID: binding.ConnectionID, Event: &event}); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}

// Disconnected обрабатывает закрытие физического соединения и освобождает только те ресурсы,
// которые были связаны с этим соединением.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - session: Постоянное или физическое состояние сессии, определяемое ее типом.
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

// strictDecode декодирует ровно одно значение JSON и отклоняет неизвестные поля или завершающие
// входные данные.
//
// @Parameters:
//   - raw: Недоверенное сериализованное значение для декодирования или проверки.
//   - target: Целевое состояние, учетная запись или декодированное значение в соответствии с
//     типом.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// validUUID проверяет, что идентификатор является ненулевым UUID, прежде чем использовать его
// для хранения или маршрутизации.
//
// @Parameters:
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func validUUID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed != uuid.Nil && parsed.String() == id
}

// decode строго декодирует полезную нагрузку разрешённой команды медиасигнализации.
//
// @Parameters:
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - raw: Недоверенное сериализованное значение для декодирования или проверки.
//
// @returns:
//   - result1: Результирующий сигнал.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
