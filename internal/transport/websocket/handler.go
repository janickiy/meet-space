// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

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

// Verifier определяет заменяемый контракт на зависимости для websocket.
type Verifier interface {
	// VerifyWithExpiry проверяет токен доступа и возвращает идентификатор учетной записи и срок
	// действия токена.
	//
	// @Parameters:
	//   - arg1: Недоверенное сериализованное значение для декодирования или проверки.
	//
	// @returns:
	//   - result1: Подтвержденный идентификатор учетной записи.
	//   - result2: Временная метка или граница допустимого срока действия, связанная с
	//   результатом.
	//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	VerifyWithExpiry(string) (string, time.Time, error)
}

// PersistentVerifier привязывает токен доступа к сеансу учетной записи, который можно отозвать.
// Гостевые и устаревшие токены сохраняют первоначальный срок действия.
type PersistentVerifier interface {
	// VerifyAuthorization проверяет токен авторизации и определяет его область действия, а
	// также срок действия и принадлежность пользователя, сеанса и гостя.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Недоверенное сериализованное значение для декодирования или проверки.
	//
	// @returns:
	//   - result1: Подтвержденный идентификатор учетной записи.
	//   - result2: Область действия гостевой конференции или пустая строка для
	//   зарегистрированной учетной записи.
	//   - result3: Постоянный идентификатор сеанса на сервере или пустая строка для
	//   гостевого или устаревшего токена.
	//   - result4: Временная метка или граница допустимого срока действия, связанная с
	//   результатом.
	//   - result5: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	VerifyAuthorization(context.Context, string) (string, string, string, time.Time, error)
	// AuthorizeSession проверяет, что сеанс сервера является текущим, не был отозван и привязан
	// к запрошенной учетной записи.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Постоянный или физический идентификатор сессии, определяемый операцией.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	AuthorizeSession(context.Context, string, string) error
}

// Долговременные привязки к носителю остаются стабильными, пока WebSocket проверяет сеанс
// резервного копирования при каждом нажатии кнопки и команде. Доступ к вращению JWT не может
// изменить одноранговый узел.
var durableAuthorizationUntil = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

// Tickets определяет заменяемый контракт зависимостей для веб-сокета.
type Tickets interface {
	// SaveTicket сохраняет одноразовый билет подключения с ограниченным сроком действия.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Одноразовый билет подключения или его идентификатор на стороне сервера.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Подтвержденный аккаунт и идентификатор физического сеанса.
	//   - arg5: Срок действия, назначенный для временного значения или записи о
	//   владельце.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	SaveTicket(context.Context, string, string, domain.Identity, time.Duration) error
	// ConsumeTicket атомарно использует билет подключения, чтобы его нельзя было воспроизвести.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Одноразовый билет подключения или его идентификатор на стороне сервера.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//
	// @returns:
	//   - result1: Полученный идентификатор.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ConsumeTicket(context.Context, string, string) (domain.Identity, error)
}

// Limiter определяет заменяемый контракт зависимостей для веб-сокета.
type Limiter interface {
	// Функция Allow оценивает ограничение скорости и возвращает оставшийся бюджет и границу
	// сброса.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
	//   - arg3: Максимальное количество результатов или операций в этом пакете.
	//   - arg4: Продолжительность действия ограничения скорости или окна наблюдения.
	//
	// @returns:
	//   - result1: Результирующий результат.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Allow(context.Context, string, int, time.Duration) (ratelimit.Result, error)
}

// Handler адаптирует запросы HTTP для веб-сокета, проверяет входные данные и формирует ответ.
type Handler struct {
	hub         *usecase.Hub
	verifier    Verifier
	tickets     Tickets
	limiter     Limiter
	cfg         config.RealtimeConfig
	media       MediaController
	connections atomic.Int64
}

