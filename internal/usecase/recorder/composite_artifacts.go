package recorder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/janickiy/meet-space/internal/domain/records"
	"github.com/janickiy/meet-space/internal/infrastructure/composite"
	"github.com/janickiy/meet-space/internal/infrastructure/ffmpeg"
	s3storage "github.com/janickiy/meet-space/internal/infrastructure/storage/s3"
)

type compositeArtifactStorage interface {
	// UploadFile загружает локальный артефакт записи и возвращает метаданные сохраненного
	// объекта.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Сгенерированный сервером закрытый ключ для хранения объектов.
	//   - arg3: Проверенный путь к локальной файловой системе или объекту.
	//   - arg4: Объявленный или обнаруженный тип контента MIME.
	//
	// @returns:
	//   - result1: Результирующий загруженный объект.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	UploadFile(context.Context, string, string, string) (s3storage.UploadedObject, error)
	// RemovePrefix удаляет объекты только в пределах валидированного префикса записи,
	// принадлежащего серверу.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Объект или префикс ключа, управляемый сервером.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	RemovePrefix(context.Context, string) error
}

type compositeArtifactRepository interface {
	// SaveCompositeArtifacts коммитит составные артефакты только для той генерации записи,
	// которой по-прежнему владеет работник.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Подписанные учетные данные или токен владения операцией для проверки.
	//   - arg4: Определяет, является ли этот файл последним и нужно ли закрыть архив.
	//   - arg5: Необязательные метаданные файла предварительного просмотра, связанные с
	//   записью.
	//   - arg6: Упорядоченная расшифровка или сегменты медиаданных.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	SaveCompositeArtifacts(context.Context, string, string, records.RecordFile, *records.RecordFile, []records.RecordSegment) error
}

// compositeArtifactPublisher связывает зависимости публикации, а не захвата или аренды.
type compositeArtifactPublisher struct {
	storage    compositeArtifactStorage
	repository compositeArtifactRepository
	maxBytes   int64
}

type compositeArtifactCommand struct {
	record records.Record
	token  string
	dir    string
	result ffmpeg.Result
}

// compositeArtifactPublication владеет префиксом удаленной подготовки одной попытки и ZIP.
// Горутина записи вызывает Publish один раз и откладывает вызов Close до завершения своей
// операции, включая события после коммита и локальную очистку.  Она не является общей.
type compositeArtifactPublication struct {
	publisher compositeArtifactPublisher
	command   compositeArtifactCommand
	base      string
	archive   string
	armed     bool
	committed bool
	closed    bool
}

// begin создает состояние публикации артефакта, привязанное к текущей команде записи.
//
// @Parameters:
//   - command: Подтвержденная команда внутренней записи или передачи медиаданных.
//
// @returns:
//   - result1: Созданная или загруженная публикация составного артефакта.
func (p compositeArtifactPublisher) begin(command compositeArtifactCommand) *compositeArtifactPublication {
	return &compositeArtifactPublication{publisher: p, command: command}
}

// Publish подготавливает неизменяемые артефакты и фиксирует их метаданные в рамках аренды.
// Close отвечает за откат при каждой неудачной попытке коммита; повторных попыток не
// предусмотрено.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *compositeArtifactPublication) Publish(ctx context.Context) error {
	if p.publisher.storage == nil {
		return fmt.Errorf("recording storage is unavailable")
	}
	record, result := p.command.record, p.command.result
	// Прежний владелец не может ни перезаписать, ни очистить файлы другой попытки.
	p.base = filepath.ToSlash(filepath.Join("recordings", record.ConferenceID, record.UUID, "artifacts", p.command.token))
	p.armed = true
	audioOnly := record.Mode == records.ModeAudioOnly || record.Mode == records.ModeIndividualTracks
	mime, fileType := "video/mp4", records.FileTypeFinalMP4
	if audioOnly {
		mime, fileType = "audio/mp4", records.FileTypeFinalAudio
	}
	finalUpload, err := p.publisher.storage.UploadFile(ctx, p.base+"/final.mp4", result.FinalPath, mime)
	if err != nil {
		return fmt.Errorf("upload final recording: %w", err)
	}
	final := records.RecordFile{FileType: fileType, Bucket: finalUpload.Bucket, ObjectKey: finalUpload.ObjectKey, FileName: "final.mp4", MimeType: mime, SizeBytes: &result.FinalSizeBytes, DurationSec: &result.DurationSec, ChecksumSHA256: &result.FinalChecksum, IsPrimary: true}
	if origin, e := composite.TimelineOrigin(p.command.dir); e == nil && origin > 0 {
		final.MetadataJSON, _ = json.Marshal(map[string]any{"timelineOriginNs": origin, "mode": record.Mode})
	}
	var preview *records.RecordFile
	if !audioOnly {
		previewUpload, err := p.publisher.storage.UploadFile(ctx, p.base+"/preview.jpg", result.PreviewPath, "image/jpeg")
		if err != nil {
			return fmt.Errorf("upload recording preview: %w", err)
		}
		preview = &records.RecordFile{FileType: records.FileTypePreviewJPG, Bucket: previewUpload.Bucket, ObjectKey: previewUpload.ObjectKey, FileName: "preview.jpg", MimeType: "image/jpeg", SizeBytes: &result.PreviewSizeBytes, ChecksumSHA256: &result.PreviewChecksum}
	}
	if record.Mode == records.ModeIndividualTracks {
		archive, size, sum, err := composite.ArchiveTracks(ctx, p.command.dir, p.publisher.maxBytes)
		if err != nil {
			return err
		}
		p.archive = archive
		upload, err := p.publisher.storage.UploadFile(ctx, p.base+"/tracks.zip", archive, "application/zip")
		if err != nil {
			return err
		}
		final.Related = []records.RecordFile{{FileType: records.FileTypeTracksArchive, Bucket: upload.Bucket, ObjectKey: upload.ObjectKey, FileName: "tracks.zip", MimeType: "application/zip", SizeBytes: &size, ChecksumSHA256: &sum}}
	}
	if err := p.publisher.repository.SaveCompositeArtifacts(ctx, record.UUID, p.command.token, final, preview, segmentMetadata(result.Segments)); err != nil {
		return err
	}
	p.committed = true
	return nil
}

// Close является идемпотентной и сохраняет прежний порядок отложенной очистки: сначала ZIP,
// затем откат префикса этой попытки с максимальной эффективностью, и только после этого —
// зафиксированный коммит.
func (p *compositeArtifactPublication) Close() {
	if p.closed {
		return
	}
	p.closed = true
	if p.archive != "" {
		_ = os.Remove(p.archive)
	}
	if p.armed && !p.committed {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.publisher.storage.RemovePrefix(cleanup, p.base+"/")
	}
}
