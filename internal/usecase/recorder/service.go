package recorder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/records"
	"github.com/janickiy/meet-space/internal/infrastructure/ffmpeg"
	localstorage "github.com/janickiy/meet-space/internal/infrastructure/storage/local"
	s3storage "github.com/janickiy/meet-space/internal/infrastructure/storage/s3"
)

// cardRepository reads recording metadata after the conference application operation authorizes
// access.
type cardRepository interface {
	// FindDetailsByUUID loads details by UUID from the recording repository.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: External recording UUID.
	//
	// Returns:
	//   - result 1: The resulting record details.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	FindDetailsByUUID(context.Context, string) (records.RecordDetails, error)
}

// workerRepository defines the replaceable dependency contract for recording jobs and their
// artifacts.
type workerRepository interface {
	ingestFailureRepository
	// MarkFinalizing conditionally moves a recording to the finalizing state without overwriting a
	// newer lifecycle transition.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: External recording UUID.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	MarkFinalizing(context.Context, string) error
	// MarkUploading conditionally moves a recording to the uploading state without overwriting a
	// newer lifecycle transition.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: External recording UUID.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	MarkUploading(context.Context, string) error
	// SaveFinalArtifacts persists the completed recording file, preview and segment metadata after
	// processing succeeds.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: External recording UUID.
	//   - argument 3: Final recording-file metadata to persist.
	//   - argument 4: Optional preview-image metadata to persist.
	//   - argument 5: Ordered transcript or media segments.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SaveFinalArtifacts(context.Context, string, records.RecordFile, *records.RecordFile, []records.RecordSegment) error
}

// mediaIngest defines the replaceable dependency contract for recording jobs and their
// artifacts.
type mediaIngest interface {
	// Prepare prepares the authorized recording job state before media negotiation begins.
	//
	// Parameters:
	//   - argument 1: External recording UUID.
	//   - argument 2: Requested segment duration in seconds.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Prepare(string, int) error
	// Stop stops the active recording job operation and releases its owned resources.
	//
	// Parameters:
	//   - argument 1: Recording UUID whose ingestion session must be stopped.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Stop(string) error
	// HandleOffer passes a recording SDP offer to the ingestion manager and returns its negotiated
	// answer.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: External recording UUID.
	//   - argument 3: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting web rtc answer response.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	HandleOffer(context.Context, string, records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error)
}

// Service builds cards for already authorized conference recordings.
type Service struct {
	repository cardRepository
	s3         *s3storage.Client
}

// WorkerService handles recording commands, incoming media, finalization and artifact uploads.
type WorkerService struct {
	repository       workerRepository
	postProcessor    *ffmpeg.PostProcessor
	ingest           mediaIngest
	s3               *s3storage.Client
	storagePath      string
	workerID         string
	conferenceLocker conferenceReleaser
	commandsMu       sync.Mutex
	commands         map[string]*recordCommandLock
	composite        *CompositeService
}

// SetComposite updates composite for recording jobs and their artifacts.
//
// Parameters:
//   - service: Application service invoked by this adapter.
func (s *WorkerService) SetComposite(service *CompositeService) { s.composite = service }

// ValidateLegacyRecord rejects legacy worker operations that target a protected conference
// recording.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *WorkerService) ValidateLegacyRecord(ctx context.Context, id string) error {
	record, err := s.repository.FindByUUID(ctx, id)
	if err != nil {
		return err
	}
	if records.IsComposite(record) {
		return records.ErrRecordStateChanged
	}
	return nil
}

// recordCommandLock serializes one recording's commands and tracks active lock users.
type recordCommandLock struct {
	mu   sync.Mutex
	refs int
}

// NewService constructs the service with the dependencies and limits required for recording jobs
// and their artifacts.
//
// Parameters:
//   - repository: Persistence dependency used by the application operation.
//   - s3: Private S3-compatible recording storage client.
//
// Returns:
//   - result 1: The constructed or loaded service.
func NewService(repository cardRepository, s3 *s3storage.Client) *Service {
	return &Service{repository: repository, s3: s3}
}

// NewWorkerService constructs the worker service with the dependencies and limits required for
// recording jobs and their artifacts.
//
// Parameters:
//   - repository: Persistence dependency used by the application operation.
//   - postProcessor: FFmpeg finalization and preview generator.
//   - ingest: WebRTC recording-ingestion component.
//   - s3: Private S3-compatible recording storage client.
//   - storagePath: Validated local recording-storage root.
//   - workerID: Identity of the worker owning this operation.
//   - locker: Ownership-aware distributed conference recording lock.
//
// Returns:
//   - result 1: The constructed or loaded worker service.
func NewWorkerService(repository workerRepository, postProcessor *ffmpeg.PostProcessor, ingest mediaIngest, s3 *s3storage.Client, storagePath string, workerID string, locker conferenceReleaser) *WorkerService {
	return &WorkerService{
		repository:       repository,
		postProcessor:    postProcessor,
		ingest:           ingest,
		s3:               s3,
		storagePath:      storagePath,
		workerID:         workerID,
		conferenceLocker: locker,
	}
}

