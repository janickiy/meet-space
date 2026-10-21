// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package websocket

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/realtime"
	"github.com/janickiy/meet-space/internal/operations"
	goredis "github.com/redis/go-redis/v9"
	"net/url"
	"sync"
	"time"
)

type UserBus interface {
	// Subscribe открывает подписку на события с областью видимости, время жизни которой
	// ограничено предоставленным контекстом.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//
	// @returns:
	//   - result1: Созданная или загруженная шина pub sub.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Subscribe(context.Context, string) (*goredis.PubSub, error)
	// Publish публикует сохраненные изменения в веб-сокетах через настроенный транспорт событий
	// или команд.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Доверенное событие для публикации или обработки.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Publish(context.Context, string, domain.Envelope) error
}
type UserPresence interface {
	// Функция Touch обновляет данные о присутствии, используя предоставленное время
	// подтвержденной активности на транспортном уровне.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Текущая интеграция учетной записи или физическое транспортное соединение.
	//
	// @returns:
	//   - result1: Возвращает количество, размер или числовое значение.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Touch(context.Context, string, string) (int64, error)
	// Remove удаляет указанного физического клиента из набора данных о присутствии учетной
	// записи.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Текущая интеграция учетной записи или физическое транспортное соединение.
	//
	// @returns:
	//   - result1: Возвращает количество, размер или числовое значение.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Remove(context.Context, string, string) (int64, error)
}
type AccountChecker interface {
	// Account проверяет, является ли действующий аккаунт зарегистрированным и не является ли он
	// гостевым.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор учетной записи для проверки перед открытием
	//   пользовательского сокета.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Account(context.Context, string) error
}

// UserHandler не зависит от конференц-сессий и Pion. Он разделяет правила для тикетов и
// источников.
type UserHandler struct {
	auth     *Handler
	bus      UserBus
	presence UserPresence
	accounts AccountChecker
	mu       sync.Mutex
	draining bool
	clients  map[*ws.Conn]context.CancelFunc
	reserved int
	wg       sync.WaitGroup
}

// NewUserHandler создает пользовательский обработчик с зависимостями и ограничениями,
// необходимыми для работы с веб-сокетами.
//
// @Parameters:
//   - verifier: Верификатор токена или приватный верификатор PKCE в зависимости от его типа.
//   - tickets: Одноразовый или подписанный эмитент и верификатор медиа-билетов.
//   - limiter: Общий ограничитель частоты запросов.
//   - cfg: Ограничения конфигурации и обработки, контролируемые оператором.
//   - bus: Публикация и подписка с ограничением области видимости.
//   - presence: Распределенный модуль чтения или записи данных о присутствии учетной записи.
//   - accounts: Зависимость от присутствия учетной записи или ее валидации.
//
// @returns:
//   - result1: Созданный или загруженный пользовательский обработчик.
func NewUserHandler(verifier Verifier, tickets Tickets, limiter Limiter, cfg config.RealtimeConfig, bus UserBus, presence UserPresence, accounts AccountChecker) *UserHandler {
	return &UserHandler{auth: NewHandler(nil, verifier, tickets, limiter, cfg), bus: bus, presence: presence, accounts: accounts, clients: map[*ws.Conn]context.CancelFunc{}}
}

// RegisterRoutes регистрирует аутентифицированные конечные точки в режиме реального времени и
// ограничения для их запросов.
//
// @Parameters:
//   - r: Маршрутизатор, на котором зарегистрированы аутентифицированные конечные точки
//     реального времени.
func (h *UserHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/api/v1/ws-ticket", h.Ticket)
	r.GET("/api/v1/ws", h.Connect)
}

// LocalCount возвращает количество локально зарегистрированных сокетов, находящихся под
// блокировкой реестра.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
func (h *UserHandler) LocalCount() int { h.mu.Lock(); defer h.mu.Unlock(); return h.reserved }

// BeginDrain прекращает принимать новые задачи, пока не завершатся текущие операции.
//
// @Parameters:
//   - v: Запрошенное логическое состояние.
func (h *UserHandler) BeginDrain(v bool) {
	h.mu.Lock()
	h.draining = v
	if v {
		for c, cancel := range h.clients {
			cancel()
			_ = c.Close()
		}
	}
	h.mu.Unlock()
}

