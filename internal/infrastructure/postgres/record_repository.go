package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/janickiy/meet-space/internal/domain/records"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RecordRepository persists recording jobs and related artifacts through GORM.
type RecordRepository struct {
	db *gorm.DB
}

// NewRecordRepository constructs the record repository with the dependencies and limits required
// for record.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//
// Returns:
//   - result 1: The constructed or loaded record repository.
func NewRecordRepository(db *gorm.DB) *RecordRepository {
	return &RecordRepository{db: db}
}

// Create creates and persists a new record for the acting account.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - record: Recording and its persisted lifecycle metadata.
//
// Returns:
//   - result 1: The resulting record.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *RecordRepository) Create(ctx context.Context, record records.Record) (records.Record, error) {
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		return records.Record{}, err
	}

	return record, nil
}

// FindByUUID loads a recording by its public UUID.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - uuid: External recording UUID.
//
// Returns:
//   - result 1: The resulting record.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *RecordRepository) FindByUUID(ctx context.Context, uuid string) (records.Record, error) {
	var record records.Record
	if err := r.db.WithContext(ctx).Where("uuid = ?", uuid).First(&record).Error; err != nil {
		return records.Record{}, err
	}

	return record, nil
}

// List returns a bounded page of record records using the supplied pagination and filters.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - limit: Maximum number of results or operations in this batch.
//   - offset: Number of results skipped before the requested page.
//
// Returns:
//   - result 1: The collected record values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *RecordRepository) List(ctx context.Context, limit int, offset int) ([]records.Record, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var result []records.Record
	err := r.db.WithContext(ctx).
		Scopes(availableRecordings).
		Where("mode <> 'composite'").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&result).
		Error
	return result, err
}

// ListDetails loads recordings with their related files, segments and lifecycle events.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - limit: Maximum number of results or operations in this batch.
//   - offset: Number of results skipped before the requested page.
//
// Returns:
//   - result 1: The collected record details values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *RecordRepository) ListDetails(ctx context.Context, limit int, offset int) ([]records.RecordDetails, error) {
	items, err := r.List(ctx, limit, offset)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []records.RecordDetails{}, nil
	}

	recordIDs := make([]int64, 0, len(items))
	for _, item := range items {
		recordIDs = append(recordIDs, item.ID)
	}
	files, segments, events, err := r.relatedByRecordIDs(ctx, recordIDs)
	if err != nil {
		return nil, err
	}

	result := make([]records.RecordDetails, 0, len(items))
	for _, item := range items {
		result = append(result, records.RecordDetails{
			Record:   item,
			Files:    files[item.ID],
			Segments: segments[item.ID],
			Events:   events[item.ID],
		})
	}

	return result, nil
}

// ListSummaryDetailsByConferenceIDs loads recording summary artifacts for a batch of conference
// identifiers.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceIDs: Conference identifiers whose recording summaries should be loaded.
//   - status: Lifecycle state or response status to record.
//
// Returns:
//   - result 1: The collected record details values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *RecordRepository) ListSummaryDetailsByConferenceIDs(ctx context.Context, conferenceIDs []string, status string) ([]records.RecordDetails, error) {
	if len(conferenceIDs) == 0 {
		return []records.RecordDetails{}, nil
	}

	query := r.db.WithContext(ctx).
		Scopes(availableRecordings).
		Where("conference_id IN ? AND mode <> 'composite'", conferenceIDs)
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var items []records.Record
	if err := query.Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []records.RecordDetails{}, nil
	}

	recordIDs := make([]int64, 0, len(items))
	for _, item := range items {
		recordIDs = append(recordIDs, item.ID)
	}
	files, err := r.filesByRecordIDs(ctx, recordIDs, records.FileTypeFinalMP4, records.FileTypeFinalAudio, records.FileTypePreviewJPG)
	if err != nil {
		return nil, err
	}

	result := make([]records.RecordDetails, 0, len(items))
	for _, item := range items {
		result = append(result, records.RecordDetails{
			Record: item,
			Files:  files[item.ID],
		})
	}

	return result, nil
}

