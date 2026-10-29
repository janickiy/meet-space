// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

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

// cardRepository считывает метаданные записи после того, как приложение конференции авторизует
// доступ.
type cardRepository interface {
	// FindDetailsByUUID загружает сведения по UUID из хранилища записей.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Внешняя запись UUID.
	//
	// @returns:
	//   - result1: Полученные сведения о записях.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	FindDetailsByUUID(context.Context, string) (records.RecordDetails, error)
}

// workerRepository определяет заменяемый контракт зависимостей для записи заданий и их
// артефактов.
type workerRepository interface {
	ingestFailureRepository
	// MarkFinalizing условно переводит запись в состояние финализации, не перезаписывая более
	// новый переход жизненного цикла.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Внешняя запись UUID.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	MarkFinalizing(context.Context, string) error
	// MarkUploading условно переводит запись в состояние загрузки, не перезаписывая более новый
	// переход жизненного цикла.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Внешняя запись UUID.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	MarkUploading(context.Context, string) error
	// SaveFinalArtifacts после успешной обработки сохраняется завершенный файл записи,
	// предварительный просмотр и метаданные сегмента.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Внешняя запись UUID.
	//   - arg3: Окончательная запись - сохранение метаданных файла.
	//   - arg4: Дополнительный предварительный просмотр - сохранение метаданных
	//   изображения.
	//   - arg5: Упорядоченные сегменты медиаданных.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	SaveFinalArtifacts(context.Context, string, records.RecordFile, *records.RecordFile, []records.RecordSegment) error
}

// mediaIngest определяет заменяемый контракт зависимостей для записи заданий и их артефактов.
type mediaIngest interface {
	// Prepare — подготавливает авторизованное состояние задания по записи до начала согласования
	// медиафайлов.
	//
	// @Parameters:
	//   - arg1: Внешняя запись UUID.
	//   - arg2: Запрошенная продолжительность сегмента в секундах.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Prepare(string, int) error
	// Stop останавливает активное задание по записи и освобождает принадлежащие ему ресурсы.
	//
	// @Parameters:
	//   - arg1: Запись UUID, сеанс приема которой необходимо остановить.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Stop(string) error
	// HandleOffer передает предложение SDP для записи менеджеру приема данных и возвращает
	// согласованный ответ.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Внешняя запись UUID.
	//   - arg3: Валидированный запрос приложения или входящие параметры операции.
	//
	// @returns:
	//   - result1: Итоговый ответ веб-RTC.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	HandleOffer(context.Context, string, records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error)
}

// Service создает карточки для уже авторизованных записей конференций.
type Service struct {
	repository cardRepository
	s3         *s3storage.Client
}

// WorkerService обрабатывает команды записи, входящие медиафайлы, финализацию и загрузку
// артефактов.
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

// SetComposite обновляет составные данные о заданиях по записи и их артефактах.
//
// @Parameters:
//   - service: Служба приложения, вызываемая этим адаптером.
func (s *WorkerService) SetComposite(service *CompositeService) { s.composite = service }

// ValidateLegacyRecord отклоняет устаревшие рабочие операции, нацеленные на защищенную запись
// конференции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// recordCommandLock сериализует команды одной записи и отслеживает активных пользователей
// блокировки.
type recordCommandLock struct {
	mu   sync.Mutex
	refs int
}

// NewService создает сервис с зависимостями и ограничениями, необходимыми для записи заданий и
// их артефактов.
//
// @Parameters:
//   - repository: Зависимость персистентности, используемая операцией приложения.
//   - s3: Частный клиент для хранения записей, совместимый с S3-.
//
// @returns:
//   - result1: Созданный или загруженный сервис.
func NewService(repository cardRepository, s3 *s3storage.Client) *Service {
	return &Service{repository: repository, s3: s3}
}

// NewWorkerService создает рабочий сервис с зависимостями и ограничениями, необходимыми для
// записи заданий и их артефактов.
//
// @Parameters:
//   - repository: Зависимость персистентности, используемая операцией приложения.
//   - postProcessor: Генератор финализации и предварительного просмотра FFmpeg.
//   - ingest: Компонент WebRTC для приема записей.
//   - s3: Частный клиент для хранения записей, совместимый с S3-.
//   - storagePath: Проверенный локальный корень хранилища записей.
//   - workerID: Идентификатор работника, которому принадлежит эта операция.
//   - locker: Блокировка распределенной записи конференции с учетом прав собственности.
//
// @returns:
//   - result1: Созданный или загруженный рабочий сервис.
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

// HandleCommand идемпотентно обрабатывает внутренние команды на запуск или остановку записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - command: Подтвержденная команда внутренней записи или передачи медиаданных.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *WorkerService) HandleCommand(ctx context.Context, command records.Command) error {
	// Команды поступают в рабочий сервис как напрямую, так и через RabbitMQ.  Проверьте
	// recordId, прежде чем включать его в какой-либо путь в файловой системе.
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
		return nil // Более новый переход жизненного цикла уже сделал эту команду устаревшей.
	}
	return commandErr
}