// Shutdown останавливает выполнение новых задач и ожидает завершения закрытия ресурсов,
// принадлежащих компоненту.
func (h *UserHandler) Shutdown() { h.BeginDrain(true); h.wg.Wait() }

// valid проверяет, что учетная запись, физический клиент и текущий уровень авторизации в
// диалоге по-прежнему позволяют использовать сокет.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *UserHandler) valid(ctx context.Context, id domain.Identity) error {
	if id.AuthSessionID != "" {
		v, ok := h.auth.verifier.(PersistentVerifier)
		if !ok {
			return apperrors.ErrUnauthorized
		}
		if err := v.AuthorizeSession(ctx, id.UserID, id.AuthSessionID); err != nil {
			return err
		}
	} else if !id.ExpiresAt.After(time.Now()) {
		return apperrors.ErrUnauthorized
	}
	return h.accounts.Account(ctx, id.UserID)
}

// После успешной авторизации Ticket выдает одноразовый билет для подключения.
//
// @Parameters:
//   - c: Контекст запроса и ответа Gin, включая аутентифицированные идентификационные данные.
func (h *UserHandler) Ticket(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	id, err := h.auth.identity(c.Request)
	if err == nil {
		err = h.valid(ctx, id)
	}
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if h.auth.limiter != nil {
		v, err := h.auth.limiter.Allow(ctx, h.auth.cfg.Namespace+":user-ticket:"+id.UserID, 30, time.Minute)
		if err != nil {
			httpresponse.Fail(c, apperrors.ErrUnavailable)
			return
		}
		if !v.Allowed {
			c.Header("Retry-After", "60")
			c.AbortWithStatus(429)
			return
		}
	}
	var random [32]byte
	if _, err = rand.Read(random[:]); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	ticket := base64.RawURLEncoding.EncodeToString(random[:])
	ttl := h.auth.cfg.TicketTTL
	if err = h.auth.tickets.SaveTicket(ctx, ticket, "global", id, ttl); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(201, gin.H{"ticket": ticket, "expiresAt": time.Now().UTC().Add(ttl)})
}

