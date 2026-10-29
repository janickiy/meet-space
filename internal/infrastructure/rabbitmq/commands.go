// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/records"
	"github.com/janickiy/meet-space/internal/operations"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Options содержит информацию о зависимостях брокера, таймаутах и параметрах создания
// компонентов.
type Options struct {
	URL         string
	Exchange    string
	Queue       string
	RoutingKey  string
	ConsumerTag string
	Logger      *log.Logger
}

// CommandHandler обрабатывает одну команду записи, полученную от RabbitMQ.
type CommandHandler func(context.Context, records.Command) error

// Publisher хранит данные издателя, используемые при доставке команд записи.
type Publisher struct {
	conn       *amqp.Connection
	channel    *amqp.Channel
	exchange   string
	routingKey string
	options    Options
	mu         sync.Mutex
	alive      atomic.Bool
	closed     atomic.Bool
	current    atomic.Pointer[amqp.Connection]
}

// Consumer хранит данные потребителя, используемые при доставке команд записи.
type Consumer struct {
	drainMu     sync.Mutex
	draining    bool
	active      int
	conn        *amqp.Connection
	channel     *amqp.Channel
	queue       string
	consumerTag string
	logger      *log.Logger
	options     Options
	mu          sync.Mutex
	alive       atomic.Bool
	closed      bool
}

// BeginDrain прекращает принимать новые задачи, пока не завершатся текущие операции.
func (c *Consumer) BeginDrain() { c.drainMu.Lock(); c.draining = true; c.drainMu.Unlock() }

// Active возвращает количество операций, принадлежащих компоненту, которые необходимо завершить
// до завершения работы.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
func (c *Consumer) Active() int { c.drainMu.Lock(); defer c.drainMu.Unlock(); return c.active }

// NewPublisher создает издателя с зависимостями и ограничениями, необходимыми для доставки
// команд записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - options: Проверенные параметры создания или выполнения компонентов.
//
// @returns:
//   - result1: Созданный или загруженный издатель.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NewPublisher(ctx context.Context, options Options) (*Publisher, error) {
	options = normalizeOptions(options)
	conn, channel, err := openDeclaredChannel(ctx, options)
	if err != nil {
		return nil, err
	}
	if err := boundedAMQPRPC(ctx, conn, func() error { return channel.Confirm(false) }); err != nil {
		closeAMQP(conn)
		return nil, fmt.Errorf("rabbitmq publisher confirms: %w", err)
	}

	p := &Publisher{
		conn:       conn,
		channel:    channel,
		exchange:   options.Exchange,
		routingKey: options.RoutingKey,
		options:    options,
	}
	p.watch(conn)
	return p, nil
}

// StartRecord публикует команду начала записи, которая подготавливает прием медиаданных.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - recordID: Внешняя запись UUID.
//   - segmentDurationSec: Запрошенная продолжительность сегмента в секундах.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *Publisher) StartRecord(ctx context.Context, recordID string, segmentDurationSec int) error {
	return p.publish(ctx, records.Command{
		Type:               "record.start",
		RecordID:           recordID,
		SegmentDurationSec: segmentDurationSec,
	})
}

// StopRecord публикует команду остановки записи, которая запускает финализацию и публикацию
// артефакта.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - recordID: Внешняя запись UUID.
//   - reason: Ограниченный жизненный цикл или причина сбоя.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *Publisher) StopRecord(ctx context.Context, recordID string, reason string) error {
	return p.publish(ctx, records.Command{
		Type:     "record.stop",
		RecordID: recordID,
		Reason:   reason,
	})
}

// Close закрывает ресурсы, принадлежащие компоненту, и завершает их жизненный цикл.
func (p *Publisher) Close() {
	if p == nil {
		return
	}
	if p.closed.Swap(true) {
		return
	}
	p.alive.Store(false)
	// Закрытие сокета перед получением мьютекса публикации также прерывает заблокированную
	// запись; сам Channel.Close ожидает снятия блокировки записи библиотеки.
	closeAMQP(p.current.Load())
	p.mu.Lock()
	defer p.mu.Unlock()
	closeAMQP(p.conn)
}

// publish публикует сохраненное изменение rabbitmq через настроенный транспорт событий или
// команд.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - command: Подтвержденная команда внутренней записи или передачи медиаданных.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *Publisher) publish(ctx context.Context, command records.Command) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if p == nil {
		return fmt.Errorf("rabbitmq publisher is not configured")
	}
	body, err := json.Marshal(command)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed.Load() {
		return fmt.Errorf("rabbitmq publisher closed")
	}

	if err := p.publishLocked(ctx, body, command.Type); err == nil {
		return nil
	}
	if err := p.reconnectLocked(ctx); err != nil {
		return err
	}

	return p.publishLocked(ctx, body, command.Type)
}

