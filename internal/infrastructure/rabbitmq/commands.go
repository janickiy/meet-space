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

// Options contains broker dependencies, timeouts and component construction settings.
type Options struct {
	URL         string
	Exchange    string
	Queue       string
	RoutingKey  string
	ConsumerTag string
	Logger      *log.Logger
}

// CommandHandler processes one recording command delivered by RabbitMQ.
type CommandHandler func(context.Context, records.Command) error

// Publisher holds the publisher data used by recording command delivery.
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

// Consumer holds the consumer data used by recording command delivery.
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

// BeginDrain stops accepting new work while existing operations finish.
func (c *Consumer) BeginDrain() { c.drainMu.Lock(); c.draining = true; c.drainMu.Unlock() }

// Active returns the count of component-owned operations still requiring completion before
// shutdown.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
func (c *Consumer) Active() int { c.drainMu.Lock(); defer c.drainMu.Unlock(); return c.active }

// NewPublisher constructs the publisher with the dependencies and limits required for recording
// command delivery.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - options: Validated component construction or execution options.
//
// Returns:
//   - result 1: The constructed or loaded publisher.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// StartRecord publishes the recording-start command that prepares media ingestion.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - recordID: External recording UUID.
//   - segmentDurationSec: Requested segment duration in seconds.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *Publisher) StartRecord(ctx context.Context, recordID string, segmentDurationSec int) error {
	return p.publish(ctx, records.Command{
		Type:               "record.start",
		RecordID:           recordID,
		SegmentDurationSec: segmentDurationSec,
	})
}

// StopRecord publishes the recording-stop command that triggers finalization and artifact
// publication.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - recordID: External recording UUID.
//   - reason: Bounded lifecycle or failure reason.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *Publisher) StopRecord(ctx context.Context, recordID string, reason string) error {
	return p.publish(ctx, records.Command{
		Type:     "record.stop",
		RecordID: recordID,
		Reason:   reason,
	})
}

// Close closes the component-owned resources and ends their lifecycle.
func (p *Publisher) Close() {
	if p == nil {
		return
	}
	if p.closed.Swap(true) {
		return
	}
	p.alive.Store(false)
	// Closing the socket before taking the publish mutex also interrupts a
	// blocked write; Channel.Close itself waits for the library's write lock.
	closeAMQP(p.current.Load())
	p.mu.Lock()
	defer p.mu.Unlock()
	closeAMQP(p.conn)
}

// publish publishes the saved rabbitmq change through its configured event or command transport.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - command: Validated internal recording or media command.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// publishLocked publishes a command while the publisher connection state is locked.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - body: Input content stream or serialized request body.
//   - commandType: Allowlisted recording command kind.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// Check checks the configured component or dependency without creating user work.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// reconnectLocked restores the publisher channel while serializing connection changes.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// watch observes one broker connection so closure of an obsolete connection cannot invalidate a
// newer publisher.
//
// Parameters:
//   - conn: Connection owned by the current operation.
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

// NewConsumer constructs the consumer with the dependencies and limits required for recording
// command delivery.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - options: Validated component construction or execution options.
//
// Returns:
//   - result 1: The constructed or loaded consumer.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// Consume processes broker recording commands until its context is canceled.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - handler: Callback responsible for processing the scoped operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// Check checks the configured component or dependency without creating user work.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Consumer) Check(ctx context.Context) error {
	if !c.alive.Load() {
		return fmt.Errorf("consumer unavailable")
	}
	return ctx.Err()
}

// reconnect restores the broker connection, QoS and acknowledgement state without reviving a
// closed consumer.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// Close closes the component-owned resources and ends their lifecycle.
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

// handleDelivery decodes an AMQP recording command and acknowledges, retries or quarantines the
// delivery.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - delivery: Broker delivery with its acknowledgement state.
//   - handler: Callback responsible for processing the scoped operation.
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
		// This return is not a job failure and does not quarantine the retried delivery.
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

// acknowledge acknowledges or requeues a broker delivery according to its processing result.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - d: The delivery state processed by this operation.
//   - ack: Whether the broker delivery should be acknowledged rather than requeued.
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

// quarantine routes a rejected message to the quarantine path instead of repeatedly processing
// invalid input.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - d: The delivery state processed by this operation.
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

// logf records a bounded diagnostic without including private payloads or credentials.
//
// Parameters:
//   - format: Diagnostic message format; sensitive values must not be included.
//   - args: Command-line, process or protocol arguments.
func (c *Consumer) logf(format string, args ...any) {
	if c.logger != nil {
		c.logger.Printf(format, args...)
	}
}

// openDeclaredChannel opens an AMQP channel and declares the required recording topology.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - options: Validated component construction or execution options.
//
// Returns:
//   - result 1: The constructed or loaded connection.
//   - result 2: The constructed or loaded channel.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// cancelConnectionIO interrupts broker I/O when the owning operation is canceled.
//
// amqp091-go checks publish contexts before writing but cannot interrupt a
// socket write. Cancellation closes this uncertain connection with an I/O
// deadline, releasing both the write and the library's shutdown locks.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conn: Connection owned by the current operation.
//
// Returns:
//   - result 1: The callback used to complete or release the operation.
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

// closeAMQP closes the owned AMQP channel and connection.
//
// Parameters:
//   - conn: Connection owned by the current operation.
func closeAMQP(conn *amqp.Connection) {
	if conn != nil {
		_ = conn.CloseDeadline(time.Now().Add(time.Second))
	}
}

// boundedAMQPRPC runs an AMQP setup operation with a bounded cancellation window.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conn: Connection owned by the current operation.
//   - call: Persisted direct-call lifecycle state.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// declareTopology declares the recording exchange, queues and bindings required by the
// component.
//
// Parameters:
//   - channel: Broker or in-process channel owned by this component.
//   - options: Validated component construction or execution options.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// dial opens a broker connection using the configured connection and cancellation limits.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - url: Validated endpoint or public resource URL.
//
// Returns:
//   - result 1: The constructed or loaded connection.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// normalizeOptions fills and bounds AMQP connection, retry and consumer options.
//
// Parameters:
//   - options: Validated component construction or execution options.
//
// Returns:
//   - result 1: The resulting options.
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
		options.ConsumerTag = "go-recorder-worker"
	}

	return options
}
