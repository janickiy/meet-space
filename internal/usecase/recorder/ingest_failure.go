package recorder

import (
	"context"
	"errors"
	"fmt"

	"github.com/janickiy/meet-space/internal/domain/records"
)

// ingestFailureRepository defines the replaceable dependency contract for recording jobs and
// their artifacts.
type ingestFailureRepository interface {
	// FindByUUID loads a recording by its public UUID.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: External recording UUID.
	//
	// Returns:
	//   - result 1: The resulting record.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	FindByUUID(context.Context, string) (records.Record, error)
	// MarkFailed conditionally moves a recording to the failed state without overwriting a newer
	// lifecycle transition.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: External recording UUID.
	//   - argument 3: Underlying error that caused the operation to fail.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	MarkFailed(context.Context, string, error) error
	// AddEvent appends a persistent recording lifecycle diagnostic event.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: External recording UUID.
	//   - argument 3: Bounded recording lifecycle event category.
	//   - argument 4: Media or content source belonging to the current operation.
	//   - argument 5: Bounded diagnostic severity.
	//   - argument 6: Persisted or outgoing chat message.
	//   - argument 7: Identity of the worker owning this operation.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	AddEvent(context.Context, string, string, string, string, string, string) error
}

// conferenceReleaser defines the replaceable dependency contract for recording jobs and their
// artifacts.
type conferenceReleaser interface {
	// Release releases only the recording job resource owned by the matching operation or token.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//   - argument 3: External recording UUID.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Release(context.Context, string, string) error
}

// FailIngest records an ingestion failure and releases only the recording's own resources and
// lock.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - repository: Persistence dependency used by the application operation.
//   - locker: Ownership-aware distributed conference recording lock.
//   - recordID: External recording UUID.
//   - workerID: Identity of the worker owning this operation.
//   - cause: Underlying error that caused the operation to fail.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func FailIngest(ctx context.Context, repository ingestFailureRepository, locker conferenceReleaser, recordID, workerID string, cause error) error {
	return failRecord(ctx, repository, locker, recordID, workerID, "record.ingest.failed", cause)
}

// failRecord records a recording failure only if its lifecycle state is still current.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - repository: Persistence dependency used by the application operation.
//   - locker: Ownership-aware distributed conference recording lock.
//   - recordID: External recording UUID.
//   - workerID: Identity of the worker owning this operation.
//   - eventType: Bounded recording lifecycle event category.
//   - cause: Underlying error that caused the operation to fail.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func failRecord(ctx context.Context, repository ingestFailureRepository, locker conferenceReleaser, recordID, workerID, eventType string, cause error) error {
	record, err := repository.FindByUUID(ctx, recordID)
	if err != nil {
		return fmt.Errorf("find failed record: %w", err)
	}
	if records.IsTerminalStatus(record.Status) {
		return releaseRecordLock(ctx, locker, record)
	}
	if err := repository.MarkFailed(ctx, recordID, cause); err != nil {
		return fmt.Errorf("mark ingest failed: %w", err)
	}
	releaseErr := releaseRecordLock(ctx, locker, record)
	message := "WebRTC ingest failed"
	if cause != nil {
		message = cause.Error()
	}
	eventErr := repository.AddEvent(ctx, recordID, eventType, "worker", "error", message, workerID)
	return errors.Join(releaseErr, eventErr)
}

// releaseRecordLock releases the distributed lock only when it belongs to this recording.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - locker: Ownership-aware distributed conference recording lock.
//   - record: Recording and its persisted lifecycle metadata.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func releaseRecordLock(ctx context.Context, locker conferenceReleaser, record records.Record) error {
	if locker == nil {
		return nil
	}
	return locker.Release(ctx, record.ConferenceID, record.UUID)
}
