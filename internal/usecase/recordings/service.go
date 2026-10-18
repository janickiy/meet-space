package recordings

import (
	"context"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/realtime"
	"github.com/janickiy/meet-space/internal/domain/records"
)

// Repository is the atomic conference recording API boundary. Delivery of its
// durable commands belongs to CommandRepository, not the request service.
type Repository interface {
	// Start starts conference recording lifecycle management and prepares the resources owned by
	// that operation.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Requested recording-segment duration.
	//
	// Returns:
	//   - result 1: The resulting record.
	//   - result 2: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Start(context.Context, string, string, int) (records.Record, bool, error)
	// Stop stops the active conference recording operation and releases its owned resources.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: External recording UUID.
	//
	// Returns:
	//   - result 1: The resulting record.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Stop(context.Context, string, string, string) (records.Record, error)
	// Accessible loads a recording only within the acting account's authorized conference scope.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: External recording UUID.
	//
	// Returns:
	//   - result 1: The resulting record.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Accessible(context.Context, string, string, string) (records.Record, error)
	// List returns a bounded page of conference recording records using the supplied pagination and
	// filters.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Maximum number of results or operations in this batch.
	//   - argument 5: Number of results skipped before the requested page.
	//
	// Returns:
	//   - result 1: The collected record values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	List(context.Context, string, string, int, int) ([]records.Record, error)
	// Delete removes the requested conference recording using the caller's scope and the resource
	// lifecycle rules.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: External recording UUID.
	//   - argument 5: Callback removing the authorized recording's owned storage objects.
	//
	// Returns:
	//   - result 1: The resulting record.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Delete(context.Context, string, string, string, func(context.Context, records.Record) error) (records.Record, error)
}

type Storage interface {
	// RemoveRecording removes only the validated private object prefixes belonging to the selected
	// recording.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Recording and its persisted lifecycle metadata.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	RemoveRecording(context.Context, records.Record) error
}

// Reader loads recording cards after the repository has checked access.
type Reader interface {
	// ReadComposite loads one recording and its artifacts.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting record card.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ReadComposite(context.Context, string) (records.RecordCard, error)
}

// Events publishes recording changes to conference participants.
type Events interface {
	// Broadcast publishes a trusted conference event.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Trusted event to publish or process.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Broadcast(context.Context, realtime.Envelope) error
}

// Service handles authorized recording requests and reads. It owns no polling
// goroutine, queue connection or distributed lock.
type Service struct {
	repo    Repository
	reader  Reader
	events  Events
	storage Storage
}

// NewConferenceService constructs the conference service with the dependencies and limits
// required for conference recording lifecycle management.
//
// Parameters:
//   - repo: Persistence dependency used by the application operation.
//   - reader: Source content stream or recording-card reader, as defined by its type.
//   - events: Destination for committed lifecycle or chat events.
//   - storage: Private object or file storage dependency.
//
// Returns:
//   - result 1: The constructed or loaded service.
func NewConferenceService(repo Repository, reader Reader, events Events, storage Storage) *Service {
	return &Service{repo: repo, reader: reader, events: events, storage: storage}
}

// Delete removes artifacts only after authorization and publishes after commit.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - recordID: External recording UUID.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Delete(ctx context.Context, userID, conferenceID, recordID string) error {
	operation, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	record, err := s.repo.Delete(operation, userID, conferenceID, recordID, s.storage.RemoveRecording)
	if err != nil {
		if operation.Err() == context.DeadlineExceeded {
			return apperrors.New(apperrors.ErrTimeout, "recording deletion timed out")
		}
		return err
	}
	publish(s.events, ctx, record, "recording.deleted")
	return nil
}