// publishLocked публикует команду, пока состояние подключения издателя заблокировано.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - body: Поток входного контента или сериализованное тело запроса.
//   - commandType: Тип команды записи, внесенный в список разрешенных.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *Publisher) publishLocked(ctx context.Context, body []byte, commandType string) error {
	if p.channel == nil {
		return fmt.Errorf("rabbitmq channel is not configured")
	}
	stop := cancelConnectionIO(ctx, p.conn)
	defer stop()

	confirmation, err := p.channel.PublishWithDeferredConfirmWithContext(ctx, p.exchange, p.routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now().UTC(),
		Type:         commandType,
		Headers:      amqp.Table{"request_id": operations.ID(ctx)},
		Body:         body,
	})
	if err != nil {
		return err
	}
	if confirmation == nil {
		return fmt.Errorf("rabbitmq publisher confirmation unavailable")
	}
	acked, err := confirmation.WaitContext(ctx)
	if err != nil {
		return err
	}
	if !acked {
		return fmt.Errorf("rabbitmq broker rejected command")
	}
	return nil
}

// Check проверяет настроенный компонент или зависимость без создания пользовательских задач.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *Publisher) Check(ctx context.Context) error {
	if p.closed.Load() {
		return fmt.Errorf("publisher closed")
	}
	if !p.alive.Load() {
		if !p.mu.TryLock() {
			return fmt.Errorf("broker reconnect in progress")
		}
		defer p.mu.Unlock()
		if p.closed.Load() {
			return fmt.Errorf("publisher closed")
		}
		return p.reconnectLocked(ctx)
	}
	return ctx.Err()
}

// reconnectLocked восстанавливает канал издателя при сериализации изменений подключения.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *Publisher) reconnectLocked(ctx context.Context) error {
	closeAMQP(p.conn)
	p.channel, p.conn = nil, nil
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.closed.Load() {
		return fmt.Errorf("rabbitmq publisher closed")
	}
	conn, channel, err := openDeclaredChannel(ctx, p.options)
	if err != nil {
		return err
	}
	if err := boundedAMQPRPC(ctx, conn, func() error { return channel.Confirm(false) }); err != nil {
		closeAMQP(conn)
		return fmt.Errorf("rabbitmq publisher confirms: %w", err)
	}
	if p.closed.Load() {
		closeAMQP(conn)
		return fmt.Errorf("rabbitmq publisher closed")
	}
	p.conn = conn
	p.channel = channel
	p.watch(conn)

	operations.Event("rabbitmq_reconnect")
	return nil
}

// watch отслеживает одно подключение к брокеру, поэтому закрытие устаревшего подключения не
// приведет к аннулированию более нового подключения издателя.
//
// @Parameters:
//   - conn: Подключение, принадлежащее текущей операции.
func (p *Publisher) watch(conn *amqp.Connection) {
	p.current.Store(conn)
	p.alive.Store(true)
	go func() {
		<-conn.NotifyClose(make(chan *amqp.Error, 1))
		p.mu.Lock()
		if p.conn == conn {
			p.alive.Store(false)
		}
		p.mu.Unlock()
	}()
}

// NewConsumer создает потребителя с зависимостями и ограничениями, необходимыми для доставки
// команд записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - options: Проверенные параметры создания или выполнения компонентов.
//
// @returns:
//   - result1: Созданный или загруженный потребитель.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NewConsumer(ctx context.Context, options Options) (*Consumer, error) {
	options = normalizeOptions(options)
	conn, channel, err := openDeclaredChannel(ctx, options)
	if err != nil {
		return nil, err
	}
	if err := boundedAMQPRPC(ctx, conn, func() error {
		if err := channel.Qos(1, 0, false); err != nil {
			return fmt.Errorf("rabbitmq qos: %w", err)
		}
		return channel.Confirm(false)
	}); err != nil {
		closeAMQP(conn)
		return nil, err
	}

	consumer := &Consumer{
		conn:        conn,
		channel:     channel,
		queue:       options.Queue,
		consumerTag: options.ConsumerTag,
		logger:      options.Logger,
		options:     options,
	}
	return consumer, nil
}