// MediaController определяет заменяемый контракт зависимостей для веб-сокета.
type MediaController interface {
	// Handle перенаправляет проверенную операцию веб-сокета в обработчик приложения.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Постоянное или физическое состояние сессии, определяемое ее типом.
	//   - arg3: Абсолютный срок действия подтвержденных учетных данных для доступа.
	//   - arg4: Доверенное событие для публикации или обработки.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Handle(context.Context, domain.Session, time.Time, domain.Envelope) error
}

// NewHandler создает обработчик с зависимостями и ограничениями, необходимыми для веб-сокета.
//
// @Parameters:
//   - hub: Центр управления в режиме реального времени, координирующий авторизованную доставку
//     данных через сокет.
//   - verifier: Верификатор токена или приватный верификатор PKCE в зависимости от его типа.
//   - tickets: Одноразовый или подписанный эмитент и верификатор медиа-билетов.
//   - limiter: Общий ограничитель частоты запросов.
//   - cfg: Ограничения конфигурации и обработки, контролируемые оператором.
//
// @returns:
//   - result1: Сконструированный или загруженный обработчик.
func NewHandler(hub *usecase.Hub, verifier Verifier, tickets Tickets, limiter Limiter, cfg config.RealtimeConfig) *Handler {
	return &Handler{hub: hub, verifier: verifier, tickets: tickets, limiter: limiter, cfg: cfg}
}

// SetMedia обновляет медиаданные для веб-сокета.
//
// @Parameters:
//   - controller: Политика в отношении медиаданных и контроллер команд.
//
// @returns:
//   - result1: Сконструированный или загруженный обработчик.
func (h *Handler) SetMedia(controller MediaController) *Handler { h.media = controller; return h }

// RegisterRoutes регистрирует аутентифицированные конечные точки в режиме реального времени и
// ограничения для их запросов.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
func (h *Handler) RegisterRoutes(router gin.IRouter) {
	router.GET("/api/v1/conferences/:id/ws", h.Connect)
	router.POST("/api/v1/conferences/:id/ws-ticket", h.Ticket)
	router.GET("/api/v1/webrtc/config", h.ICE)
}

// identity определяет подтвержденную учетную запись и область подключения для запроса
// WebSocket.
//
// @Parameters:
//   - r: Входящий запрос HTTP.
//
// @returns:
//   - result1: Полученный идентификатор.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// conferenceID проверяет и нормализует идентификатор конференции, полученный из маршрута
// запроса.
//
// @Parameters:
//   - c: Контекст запроса и ответа Gin, включая аутентифицированные идентификационные данные.
//
// @returns:
//   - result1: Подтвержденная конференция UUID.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func conferenceID(c *gin.Context) (string, error) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil {
		return "", apperrors.New(apperrors.ErrInvalidInput, "invalid conferenceId")
	}
	return id.String(), nil
}

// После успешной авторизации Ticket выдает одноразовый билет для подключения.
//
// @Parameters:
//   - c: Контекст запроса и ответа Gin, включая аутентифицированные идентификационные данные.
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

// ICE применяет валидированный кандидат ICE к текущему медиасоединению.
//
// @Parameters:
//   - c: Контекст запроса и ответа Gin, включая аутентифицированные идентификационные данные.
func (h *Handler) ICE(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if _, err := h.identity(c.Request); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, h.cfg.TURN.ClientICE(h.cfg.ICE, time.Now()))
}

// origin — Origin проверяет соответствие WebSocket Origin настроенному списку разрешенных источников.
//
// @Parameters:
//   - r: Входящий запрос HTTP.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (h *Handler) origin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	} // Нативные клиенты используют заголовки авторизации Bearer.
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

// Connect открывает настроенную зависимость или запускает авторизованный процесс подключения.
//
// @Parameters:
//   - c: Контекст запроса и ответа Gin, включая аутентифицированные идентификационные данные.
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

// control передает одну управляющую команду WebSocket единственному записывающему устройству
// сокета.
type control struct {
	kind int
	data []byte
}

// client — клиент владеет одним физическим WebSocket, его единственным записывающим устройством,
// ограниченными очередями событий и лимитами входящих сообщений.
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

