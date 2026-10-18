package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/janickiy/meet-space/internal/domain/records"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RecordRepository сохраняет задания записи и связанные с ними артефакты через GORM.
type RecordRepository struct {
	db *gorm.DB
}

// NewRecordRepository создает репозиторий записей с зависимостями и ограничениями, необходимыми
// для работы с записями.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//
// @returns:
//   - result1: Созданный или загруженный репозиторий записей.
func NewRecordRepository(db *gorm.DB) *RecordRepository {
	return &RecordRepository{db: db}
}

// Create создает и сохраняет новую запись для действующей учетной записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - record: Запись и сохраненные метаданные ее жизненного цикла.
//
// @returns:
//   - result1: Результирующая запись.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *RecordRepository) Create(ctx context.Context, record records.Record) (records.Record, error) {
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		return records.Record{}, err
	}

	return record, nil
}

// FindByUUID загружает запись по ее публичному идентификатору UUID.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - uuid: Внешняя запись UUID.
//
// @returns:
//   - result1: Результирующая запись.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *RecordRepository) FindByUUID(ctx context.Context, uuid string) (records.Record, error) {
	var record records.Record
	if err := r.db.WithContext(ctx).Where("uuid = ?", uuid).First(&record).Error; err != nil {
		return records.Record{}, err
	}

	return record, nil
}

// List возвращает страницу записей с ограниченным количеством записей, используя заданную
// разбивку на страницы и фильтры.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//   - offset: Количество результатов, пропущенных до перехода на запрошенную страницу.
//
// @returns:
//   - result1: Собранные значения записей.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// ListDetails загружает записи с сопутствующими файлами, сегментами и событиями жизненного
// цикла.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//   - offset: Количество результатов, пропущенных до перехода на запрошенную страницу.
//
// @returns:
//   - result1: Собранные сведения о записях.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// ListSummaryDetailsByConferenceIDs загружает сводные записи по артефактам для группы
// идентификаторов конференций.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceIDs: Идентификаторы конференций, по которым необходимо загрузить сводные
//     записи.
//   - status: Состояние жизненного цикла или статус ответа на запись.
//
// @returns:
//   - result1: Собранные сведения о записях.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// FindDetailsByUUID загружает сведения по UUID из хранилища записей.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - uuid: Внешняя запись UUID.
//
// @returns:
//   - result1: Полученные сведения о записях.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// FindDetailsByUUIDs загружает одну авторизованную страницу за фиксированное количество
// запросов.  Здесь повторно проверяется доступность, как и в FindDetailsByUUID; ответственность
// за проверку членства лежит на вызывающей стороне.  Результаты выдаются в запрошенном порядке.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - ids: Идентификаторы ресурсов для обработки в рамках ограниченного пакета.
//
// @returns:
//   - result1: Собранные сведения о записях.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// MarkRecording условно переводит запись в состояние записи, не перезаписывая более новый
// переход жизненного цикла.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - uuid: Внешняя запись UUID.
//   - workerID: Идентификатор работника, которому принадлежит эта операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *RecordRepository) MarkRecording(ctx context.Context, uuid string, workerID string) error {
	now := time.Now().UTC()
	return r.transition(ctx, uuid, []string{records.StatusStarting}, map[string]any{
		"status":     records.StatusRecording,
		"worker_id":  workerID,
		"started_at": now,
	})
}

// MarkStopping условно переводит запись в состояние остановки, не перезаписывая более новый
// переход жизненного цикла.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - uuid: Внешняя запись UUID.
//   - reason: Ограниченный жизненный цикл или причина сбоя.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *RecordRepository) MarkStopping(ctx context.Context, uuid string, reason string) error {
	now := time.Now().UTC()
	return r.transition(ctx, uuid, []string{records.StatusStarting, records.StatusRecording, records.StatusDegraded}, map[string]any{
		"status":       records.StatusStopping,
		"stopped_at":   now,
		"ended_reason": reason,
	})
}

// MarkFinalizing условно переводит запись в состояние финализации, не перезаписывая более новый
// переход жизненного цикла.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - uuid: Внешняя запись UUID.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *RecordRepository) MarkFinalizing(ctx context.Context, uuid string) error {
	// При повторных попытках может отслеживаться состояние загрузки: локальные артефакты
	// сохраняются до успешного подтверждения транзакции в базе данных.
	return r.transition(ctx, uuid, []string{records.StatusStarting, records.StatusRecording, records.StatusDegraded, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}, map[string]any{"status": records.StatusFinalizing})
}

// MarkUploading условно переводит запись в состояние загрузки, не перезаписывая более новый
// переход жизненного цикла.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - uuid: Внешняя запись UUID.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *RecordRepository) MarkUploading(ctx context.Context, uuid string) error {
	return r.transition(ctx, uuid, []string{records.StatusFinalizing, records.StatusUploading}, map[string]any{"status": records.StatusUploading})
}

// MarkFailed автоматически переводит запись в состояние сбоя, не перезаписывая новый переход
// жизненного цикла.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - uuid: Внешняя запись UUID.
//   - cause: Основная ошибка, из-за которой операция завершилась неудачно.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// SaveFinalArtifacts после успешной обработки сохраняется завершенный файл записи,
// предварительный просмотр и метаданные сегмента.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - uuid: Внешняя запись UUID.
//   - finalFile: Окончательная запись - сохранение метаданных файла.
//   - previewFile: Дополнительный предварительный просмотр - сохранение метаданных изображения.
//   - segments: Упорядоченная расшифровка или сегменты медиаданных.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// AddEvent добавляет событие диагностики жизненного цикла постоянной записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - recordUUID: Внешняя запись UUID.
//   - eventType: Ограниченная категория событий жизненного цикла записи.
//   - source: Медиа или источник контента, связанный с текущей операцией.
//   - severity: Ограниченная степень серьезности диагностики.
//   - message: Ограниченный текст сообщения, используемый в этой операции; не указывайте личные
//     учетные данные.
//   - workerID: Идентификатор работника, которому принадлежит эта операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// transition — переход условно изменяет состояние записи без перезаписи более нового перехода жизненного
// цикла.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - uuid: Внешняя запись UUID.
//   - from: Исходное состояние предлагаемого перехода между состояниями жизненного цикла.
//   - updates: Проверенные поля для обновления в сохраняемом ресурсе.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// filesByRecordIDs загружает файлы записей пакетами, чтобы не выполнять запрос для каждой
// записи по отдельности.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - recordIDs: Идентификаторы записей для загрузки в ограниченном пакете.
//   - fileTypes: Допустимые типы артефактов, используемые для фильтрации запроса к файлам.
//
// @returns:
//   - result1: Результирующий файл записи.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// relatedByRecordIDs загружает связанные метаданные записи пакетами.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - recordIDs: Идентификаторы записей для загрузки в ограниченном пакете.
//
// @returns:
//   - result1: Результирующий файл записи.
//   - result2: Результирующий сегмент записи.
//   - result3: Результирующее событие записи.
//   - result4: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// upsertRecordFile вставляет или обновляет строку с файлом записи, используя его стабильный
// идентификатор артефакта.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - file: Проверенные метаданные файла или артефакта.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// upsertRecordSegment вставляет или обновляет метаданные сегмента, используя его стабильный
// идентификатор записи.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - segment: Метаданные записанного медиасегмента.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