// FindDetailsByUUID loads details by UUID from the recording repository.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - uuid: External recording UUID.
//
// Returns:
//   - result 1: The resulting record details.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *RecordRepository) FindDetailsByUUID(ctx context.Context, uuid string) (records.RecordDetails, error) {
	var record records.Record
	err := r.db.WithContext(ctx).Scopes(availableRecordings).Where("uuid = ?", uuid).First(&record).Error
	if err != nil {
		return records.RecordDetails{}, err
	}
	files, segments, events, err := r.relatedByRecordIDs(ctx, []int64{record.ID})
	if err != nil {
		return records.RecordDetails{}, err
	}

	return records.RecordDetails{
		Record:   record,
		Files:    files[record.ID],
		Segments: segments[record.ID],
		Events:   events[record.ID],
	}, nil
}

// FindDetailsByUUIDs loads one authorized page in a fixed number of queries.
// Availability is rechecked here, just as in FindDetailsByUUID; callers retain
// responsibility for membership checks. Results follow the requested order.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - ids: Resource identifiers to process as a bounded batch.
//
// Returns:
//   - result 1: The collected record details values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *RecordRepository) FindDetailsByUUIDs(ctx context.Context, ids []string) ([]records.RecordDetails, error) {
	if len(ids) == 0 {
		return []records.RecordDetails{}, nil
	}
	var rows []records.Record
	if err := r.db.WithContext(ctx).Scopes(availableRecordings).Where("uuid IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	byUUID := make(map[string]records.Record, len(rows))
	recordIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		byUUID[row.UUID] = row
		recordIDs = append(recordIDs, row.ID)
	}
	for _, id := range ids {
		if _, ok := byUUID[id]; !ok {
			return nil, gorm.ErrRecordNotFound
		}
	}
	files, segments, events, err := r.relatedByRecordIDs(ctx, recordIDs)
	if err != nil {
		return nil, err
	}
	result := make([]records.RecordDetails, 0, len(ids))
	for _, id := range ids {
		row := byUUID[id]
		result = append(result, records.RecordDetails{Record: row, Files: files[row.ID], Segments: segments[row.ID], Events: events[row.ID]})
	}
	return result, nil
}

// MarkRecording conditionally moves a recording to the recording state without overwriting a
// newer lifecycle transition.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - uuid: External recording UUID.
//   - workerID: Identity of the worker owning this operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *RecordRepository) MarkRecording(ctx context.Context, uuid string, workerID string) error {
	now := time.Now().UTC()
	return r.transition(ctx, uuid, []string{records.StatusStarting}, map[string]any{
		"status":     records.StatusRecording,
		"worker_id":  workerID,
		"started_at": now,
	})
}

// MarkStopping conditionally moves a recording to the stopping state without overwriting a newer
// lifecycle transition.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - uuid: External recording UUID.
//   - reason: Bounded lifecycle or failure reason.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *RecordRepository) MarkStopping(ctx context.Context, uuid string, reason string) error {
	now := time.Now().UTC()
	return r.transition(ctx, uuid, []string{records.StatusStarting, records.StatusRecording, records.StatusDegraded}, map[string]any{
		"status":       records.StatusStopping,
		"stopped_at":   now,
		"ended_reason": reason,
	})
}

// MarkFinalizing conditionally moves a recording to the finalizing state without overwriting a
// newer lifecycle transition.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - uuid: External recording UUID.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *RecordRepository) MarkFinalizing(ctx context.Context, uuid string) error {
	// Retries may observe uploading state: retain local artifacts until the database commit
	// succeeds.
	return r.transition(ctx, uuid, []string{records.StatusStarting, records.StatusRecording, records.StatusDegraded, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}, map[string]any{"status": records.StatusFinalizing})
}

// MarkUploading conditionally moves a recording to the uploading state without overwriting a
// newer lifecycle transition.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - uuid: External recording UUID.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *RecordRepository) MarkUploading(ctx context.Context, uuid string) error {
	return r.transition(ctx, uuid, []string{records.StatusFinalizing, records.StatusUploading}, map[string]any{"status": records.StatusUploading})
}