// newClient создает клиентский сокет с ограниченными очередями и подтвержденным идентификатором
// физического сеанса.
//
// @Parameters:
//   - conn: Подключение, принадлежащее текущей операции.
//   - h: Адаптер обработчика HTTP для регистрации или привязки к клиенту.
//   - s: Состояние сеанса, обработанное в ходе этой операции.
//   - expiry: Срок действия или интервал действительности, определяемый типом.
//
// @returns:
//   - result1: Созданный или загруженный клиент.
func newClient(conn *ws.Conn, h *Handler, s domain.Session, expiry time.Time) *client {
	return &client{conn: conn, handler: h, session: s, expiresAt: expiry, out: make(chan domain.Envelope, h.cfg.QueueSize), low: make(chan domain.Envelope, 8), controls: make(chan control, 8), done: make(chan struct{})}
}

// Offer помещает событие в ограниченную очередь сокета в соответствии с его приоритетом и
// сообщает, было ли оно принято.
//
// @Parameters:
//   - event: Доверенное событие для публикации или обработки.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
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
		return true // При отбрасывании восстанавливаемых событий не должно происходить закрытия
		// медиасоединений.
	}
	select {
	case c.out <- event:
		return true
	default:
		c.Stop("slow_client")
		return false
	}
}

// Stop останавливает активную операцию с веб-сокетами и освобождает принадлежащие ей ресурсы.
//
// @Parameters:
//   - reason: Ограниченный жизненный цикл или причина сбоя.
func (c *client) Stop(reason string) {
	c.once.Do(

		func() { c.reason = reason; close(c.done) })
}

// run — запускает веб-сокеты до тех пор, пока контекст не будет отменён или пока не произойдёт сбой
// при обработке.
func (c *client) run() {
	written := make(chan struct{})
	go func() { defer close(written); c.write() }()
	c.read()
	c.Stop("client_closed")
	// при закрытии фрейма может произойти ожидание тайм-аута записи в сеть.  Не сохраняйте
	// состояние присутствия или медиаданных для физического соединения, которое уже потеряно.
	c.handler.hub.Unregister(c.session)
	<-written
}

// write — при последовательной записи отправляет события сокета и управляющие фреймы, отдавая приоритет
// критически важным данным.
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
			// Когда обе очереди будут готовы, сначала отправьте критически важное событие.
			// Выбранное событие с низким приоритетом может быть отброшено, если его очередь
			// переполнена.
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

// bucket содержит локальные данные о частоте входящих сообщений для одного физического сокета.
type bucket struct {
	tokens      float64
	updated     time.Time
	rate, burst float64
}

// allow проверяет локальный лимит входящих сообщений для каждого сокета.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
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

// authorize позволяет отзывать устойчивые сеансы без привязки медиаданных к таймеру JWT.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// read считывает входящие потоковые события до тех пор, пока не произойдёт отмена или сбой
// терминала, и обеспечивает соблюдение ограничений для очереди назначения.
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
			// Отсчитывайте время с момента получения pong, а не с момента завершения запроса на
			// хранение.
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

// failure — при возникновении ошибки создается ограниченное событие с ошибкой протокола, содержащее
// только безопасный код и подтвержденный корреляционный идентификатор.
//
// @Parameters:
//   - replyTo: Подтвержденный корреляционный идентификатор клиента для ответа об ошибке.
//   - code: Ограниченный публичный статус или код ошибки.
func (c *client) failure(replyTo, code string) {
	event := domain.Event("error", c.session.ConferenceID, map[string]string{"code": code})
	if len(replyTo) <= 36 {
		event.ReplyTo = replyTo
	}
	c.Offer(event)
}

// strictJSON декодирует ограниченный JSON с отбрасыванием неизвестных полей и завершающих
// данных.
//
// @Parameters:
//   - raw: Недоверенное сериализованное значение для декодирования или проверки.
//   - value: Значение для проверки, нормализации или преобразования.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