// Consume обрабатывает команды записи брокера до тех пор, пока его контекст не будет отменен.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - handler: Обратный вызов, отвечающий за обработку операции с ограниченной областью
//     видимости.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Consumer) Consume(ctx context.Context, handler CommandHandler) error {
	if c == nil {
		return fmt.Errorf("rabbitmq consumer is not configured")
	}
	for ctx.Err() == nil {
		c.mu.Lock()
		channel := c.channel
		conn := c.conn
		closed := c.closed
		c.mu.Unlock()
		if closed {
			return nil
		}
		var err error
		var deliveries <-chan amqp.Delivery
		if channel == nil {
			err = fmt.Errorf("broker disconnected")
		} else {
			err = boundedAMQPRPC(ctx, conn, func() error {
				var callErr error
				deliveries, callErr = channel.Consume(c.queue, c.consumerTag, false, false, false, false, nil)
				return callErr
			})
		}
		if err == nil {
			c.alive.Store(true)
		loop:
			for {
				select {
				case <-ctx.Done():
					c.alive.Store(false)
					return nil
				case delivery, ok := <-deliveries:
					if !ok {
						break loop
					}
					c.handleDelivery(ctx, delivery, handler)
				}
			}
		}
		c.alive.Store(false)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
		if err := c.reconnect(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("broker reconnect pending", "event_type", "rabbitmq.reconnect")
		}
	}
	return nil
}

// Check проверяет настроенный компонент или зависимость без создания пользовательских задач.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Consumer) Check(ctx context.Context) error {
	if !c.alive.Load() {
		return fmt.Errorf("consumer unavailable")
	}
	return ctx.Err()
}

// reconnect — переподключение восстанавливает соединение с брокером, QoS и состояние подтверждения, не
// возвращая к жизни закрытого потребителя.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Consumer) reconnect(ctx context.Context) error {
	conn, ch, err := openDeclaredChannel(ctx, c.options)
	if err != nil {
		return err
	}
	if err = boundedAMQPRPC(ctx, conn, func() error { return errors.Join(ch.Qos(1, 0, false), ch.Confirm(false)) }); err != nil {
		closeAMQP(conn)
		return err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		closeAMQP(conn)
		return context.Canceled
	}
	old := c.conn
	c.conn = conn
	c.channel = ch
	c.mu.Unlock()
	if old != nil {
		closeAMQP(old)
	}
	operations.Event("rabbitmq_reconnect")
	return nil
}

// Close закрывает ресурсы, принадлежащие компоненту, и завершает их жизненный цикл.
func (c *Consumer) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	c.alive.Store(false)
	conn := c.conn
	c.mu.Unlock()
	if conn != nil {
		closeAMQP(conn)
	}
}

// handleDelivery декодирует команду записи AMQP и подтверждает доставку, повторяет попытку или
// помещает её в карантин.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - delivery: Доставка от брокера с указанием состояния подтверждения.
//   - handler: Обратный вызов, отвечающий за обработку операции с ограниченной областью
//     видимости.
func (c *Consumer) handleDelivery(ctx context.Context, delivery amqp.Delivery, handler CommandHandler) {
	started := time.Now()
	defer func() { operations.Observe("command", time.Since(started).Seconds()) }()
	if !delivery.Timestamp.IsZero() {
		operations.State("rabbitmq_last_queue_latency_seconds", time.Since(delivery.Timestamp).Seconds())
	}
	var command records.Command
	if len(delivery.Body) > 65536 || json.Unmarshal(delivery.Body, &command) != nil {
		c.quarantine(ctx, delivery)
		return
	}
	if _, err := uuid.Parse(command.RecordID); err != nil || (command.Type != "record.start" && command.Type != "record.stop") {
		c.quarantine(ctx, delivery)
		return
	}
	c.drainMu.Lock()
	deferred := c.draining && command.Type == "record.start"
	if !deferred {
		c.active++
	}
	c.drainMu.Unlock()
	if deferred {
		// Возврат не означает сбой задания и не помещает повторную доставку в карантин.
		c.acknowledge(ctx, delivery, false)
		select {
		case <-ctx.Done():
		case <-time.After(250 * time.Millisecond):
		}
		return
	}
	defer func() { c.drainMu.Lock(); c.active--; c.drainMu.Unlock() }()
	c.logf("received RabbitMQ command %s for record %s", command.Type, command.RecordID)
	messageCtx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	if id, ok := delivery.Headers["request_id"].(string); ok {
		messageCtx = operations.WithID(messageCtx, id)
	}
	slog.Info("recording command received", "event_type", command.Type, "recording_id", command.RecordID, "request_id", operations.ID(messageCtx), "queue_latency_seconds", time.Since(delivery.Timestamp).Seconds())
	defer cancel()
	if err := handler(messageCtx, command); err != nil {
		c.logf("command %s for record %s failed: %v", command.Type, command.RecordID, err)
		if delivery.Redelivered {
			c.quarantine(ctx, delivery)
			return
		}
		c.acknowledge(ctx, delivery, false)
		return
	}
	c.acknowledge(ctx, delivery, true)
}