// MarkFailed conditionally moves a recording to the failed state without overwriting a newer
// lifecycle transition.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - uuid: External recording UUID.
//   - cause: Underlying error that caused the operation to fail.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *RecordRepository) MarkFailed(ctx context.Context, uuid string, cause error) error {
	message := "unknown error"
	if cause != nil {
		message = cause.Error()
	}
	now := time.Now().UTC()
	return r.transition(ctx, uuid, []string{records.StatusStarting, records.StatusRecording, records.StatusDegraded, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}, map[string]any{
		"status":        records.StatusFailed,
		"error_message": message,
		"ended_at":      now,
	})
}

// SaveFinalArtifacts persists the completed recording file, preview and segment metadata after
// processing succeeds.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - uuid: External recording UUID.
//   - finalFile: Final recording-file metadata to persist.
//   - previewFile: Optional preview-image metadata to persist.
//   - segments: Ordered transcript or media segments.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *RecordRepository) SaveFinalArtifacts(ctx context.Context, uuid string, finalFile records.RecordFile, previewFile *records.RecordFile, segments []records.RecordSegment) error {
	return r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			var record records.Record
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ?", uuid).First(&record).Error; err != nil {
				return err
			}
			if record.Status != records.StatusUploading {
				return records.ErrRecordStateChanged
			}

			finalFile.RecordID = record.ID
			if err := upsertRecordFile(tx, &finalFile); err != nil {
				return err
			}
			updates := map[string]any{
				"status":             records.StatusReady,
				"storage_bucket":     finalFile.Bucket,
				"storage_object_key": finalFile.ObjectKey,
				"size_bytes":         finalFile.SizeBytes,
				"duration_sec":       finalFile.DurationSec,
				"ended_at":           time.Now().UTC(),
				"error_message":      nil,
			}

			if previewFile != nil {
				previewFile.RecordID = record.ID
				if err := upsertRecordFile(tx, previewFile); err != nil {
					return err
				}
				updates["preview_object_key"] = previewFile.ObjectKey
			}
			for index := range segments {
				segments[index].RecordID = record.ID
				if err := upsertRecordSegment(tx, &segments[index]); err != nil {
					return err
				}
			}

			return tx.Model(&records.Record{}).Where("id = ?", record.ID).Updates(updates).Error
		})
}

// AddEvent appends a persistent recording lifecycle diagnostic event.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - recordUUID: External recording UUID.
//   - eventType: Bounded recording lifecycle event category.
//   - source: Media or content source belonging to the current operation.
//   - severity: Bounded diagnostic severity.
//   - message: Bounded message text used by this operation; do not include private credentials.
//   - workerID: Identity of the worker owning this operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *RecordRepository) AddEvent(ctx context.Context, recordUUID string, eventType string, source string, severity string, message string, workerID string) error {
	record, err := r.FindByUUID(ctx, recordUUID)
	if err != nil {
		return err
	}
	event := records.RecordEvent{
		RecordID:    record.ID,
		EventType:   eventType,
		EventSource: source,
		Severity:    severity,
		Message:     message,
		WorkerID:    workerID,
	}

	return r.db.WithContext(ctx).Create(&event).Error
}

// transition conditionally changes recording state without overwriting a newer lifecycle
// transition.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - uuid: External recording UUID.
//   - from: Source state of the proposed lifecycle transition.
//   - updates: Validated fields to update in the persisted resource.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *RecordRepository) transition(ctx context.Context, uuid string, from []string, updates map[string]any) error {
	result := r.db.WithContext(ctx).Model(&records.Record{}).
		Where("uuid = ? AND status IN ?", uuid, from).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return records.ErrRecordStateChanged
	}
	return nil
}

// filesByRecordIDs loads recording files in a batch to avoid a query per recording.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - recordIDs: Recording identifiers to load in a bounded batch.
//   - fileTypes: Allowed artifact kinds used to filter the file query.
//
// Returns:
//   - result 1: The resulting record file.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *RecordRepository) filesByRecordIDs(ctx context.Context, recordIDs []int64, fileTypes ...string) (map[int64][]records.RecordFile, error) {
	filesByRecord := make(map[int64][]records.RecordFile, len(recordIDs))
	query := r.db.WithContext(ctx).Where("record_id IN ?", recordIDs)
	if len(fileTypes) > 0 {
		query = query.Where("file_type IN ?", fileTypes)
	}
	var files []records.RecordFile
	if err := query.Order("file_type ASC").Find(&files).Error; err != nil {
		return nil, err
	}
	for _, file := range files {
		filesByRecord[file.RecordID] = append(filesByRecord[file.RecordID], file)
	}
	return filesByRecord, nil
}