// lockCommand сериализует команды для одной записи и снимает блокировку после выхода из системы
// последнего пользователя.
//
// @Parameters:
//   - recordID: Внешняя запись UUID.
//
// @returns:
//   - result1: Обратный вызов, используемый для завершения операции или освобождения
//     ресурсов.
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

// handleStart идемпотентно подготавливает запись к приему после получения команды на запуск.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - command: Подтвержденная команда внутренней записи или передачи медиаданных.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
		// Rabbit может поместить повторяющуюся ошибку в карантин вместо повторной попытки.
		// Откатите локальное выделение памяти, прежде чем возвращать ошибку, чтобы незапущенная
		// запись не занимала слот для приема данных.
		if s.ingest != nil {
			if stopErr := s.ingest.Stop(command.RecordID); stopErr != nil && !errors.Is(stopErr, records.ErrNoMedia) {
				return errors.Join(err, stopErr)
			}
		}
		return err
	}

	return nil
}

// failWorkerRecord сохраняет информацию о сбое в работе воркера и освобождает ресурсы,
// принадлежащие записи, работа которой была прервана.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - recordID: Внешняя запись UUID.
//   - eventType: Ограниченная категория событий жизненного цикла записи.
//   - cause: Основная ошибка, из-за которой операция завершилась неудачно.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *WorkerService) failWorkerRecord(ctx context.Context, recordID string, eventType string, cause error) error {
	err := failRecord(ctx, s.repository, s.conferenceLocker, recordID, s.workerID, eventType, cause)
	s.cleanupEmptyLocalStorageAfterFailure(ctx, recordID)
	return err
}

// handleStop останавливает прием данных, завершает работу, загружает и сохраняет артефакты
// записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - command: Подтвержденная команда внутренней записи или передачи медиаданных.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *WorkerService) handleStop(ctx context.Context, command records.Command) error {
	record, err := s.repository.FindByUUID(ctx, command.RecordID)
	if err != nil {
		return err
	}
	if records.IsTerminalStatus(record.Status) {
		// Подтверждает повторную доставку без изменения артефактов или конечного состояния
		// жизненного цикла.
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

// recordDir создает валидированный локальный каталог для одного задания по записи.
//
// @Parameters:
//   - recordID: Внешняя запись UUID.
//
// @returns:
//   - result1: Проверенный локальный каталог задания по записи.
func (s *WorkerService) recordDir(recordID string) string {
	return filepath.Join(s.storagePath, "records", recordID)
}

// recordCard создает карточку записи с соответствующими артефактами и временно считывает URLs.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - details: Метаданные записи и связанные артефакты форматируются.
//
// @returns:
//   - result1: Полученная карта записи.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// presignedURL создает временный объект чтения URL для частного артефакта записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - objectKey: Сгенерированный сервером закрытый ключ для хранения объектов.
//
// @returns:
//   - result1: Временный подписанный объект чтения URL для артефакта записи.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) presignedURL(ctx context.Context, objectKey string) (string, error) {
	if s.s3 == nil || objectKey == "" {
		return "", nil
	}

	return s.s3.PresignedGetURL(ctx, objectKey, 24*time.Hour)
}

// segmentMetadata преобразует локальный сегмент медиа в сохраняемые метаданные записи.
//
// @Parameters:
//   - segments: Упорядоченные сегменты медиаданных.
//
// @returns:
//   - result1: Собранные значения сегментов записи.
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

// cleanupLocalStorage удаляет успешно сохраненные локальные артефакты записи после их
// публикации.
//
// @Parameters:
//   - recordID: Внешняя запись UUID.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *WorkerService) cleanupLocalStorage(recordID string) error {
	if err := os.RemoveAll(s.recordDir(recordID)); err != nil {
		return fmt.Errorf("cleanup record storage: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(s.storagePath, "tmp", recordID)); err != nil {
		return fmt.Errorf("cleanup tmp storage: %w", err)
	}

	return nil
}

// cleanupEmptyLocalStorage удаляет пустые каталоги с записями после завершения обработки.
//
// @Parameters:
//   - recordID: Внешняя запись UUID.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *WorkerService) cleanupEmptyLocalStorage(recordID string) error {
	return localstorage.RemoveEmptyTrees(
		s.recordDir(recordID),
		filepath.Join(s.storagePath, "tmp", recordID),
	)
}

// cleanupEmptyLocalStorageAfterFailure удаляет пустые каталоги после сбоя, не удаляя оставшиеся
// диагностические артефакты.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - recordID: Внешняя запись UUID.
func (s *WorkerService) cleanupEmptyLocalStorageAfterFailure(ctx context.Context, recordID string) {
	if err := s.cleanupEmptyLocalStorage(recordID); err != nil {
		_ = s.repository.AddEvent(ctx, recordID, "record.storage.empty_cleanup_failed", "worker", "warning", err.Error(), s.workerID)
	}
}

// HandleOffer передает предложение SDP для записи менеджеру приема данных и возвращает
// согласованный ответ.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - recordID: Внешняя запись UUID.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Итоговый ответ веб-RTC.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