// Start commits a recording request; the dispatcher delivers its durable command.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting record card.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Start(ctx context.Context, userID, conferenceID string, request records.ConferenceStartRequest) (records.RecordCard, error) {
	if request.Mode == "" {
		request.Mode = records.ModeComposite
	}
	if !records.ValidConferenceMode(request.Mode) {
		return records.RecordCard{}, apperrors.ErrInvalidInput
	}
	segmentDurationSec := request.SegmentDurationSec
	if segmentDurationSec == 0 {
		segmentDurationSec = 5
	}
	if segmentDurationSec < 2 || segmentDurationSec > 30 {
		return records.RecordCard{}, apperrors.New(apperrors.ErrInvalidInput, "segmentDurationSec must be between 2 and 30")
	}
	var record records.Record
	var created bool
	var err error
	if modes, ok := s.repo.(interface {
		// StartMode starts a recording in the selected validated conference recording mode.
		//
		// Parameters:
		//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
		//   - argument 2: Acting account identifier used to scope access.
		//   - argument 3: Conference identifier limiting the operation scope.
		//   - argument 4: Requested recording-segment duration.
		//   - argument 5: Validated operation or recording mode.
		//
		// Returns:
		//   - result 1: The resulting record.
		//   - result 2: Whether the operation's documented condition holds or its conditional state
		//     change succeeded.
		//   - result 3: nil on success; otherwise the validation, access, transport or persistence
		//     failure.
		StartMode(context.Context, string, string, int, string) (records.Record, bool, error)
	}); ok {
		record, created, err = modes.StartMode(ctx, userID, conferenceID, segmentDurationSec, request.Mode)
	} else if request.Mode == records.ModeComposite {
		record, created, err = s.repo.Start(ctx, userID, conferenceID, segmentDurationSec)
	} else {
		return records.RecordCard{}, apperrors.ErrInvalidInput
	}
	if err != nil {
		return records.RecordCard{}, err
	}
	if created {
		publish(s.events, ctx, record, "recording.starting")
	}
	return s.card(ctx, record.UUID)
}

// Stop requests a stop; the capture worker releases the recording resources.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - recordID: External recording UUID.
//
// Returns:
//   - result 1: The resulting record card.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Stop(ctx context.Context, userID, conferenceID, recordID string) (records.RecordCard, error) {
	record, err := s.repo.Stop(ctx, userID, conferenceID, recordID)
	if err != nil {
		return records.RecordCard{}, err
	}
	if record.Status == records.StatusStopping {
		publish(s.events, ctx, record, "recording.stopping")
	}
	return s.card(ctx, record.UUID)
}

// Read checks access before loading the recording card.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - recordID: External recording UUID.
//
// Returns:
//   - result 1: The resulting record card.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Read(ctx context.Context, userID, conferenceID, recordID string) (records.RecordCard, error) {
	record, err := s.repo.Accessible(ctx, userID, conferenceID, recordID)
	if err != nil {
		return records.RecordCard{}, err
	}
	return s.card(ctx, record.UUID)
}

// List checks access and uses batched reads when the reader supports them.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - limit: Maximum number of results or operations in this batch.
//   - offset: Number of results skipped before the requested page.
//
// Returns:
//   - result 1: The collected record card values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) List(ctx context.Context, userID, conferenceID string, limit, offset int) ([]records.RecordCard, error) {
	rows, err := s.repo.List(ctx, userID, conferenceID, limit, offset)
	if err != nil {
		return nil, err
	}
	if batch, ok := s.reader.(interface {
		// ReadComposites loads authorized composite-recording cards in a bounded batch.
		//
		// Parameters:
		//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
		//   - argument 2: Resource identifiers to process as a bounded batch.
		//
		// Returns:
		//   - result 1: The collected record card values.
		//   - result 2: nil on success; otherwise the validation, access, transport or persistence
		//     failure.
		ReadComposites(context.Context, []string) ([]records.RecordCard, error)
	}); ok {
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.UUID)
		}
		items, err := batch.ReadComposites(ctx, ids)
		if err != nil {
			return nil, err
		}
		for i := range items {
			items[i].Status = records.PublicStatus(items[i].Status)
		}
		return items, nil
	}
	items := make([]records.RecordCard, 0, len(rows))
	for _, record := range rows {
		card, err := s.card(ctx, record.UUID)
		if err != nil {
			return nil, err
		}
		items = append(items, card)
	}
	return items, nil
}

// card maps internal finalization states to the public recording status.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - recordID: External recording UUID.
//
// Returns:
//   - result 1: The resulting record card.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) card(ctx context.Context, recordID string) (records.RecordCard, error) {
	card, err := s.reader.ReadComposite(ctx, recordID)
	card.Status = records.PublicStatus(card.Status)
	return card, err
}

// publish sends a best-effort notification; delivery failure does not undo the request.
//
// Parameters:
//   - events: Destination for committed lifecycle or chat events.
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - record: Recording and its persisted lifecycle metadata.
//   - eventType: Bounded recording lifecycle event category.
func publish(events Events, ctx context.Context, record records.Record, eventType string) {
	if events != nil {
		_ = events.Broadcast(ctx, realtime.Event(eventType, record.ConferenceID, map[string]any{"recordingId": record.UUID, "conferenceId": record.ConferenceID, "status": records.PublicStatus(record.Status), "mode": record.Mode, "requestedBy": record.RequestedBy}))
	}
}