// relatedByRecordIDs loads the related recording metadata in a batch.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - recordIDs: Recording identifiers to load in a bounded batch.
//
// Returns:
//   - result 1: The resulting record file.
//   - result 2: The resulting record segment.
//   - result 3: The resulting record event.
//   - result 4: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *RecordRepository) relatedByRecordIDs(ctx context.Context, recordIDs []int64) (map[int64][]records.RecordFile, map[int64][]records.RecordSegment, map[int64][]records.RecordEvent, error) {
	filesByRecord, err := r.filesByRecordIDs(ctx, recordIDs)
	if err != nil {
		return nil, nil, nil, err
	}
	segmentsByRecord := make(map[int64][]records.RecordSegment, len(recordIDs))
	eventsByRecord := make(map[int64][]records.RecordEvent, len(recordIDs))

	var segments []records.RecordSegment
	if err := r.db.WithContext(ctx).
		Where("record_id IN ?", recordIDs).
		Order("seq_no ASC").
		Find(&segments).
		Error; err != nil {
		return nil, nil, nil, err
	}
	for _, segment := range segments {
		segmentsByRecord[segment.RecordID] = append(segmentsByRecord[segment.RecordID], segment)
	}

	var events []records.RecordEvent
	if err := r.db.WithContext(ctx).
		Where("record_id IN ?", recordIDs).
		Order("created_at ASC").
		Find(&events).
		Error; err != nil {
		return nil, nil, nil, err
	}
	for _, event := range events {
		eventsByRecord[event.RecordID] = append(eventsByRecord[event.RecordID], event)
	}

	return filesByRecord, segmentsByRecord, eventsByRecord, nil
}

// upsertRecordFile inserts or updates a recording-file row using its stable artifact identity.
//
// Parameters:
//   - tx: Current database transaction.
//   - file: Validated file or artifact metadata.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func upsertRecordFile(tx *gorm.DB, file *records.RecordFile) error {
	if file.RecordID == 0 {
		return fmt.Errorf("recordID is required for file %s", file.FileType)
	}
	var existing records.RecordFile
	result := tx.Where("record_id = ? AND file_type = ?", file.RecordID, file.FileType).Limit(1).Find(&existing)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return tx.Model(&records.RecordFile{}).
			Where("record_id = ? AND file_type = ?", file.RecordID, file.FileType).
			Updates(map[string]any{
				"bucket":          file.Bucket,
				"object_key":      file.ObjectKey,
				"file_name":       file.FileName,
				"mime_type":       file.MimeType,
				"size_bytes":      file.SizeBytes,
				"duration_sec":    file.DurationSec,
				"checksum_sha256": file.ChecksumSHA256,
				"is_primary":      file.IsPrimary,
				"is_public":       file.IsPublic,
				"metadata_json":   file.MetadataJSON,
			}).Error
	}

	return tx.Create(file).Error
}

// upsertRecordSegment inserts or updates segment metadata using its stable recording identity.
//
// Parameters:
//   - tx: Current database transaction.
//   - segment: Recorded media-segment metadata.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func upsertRecordSegment(tx *gorm.DB, segment *records.RecordSegment) error {
	if segment.RecordID == 0 {
		return fmt.Errorf("recordID is required for segment %d", segment.SeqNo)
	}
	var existing records.RecordSegment
	result := tx.Where("record_id = ? AND seq_no = ?", segment.RecordID, segment.SeqNo).Limit(1).Find(&existing)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return tx.Model(&records.RecordSegment{}).
			Where("record_id = ? AND seq_no = ?", segment.RecordID, segment.SeqNo).
			Updates(map[string]any{
				"status":          segment.Status,
				"local_path":      segment.LocalPath,
				"object_key":      segment.ObjectKey,
				"file_name":       segment.FileName,
				"mime_type":       segment.MimeType,
				"started_at":      segment.StartedAt,
				"ended_at":        segment.EndedAt,
				"duration_ms":     segment.DurationMS,
				"size_bytes":      segment.SizeBytes,
				"checksum_sha256": segment.ChecksumSHA256,
			}).Error
	}

	return tx.Create(segment).Error
}
