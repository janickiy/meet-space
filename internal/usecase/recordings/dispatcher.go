package recordings

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/records"
)

// CommandRepository leases commands already committed with the recording state.
type CommandRepository interface {
	// ClaimCommand claims one transactional recording-outbox command together with its current
	// recording state.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: The resulting outbox command.
	//   - result 2: The resulting record.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ClaimCommand(context.Context) (records.OutboxCommand, records.Record, error)
	// CompleteCommand acknowledges a recording-outbox command only while its claim is current.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: The outbox command value required by the contract.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	CompleteCommand(context.Context, records.OutboxCommand) error
	// RetryCommand reschedules the claimed recording command using its bounded retry policy.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: The outbox command value required by the contract.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	RetryCommand(context.Context, records.OutboxCommand) error
}

type Commander interface {
	// StartRecord publishes the recording-start command that prepares media ingestion.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: External recording UUID.
	//   - argument 3: Requested segment duration in seconds.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	StartRecord(context.Context, string, int) error
	// StopRecord publishes the recording-stop command that triggers finalization and artifact
	// publication.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: External recording UUID.
	//   - argument 3: Bounded lifecycle or failure reason.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	StopRecord(context.Context, string, string) error
}

// Locker reserves the recording before delivery. The capture worker owns release.
type Locker interface {
	// Acquire atomically acquires a conference recording lock with the specified recording as its
	// owner.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//   - argument 3: External recording UUID.
	//
	// Returns:
	//   - result 1: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Acquire(context.Context, string, string) (bool, error)
}

// CommandDispatcher owns outbox polling and delivery ordering, independent of
// HTTP reads/authorization. Its caller owns cancellation and shared clients.
type CommandDispatcher struct {
	repo     CommandRepository
	commands Commander
	locker   Locker
	events   Events
}

// NewCommandDispatcher constructs the command dispatcher with the dependencies and limits
// required for conference recording lifecycle management.
//
// Parameters:
//   - repo: Persistence dependency used by the application operation.
//   - commands: Recording command transport.
//   - locker: Ownership-aware distributed conference recording lock.
//   - events: Destination for committed lifecycle or chat events.
//
// Returns:
//   - result 1: The constructed or loaded command dispatcher.
func NewCommandDispatcher(repo CommandRepository, commands Commander, locker Locker, events Events) *CommandDispatcher {
	return &CommandDispatcher{repo: repo, commands: commands, locker: locker, events: events}
}

// Run runs conference recording lifecycle management until the context is canceled or processing
// fails.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
func (d *CommandDispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		for range 16 {
			if err := d.dispatchOne(ctx); err != nil {
				break
			}
		}
	}
}

// dispatchOne keeps the lease, publication, completion and retry in one bounded
// operation. A terminal recording only completes its command: it is not retried
// through delivery, preserving the existing recovery/fencing contract.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (d *CommandDispatcher) dispatchOne(ctx context.Context) error {
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command, record, err := d.repo.ClaimCommand(op)
	if err != nil {
		return err
	}
	if records.IsTerminalStatus(record.Status) {
		return d.repo.CompleteCommand(op, command)
	}
	err = d.deliver(op, command, record)
	if err == nil {
		err = d.repo.CompleteCommand(op, command)
	}
	if err != nil {
		_ = d.repo.RetryCommand(op, command)
		if !errors.Is(err, context.Canceled) {
			slog.Warn("recording command retry", "recording_id", record.UUID, "command", command.CommandType)
		}
	}
	return err
}

// deliver checks the current recipient authorization immediately before the saved event or
// command is delivered.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - command: Validated internal recording or media command.
//   - record: Recording and its persisted lifecycle metadata.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (d *CommandDispatcher) deliver(ctx context.Context, command records.OutboxCommand, record records.Record) error {
	if command.CommandType != "record.start" {
		if err := d.commands.StopRecord(ctx, record.UUID, command.Reason); err != nil {
			return err
		}
		publish(d.events, ctx, record, "recording.stopping")
		return nil
	}
	if d.locker != nil {
		acquired, err := d.locker.Acquire(ctx, record.ConferenceID, record.UUID)
		if err != nil {
			return err
		}
		if !acquired {
			return apperrors.ErrConflict
		}
	}
	if err := d.commands.StartRecord(ctx, record.UUID, record.SegmentDurationSec); err != nil {
		return err
	}
	if record.Status == records.StatusStarting {
		publish(d.events, ctx, record, "recording.starting")
	}
	return nil
}