// HandleCommand idempotently handles an internal recording-start or recording-stop command.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - command: Validated internal recording or media command.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *WorkerService) HandleCommand(ctx context.Context, command records.Command) error {
	// Commands reach the worker both directly and through RabbitMQ. Validate recordId before
	// including it in any filesystem path.
	id, err := uuid.Parse(command.RecordID)
	if err != nil {
		return fmt.Errorf("recordId must be valid UUID: %w", err)
	}
	command.RecordID = id.String()
	if s.composite != nil {
		record, err := s.repository.FindByUUID(ctx, command.RecordID)
		if err != nil {
			return err
		}
		if records.IsComposite(record) {
			return s.composite.HandleCommand(ctx, command, record)
		}
	}
	unlock := s.lockCommand(command.RecordID)
	defer unlock()
	var commandErr error
	switch command.Type {
	case "record.start":
		commandErr = s.handleStart(ctx, command)
	case "record.stop":
		commandErr = s.handleStop(ctx, command)
	default:
		return fmt.Errorf("unknown command type %q", command.Type)
	}
	if errors.Is(commandErr, records.ErrRecordStateChanged) {
		return nil // A newer lifecycle transition has already made this command obsolete.
	}
	return commandErr
}

// lockCommand serializes commands for one recording and removes the lock after its final user
// exits.
//
// Parameters:
//   - recordID: External recording UUID.
//
// Returns:
//   - result 1: The callback used to complete or release the operation.
func (s *WorkerService) lockCommand(recordID string) func() {
	s.commandsMu.Lock()
	if s.commands == nil {
		s.commands = make(map[string]*recordCommandLock)
	}
	lock := s.commands[recordID]
	if lock == nil {
		lock = &recordCommandLock{}
		s.commands[recordID] = lock
	}
	lock.refs++
	s.commandsMu.Unlock()
	lock.mu.Lock()

	return func() {
		lock.mu.Unlock()
		s.commandsMu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(s.commands, recordID)
		}
		s.commandsMu.Unlock()
	}
}

// handleStart idempotently prepares recording ingestion for a delivered start command.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - command: Validated internal recording or media command.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *WorkerService) handleStart(ctx context.Context, command records.Command) error {
	record, err := s.repository.FindByUUID(ctx, command.RecordID)
	if err != nil {
		return err
	}
	if record.Status != records.StatusStarting {
		if records.IsTerminalStatus(record.Status) {
			return releaseRecordLock(ctx, s.conferenceLocker, record)
		}
		return nil
	}
	recordDir := s.recordDir(command.RecordID)
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		return s.failWorkerRecord(ctx, command.RecordID, "record.worker.prepare.failed", err)
	}
	if s.ingest != nil {
		if err := s.ingest.Prepare(command.RecordID, command.SegmentDurationSec); err != nil {
			return s.failWorkerRecord(ctx, command.RecordID, "record.worker.prepare.failed", err)
		}
	}
	if err := s.repository.AddEvent(ctx, command.RecordID, "record.worker.ready", "worker", "info", "worker prepared WebRTC ingest", s.workerID); err != nil {
		// Rabbit may quarantine a repeated error instead of retrying again.
		// Roll back the local allocation before returning that error so an
		// unstarted recording cannot permanently occupy an ingest slot.
		if s.ingest != nil {
			if stopErr := s.ingest.Stop(command.RecordID); stopErr != nil && !errors.Is(stopErr, records.ErrNoMedia) {
				return errors.Join(err, stopErr)
			}
		}
		return err
	}

	return nil
}

// failWorkerRecord stores the worker failure and releases resources owned by the failed
// recording.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - recordID: External recording UUID.
//   - eventType: Bounded recording lifecycle event category.
//   - cause: Underlying error that caused the operation to fail.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *WorkerService) failWorkerRecord(ctx context.Context, recordID string, eventType string, cause error) error {
	err := failRecord(ctx, s.repository, s.conferenceLocker, recordID, s.workerID, eventType, cause)
	s.cleanupEmptyLocalStorageAfterFailure(ctx, recordID)
	return err
}