// acknowledge — подтверждение или повторная постановка в очередь доставки от брокера в соответствии с
// результатом её обработки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - d: Состояние доставки, обрабатываемое этой операцией.
//   - ack: Указывает, следует ли подтвердить доставку от брокера, а не ставить её в очередь
//     повторно.
func (c *Consumer) acknowledge(ctx context.Context, d amqp.Delivery, ack bool) {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	_ = boundedAMQPRPC(ctx, conn, func() error {
		if ack {
			return d.Ack(false)
		}
		return d.Nack(false, true)
	})
}

// quarantine — карантин перенаправляет отклоненное сообщение в карантин, вместо того чтобы повторно
// обрабатывать неверные входные данные.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - d: Состояние доставки, обрабатываемое этой операцией.
func (c *Consumer) quarantine(ctx context.Context, d amqp.Delivery) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c.mu.Lock()
	ch := c.channel
	conn := c.conn
	c.mu.Unlock()
	if ch == nil {
		c.acknowledge(ctx, d, false)
		return
	}
	stop := cancelConnectionIO(ctx, conn)
	defer stop()
	body := d.Body
	if len(body) > 65536 {
		body = body[:65536]
	}
	confirm, err := ch.PublishWithDeferredConfirmWithContext(ctx, "", c.queue+".failed", false, false, amqp.Publishing{ContentType: d.ContentType, DeliveryMode: amqp.Persistent, Timestamp: time.Now().UTC(), Body: body, Headers: amqp.Table{"failure": "poison_or_repeated_error", "original_bytes": int64(len(d.Body))}})
	if err == nil && confirm != nil {
		var ack bool
		ack, err = confirm.WaitContext(ctx)
		if ack && err == nil {
			c.acknowledge(ctx, d, true)
			operations.Event("rabbitmq_dead_letter")
			slog.Warn("recording command quarantined", "event_type", "rabbitmq.dead_letter")
			return
		}
	}
	c.acknowledge(ctx, d, false)
}

// logf записывает ограниченную диагностическую информацию без включения конфиденциальных данных
// или учетных данных.
//
// @Parameters:
//   - format: Формат диагностического сообщения; конфиденциальные данные не должны включаться.
//   - args: Аргументы командной строки, процесса или протокола.
func (c *Consumer) logf(format string, args ...any) {
	if c.logger != nil {
		c.logger.Printf(format, args...)
	}
}

// openDeclaredChannel открывает канал AMQP и объявляет требуемую топологию записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - options: Проверенные параметры создания или выполнения компонентов.
//
// @returns:
//   - result1: Созданное или загруженное соединение.
//   - result2: Созданный или загруженный канал.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func openDeclaredChannel(ctx context.Context, options Options) (*amqp.Connection, *amqp.Channel, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, err := dial(ctx, options.URL)
	if err != nil {
		return nil, nil, err
	}
	stop := cancelConnectionIO(ctx, conn)
	defer stop()
	channel, err := conn.Channel()
	if err != nil {
		closeAMQP(conn)
		return nil, nil, fmt.Errorf("rabbitmq channel: %w", err)
	}
	if err := declareTopology(channel, options); err != nil {
		closeAMQP(conn)
		return nil, nil, err
	}

	return conn, channel, nil
}

// cancelConnectionIO прерывает операции ввода-вывода брокера при отмене операции, которой он
// принадлежит.
//
// amqp091-go проверяет контексты публикации перед записью, но не может прервать запись в сокет.
// При отмене операции это нестабильное соединение закрывается в соответствии с установленным
// сроком ввода-вывода, освобождая блокировки записи и блокировки завершения работы библиотеки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conn: Подключение, принадлежащее текущей операции.
//
// @returns:
//   - result1: Обратный вызов, используемый для завершения операции или освобождения
//     ресурсов.
func cancelConnectionIO(ctx context.Context, conn *amqp.Connection) func() {
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(done)
		if conn != nil {
			_ = conn.CloseDeadline(time.Now())
		}
	})
	return func() {
		if !stop() {
			<-done
		}
	}
}