// Connect открывает настроенную зависимость или запускает авторизованный процесс подключения.
//
// @Parameters:
//   - c: Контекст запроса и ответа Gin, включая аутентифицированные идентификационные данные.
func (h *UserHandler) Connect(c *gin.Context) {
	h.mu.Lock()
	if h.draining || (h.auth.cfg.MaxConnections > 0 && h.reserved >= h.auth.cfg.MaxConnections) {
		h.mu.Unlock()
		c.AbortWithStatus(503)
		return
	}
	h.reserved++
	h.wg.Add(1)
	h.mu.Unlock()
	handed := false
	defer func() {
		if !handed {
			h.release(nil)
		}
	}()
	if !h.auth.origin(c.Request) {
		httpresponse.Fail(c, apperrors.ErrForbidden)
		return
	}
	q, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil || len(c.Request.URL.RawQuery) > 512 || len(q) != 1 || len(q["ticket"]) != 1 || len(q.Get("ticket")) != 43 || c.GetHeader("Authorization") != "" {
		httpresponse.Fail(c, apperrors.ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	id, err := h.auth.tickets.ConsumeTicket(ctx, q.Get("ticket"), "global")
	if err == nil {
		err = h.valid(ctx, id)
	}
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	sub, err := h.bus.Subscribe(ctx, id.UserID)
	if err != nil {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	conn, err := (&ws.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 4096, HandshakeTimeout: 5 * time.Second, CheckOrigin: h.auth.origin}).Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		_ = sub.Close()
		return
	}
	lifetime, stop := context.WithCancel(context.Background())
	h.mu.Lock()
	if h.draining {
		h.mu.Unlock()
		stop()
		_ = sub.Close()
		_ = conn.Close()
		return
	}
	h.clients[conn] = stop
	h.mu.Unlock()
	handed = true
	go h.run(lifetime, stop, conn, sub, id)
}

// release освобождает только ресурс веб-сокета, принадлежащий соответствующей операции или
// токену.
//
// @Parameters:
//   - conn: Подключение, принадлежащее текущей операции.
func (h *UserHandler) release(conn *ws.Conn) {
	h.mu.Lock()
	delete(h.clients, conn)
	h.reserved--
	h.mu.Unlock()
	h.wg.Done()
}

// run — запускает веб-сокеты до тех пор, пока контекст не будет отменён или пока не произойдёт сбой
// при обработке.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - cancel: Функция отмены для контекста, принадлежащего компоненту.
//   - conn: Подключение, принадлежащее текущей операции.
//   - sub: Подписка на события Redis, принадлежащая компоненту.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
func (h *UserHandler) run(ctx context.Context, cancel context.CancelFunc, conn *ws.Conn, sub *goredis.PubSub, id domain.Identity) {
	operations.WSActive(1)
	defer operations.WSActive(-1)
	defer h.release(conn)
	connection := uuid.NewString()
	cfg := h.auth.cfg
	queue := make(chan []byte, cfg.QueueSize)
	pong := make(chan struct{}, 1)
	var children sync.WaitGroup
	defer func() {
		cancel()
		_ = conn.Close()
		_ = sub.Close()
		children.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		n, err := h.presence.Remove(cleanup, id.UserID, connection)
		if err == nil {
			_ = h.bus.Publish(cleanup, id.UserID, domain.Event("user.presence", "", gin.H{"userId": id.UserID, "online": n > 0, "connections": n}))
		}
	}()
	op, stop := context.WithTimeout(ctx, 3*time.Second)
	n, err := h.presence.Touch(op, id.UserID, connection)
	if err == nil {
		err = h.bus.Publish(op, id.UserID, domain.Event("user.presence", "", gin.H{"userId": id.UserID, "online": n > 0, "connections": n}))
	}
	stop()
	if err != nil {
		return
	}
	conn.SetReadLimit(1024)
	_ = conn.SetReadDeadline(time.Now().Add(cfg.PingInterval + cfg.PongTimeout))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(cfg.PingInterval + cfg.PongTimeout))
		select {
		case pong <- struct{}{}:
		default:
		}
		return nil
	})
	conn.SetPingHandler(func(string) error { return nil })
	children.Add(2)
	go func() {
		defer children.Done()
		defer cancel()
		// Это поток только для сервера.  Команды приложения используют авторизованный
		// REST.
		_, _, _ = conn.ReadMessage()

	}()
	go func() {
		defer children.Done()
		defer cancel()
		for {
			msg, err := sub.ReceiveMessage(ctx)
			if err != nil {
				return
			}
			data := []byte(msg.Payload)
			if len(data) > cfg.OutboundBytes {
				continue
			}
			var event domain.Envelope
			if json.Unmarshal(data, &event) != nil {
				continue
			}
			select {
			case queue <- data:
			case <-ctx.Done():
				return
			default:
				return /* медленное повторное подключение сокета к достоверной истории */
			}
		}
	}()
	ping := time.NewTicker(cfg.PingInterval)
	defer ping.Stop()
	auth := time.NewTicker(5 * time.Second)
	defer auth.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-queue:
			op, stop := context.WithTimeout(ctx, cfg.WriteTimeout+2*time.Second)
			err := h.deliver(op, id.UserID, data, func(outbound []byte) error {
				_ = conn.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
				return conn.WriteMessage(ws.TextMessage, outbound)
			})
			stop()
			if err != nil {
				return
			}
		case <-ping.C:
			if conn.WriteControl(ws.PingMessage, nil, time.Now().Add(cfg.WriteTimeout)) != nil {
				return
			}
		case <-pong:
			op, stop := context.WithTimeout(ctx, 2*time.Second)
			_, err := h.presence.Touch(op, id.UserID, connection)
			stop()
			if err != nil {
				return
			}
		case <-auth.C:
			op, stop := context.WithTimeout(ctx, 3*time.Second)
			err := h.valid(op, id)
			stop()
			if err != nil {
				return
			}
		}
	}
}