// handleStop stops ingestion and finalizes, uploads and persists the recording artifacts.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - command: Validated internal recording or media command.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *WorkerService) handleStop(ctx context.Context, command records.Command) error {
	record, err := s.repository.FindByUUID(ctx, command.RecordID)
	if err != nil {
		return err
	}
	if records.IsTerminalStatus(record.Status) {
		// Acknowledge redelivery without modifying artifacts or the final lifecycle state.
		return releaseRecordLock(ctx, s.conferenceLocker, record)
	}
	if s.ingest != nil {
		if err := s.ingest.Stop(command.RecordID); err != nil && !errors.Is(err, records.ErrNoMedia) {
			return s.failWorkerRecord(ctx, command.RecordID, "record.ingest.stop.failed", err)
		}
	}
	if err := releaseRecordLock(ctx, s.conferenceLocker, record); err != nil {
		return err
	}
	if err := s.repository.MarkFinalizing(ctx, command.RecordID); err != nil {
		return err
	}
	result, err := s.postProcessor.Finalize(ctx, s.recordDir(command.RecordID))
	if err != nil {
		return s.failWorkerRecord(ctx, command.RecordID, "record.finalize.failed", err)
	}

	if err := s.repository.MarkUploading(ctx, command.RecordID); err != nil {
		return err
	}
	finalKey := filepath.ToSlash(filepath.Join("records", command.RecordID, "final.mp4"))
	previewKey := filepath.ToSlash(filepath.Join("records", command.RecordID, "preview.jpg"))
	finalUpload, err := s.s3.UploadFile(ctx, finalKey, result.FinalPath, "video/mp4")
	if err != nil {
		return s.failWorkerRecord(ctx, command.RecordID, "record.upload.failed", err)
	}
	previewUpload, err := s.s3.UploadFile(ctx, previewKey, result.PreviewPath, "image/jpeg")
	if err != nil {
		return s.failWorkerRecord(ctx, command.RecordID, "record.upload.failed", err)
	}
	if err := s.s3.RemovePrefix(ctx, filepath.ToSlash(filepath.Join("records", command.RecordID, "segments"))+"/"); err != nil {
		return s.failWorkerRecord(ctx, command.RecordID, "record.segments.cleanup.failed", err)
	}
	segments := segmentMetadata(result.Segments)

	duration := result.DurationSec
	finalSize := result.FinalSizeBytes
	finalChecksum := result.FinalChecksum
	previewSize := result.PreviewSizeBytes
	previewChecksum := result.PreviewChecksum
	previewFile := &records.RecordFile{
		FileType:       records.FileTypePreviewJPG,
		Bucket:         previewUpload.Bucket,
		ObjectKey:      previewUpload.ObjectKey,
		FileName:       "preview.jpg",
		MimeType:       "image/jpeg",
		SizeBytes:      &previewSize,
		ChecksumSHA256: &previewChecksum,
		IsPrimary:      false,
		IsPublic:       false,
	}
	finalFile := records.RecordFile{
		FileType:       records.FileTypeFinalMP4,
		Bucket:         finalUpload.Bucket,
		ObjectKey:      finalUpload.ObjectKey,
		FileName:       "final.mp4",
		MimeType:       "video/mp4",
		SizeBytes:      &finalSize,
		DurationSec:    &duration,
		ChecksumSHA256: &finalChecksum,
		IsPrimary:      true,
		IsPublic:       false,
	}
	if err := s.repository.SaveFinalArtifacts(ctx, command.RecordID, finalFile, previewFile, segments); err != nil {
		return err
	}
	_ = s.repository.AddEvent(ctx, command.RecordID, "record.artifacts.uploaded", "worker", "info", "final.mp4 and preview.jpg uploaded to MinIO", s.workerID)
	if err := s.cleanupLocalStorage(command.RecordID); err != nil {
		_ = s.repository.AddEvent(ctx, command.RecordID, "record.storage.cleanup_failed", "worker", "warning", err.Error(), s.workerID)
		return nil
	}
	_ = s.repository.AddEvent(ctx, command.RecordID, "record.storage.cleaned", "worker", "info", "local storage cleaned after successful MinIO upload", s.workerID)

	return nil
}

// recordDir builds the validated local directory of one recording job.
//
// Parameters:
//   - recordID: External recording UUID.
//
// Returns:
//   - result 1: Validated local directory of the recording job.
func (s *WorkerService) recordDir(recordID string) string {
	return filepath.Join(s.storagePath, "records", recordID)
}