// closeAMQP закрывает принадлежащий ему канал AMQP и соединение.
//
// @Parameters:
//   - conn: Подключение, принадлежащее текущей операции.
func closeAMQP(conn *amqp.Connection) {
	if conn != nil {
		_ = conn.CloseDeadline(time.Now().Add(time.Second))
	}
}

// boundedAMQPRPC запускает операцию настройки AMQP с ограниченным окном отмены.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conn: Подключение, принадлежащее текущей операции.
//   - call: Сохраненное состояние жизненного цикла прямого вызова.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func boundedAMQPRPC(ctx context.Context, conn *amqp.Connection, call func() error) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stop := cancelConnectionIO(ctx, conn)
	defer stop()
	if err := ctx.Err(); err != nil {
		return err
	}
	return call()
}

// declareTopology объявляет обмен данными, очереди и привязки, необходимые для работы
// компонента.
//
// @Parameters:
//   - channel: Брокер или внутрипроцессный канал, принадлежащий этому компоненту.
//   - options: Проверенные параметры создания или выполнения компонентов.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func declareTopology(channel *amqp.Channel, options Options) error {
	if err := channel.ExchangeDeclare(options.Exchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq declare exchange: %w", err)
	}
	if _, err := channel.QueueDeclare(options.Queue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq declare queue: %w", err)
	}
	if _, err := channel.QueueDeclare(options.Queue+".failed", true, false, false, false, amqp.Table{"x-message-ttl": int32(7 * 24 * 60 * 60 * 1000), "x-max-length": int32(10000), "x-max-length-bytes": int64(64 << 20)}); err != nil {
		return fmt.Errorf("rabbitmq declare failed queue: %w", err)
	}
	if err := channel.QueueBind(options.Queue, options.RoutingKey, options.Exchange, false, nil); err != nil {
		return fmt.Errorf("rabbitmq bind queue: %w", err)
	}

	return nil
}

// dial устанавливает соединение с брокером, используя настроенные лимиты на количество
// подключений и отмен.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - url: Проверенная конечная точка или общедоступный ресурс URL.
//
// @returns:
//   - result1: Созданное или загруженное соединение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func dial(ctx context.Context, url string) (*amqp.Connection, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	}
	var stopHandshake func()
	defer func() {
		if stopHandshake != nil {
			stopHandshake()
		}
	}()
	conn, err := amqp.DialConfig(url, amqp.Config{Heartbeat: 5 * time.Second, Dial: func(network, address string) (net.Conn, error) {
		d := net.Dialer{Timeout: 5 * time.Second}
		socket, err := d.DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		deadline, _ := ctx.Deadline()
		_ = socket.SetDeadline(deadline)
		done := make(chan struct{})
		stop := context.AfterFunc(ctx, func() {
			defer close(done)
			_ = socket.Close()
		})
		stopHandshake = func() {
			if !stop() {
				<-done
			}
		}
		return socket, nil
	}})
	if err != nil {
		return nil, fmt.Errorf("rabbitmq connect failed")
	}
	if ctx.Err() != nil {
		closeAMQP(conn)
		return nil, ctx.Err()
	}
	return conn, nil
}

// normalizeOptions заполняет и ограничивает параметры подключения, повторных попыток и
// использования AMQP.
//
// @Parameters:
//   - options: Проверенные параметры создания или выполнения компонентов.
//
// @returns:
//   - result1: Результирующие параметры.
func normalizeOptions(options Options) Options {
	options.URL = strings.TrimSpace(options.URL)
	if options.URL == "" {
		options.URL = "amqp://go_recorder:go_recorder_pass@rabbitmq:5672/%2F"
	}
	options.Exchange = strings.TrimSpace(options.Exchange)
	if options.Exchange == "" {
		options.Exchange = "go-recorder.commands"
	}
	options.Queue = strings.TrimSpace(options.Queue)
	if options.Queue == "" {
		options.Queue = "go-recorder.recording.commands"
	}
	options.RoutingKey = strings.TrimSpace(options.RoutingKey)
	if options.RoutingKey == "" {
		options.RoutingKey = "record.commands"
	}
	options.ConsumerTag = strings.TrimSpace(options.ConsumerTag)
	if options.ConsumerTag == "" {
		options.ConsumerTag = "meet-space-record-worker"
	}

	return options
}