// recordCard builds a recording card with related artifacts and temporary read URLs.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - details: Recording metadata and related artifacts to format.
//
// Returns:
//   - result 1: The resulting record card.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) recordCard(ctx context.Context, details records.RecordDetails) (records.RecordCard, error) {
	card := records.RecordCard{
		Record:   details.Record,
		Files:    make([]records.RecordFileView, 0, len(details.Files)),
		Segments: make([]records.RecordSegmentView, 0, len(details.Segments)),
		Events:   details.Events,
	}
	for _, file := range details.Files {
		view := records.RecordFileView{RecordFile: file}
		url, err := s.presignedURL(ctx, file.ObjectKey)
		if err != nil {
			return records.RecordCard{}, err
		}
		view.URL = url
		card.Files = append(card.Files, view)
	}

	for _, segment := range details.Segments {
		view := records.RecordSegmentView{RecordSegment: segment}
		if segment.ObjectKey != nil {
			if details.Record.StorageBucket != nil {
				view.Bucket = *details.Record.StorageBucket
			} else if s.s3 != nil {
				view.Bucket = s.s3.Bucket()
			}
			url, err := s.presignedURL(ctx, *segment.ObjectKey)
			if err != nil {
				return records.RecordCard{}, err
			}
			view.URL = url
		}
		card.Segments = append(card.Segments, view)
	}

	return card, nil
}

// presignedURL creates a temporary read URL for a private recording artifact.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - objectKey: Server-generated private object-storage key.
//
// Returns:
//   - result 1: Temporary signed read URL for the recording artifact.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) presignedURL(ctx context.Context, objectKey string) (string, error) {
	if s.s3 == nil || objectKey == "" {
		return "", nil
	}

	return s.s3.PresignedGetURL(ctx, objectKey, 24*time.Hour)
}

// segmentMetadata converts a local media segment into persistable recording metadata.
//
// Parameters:
//   - segments: Ordered transcript or media segments.
//
// Returns:
//   - result 1: The collected record segment values.
func segmentMetadata(segments []ffmpeg.Segment) []records.RecordSegment {
	result := make([]records.RecordSegment, 0, len(segments))
	for _, segment := range segments {
		fileName := segment.FileName
		sizeBytes := segment.SizeBytes
		checksum := segment.ChecksumSHA256
		result = append(result, records.RecordSegment{
			SeqNo:          segment.SeqNo,
			Status:         "closed",
			FileName:       &fileName,
			SizeBytes:      &sizeBytes,
			ChecksumSHA256: &checksum,
		})
	}

	return result
}

// cleanupLocalStorage removes successful local recording artifacts after their durable
// publication.
//
// Parameters:
//   - recordID: External recording UUID.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *WorkerService) cleanupLocalStorage(recordID string) error {
	if err := os.RemoveAll(s.recordDir(recordID)); err != nil {
		return fmt.Errorf("cleanup record storage: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(s.storagePath, "tmp", recordID)); err != nil {
		return fmt.Errorf("cleanup tmp storage: %w", err)
	}

	return nil
}

// cleanupEmptyLocalStorage removes empty recording directories after processing completes.
//
// Parameters:
//   - recordID: External recording UUID.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *WorkerService) cleanupEmptyLocalStorage(recordID string) error {
	return localstorage.RemoveEmptyTrees(
		s.recordDir(recordID),
		filepath.Join(s.storagePath, "tmp", recordID),
	)
}

// cleanupEmptyLocalStorageAfterFailure removes empty directories after a failure without
// deleting remaining diagnostic artifacts.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - recordID: External recording UUID.
func (s *WorkerService) cleanupEmptyLocalStorageAfterFailure(ctx context.Context, recordID string) {
	if err := s.cleanupEmptyLocalStorage(recordID); err != nil {
		_ = s.repository.AddEvent(ctx, recordID, "record.storage.empty_cleanup_failed", "worker", "warning", err.Error(), s.workerID)
	}
}

// HandleOffer passes a recording SDP offer to the ingestion manager and returns its negotiated
// answer.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - recordID: External recording UUID.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting web rtc answer response.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *WorkerService) HandleOffer(ctx context.Context, recordID string, request records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error) {
	record, err := s.repository.FindByUUID(ctx, recordID)
	if err != nil {
		return records.WebRTCAnswerResponse{}, err
	}
	if records.IsComposite(record) {
		return records.WebRTCAnswerResponse{}, records.ErrRecordStateChanged
	}
	if s.ingest == nil {
		return records.WebRTCAnswerResponse{}, fmt.Errorf("webrtc ingest is not configured")
	}

	return s.ingest.HandleOffer(ctx, recordID, request)
}
