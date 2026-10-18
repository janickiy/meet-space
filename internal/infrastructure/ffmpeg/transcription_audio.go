package ffmpeg

import (
	"context"
	domain "github.com/janickiy/meet-space/internal/domain/content"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// RecordingObjectReader считывает только приватные сохраненные объекты и никогда не принимает
// внешние URLs.
type RecordingObjectReader interface {
	// OpenRecording открывает приватный аудиопоток или медиапоток для записи, используя
	// сохраненный идентификатор артефакта.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
	//   - arg3: Максимально допустимый размер или количество символов.
	//
	// @returns:
	//   - result1: Результирующий считывающий модуль.
	//   - result2: Возвращает количество, размер или числовое значение.
	//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	OpenRecording(context.Context, string, int64) (io.ReadCloser, int64, error)
}

// TranscriptionAudio извлекает аудио WAV независимо от завершения записи и передачи
// медиаданных.
type TranscriptionAudio struct {
	objects                      RecordingObjectReader
	binary, tempRoot             string
	maxVideoBytes, maxAudioBytes int64
	maxDuration                  int
}

// NewTranscriptionAudio создает аудиозапись расшифровки с учетом зависимостей и ограничений,
// необходимых для записи и постобработки FFmpeg.
//
// @Parameters:
//   - objects: Закрытый объект для чтения записей.
//   - binary: Путь к настроенному внешнему исполняемому файлу.
//   - tempRoot: Корневой каталог, ограничивающий размер временных медиафайлов.
//   - maxVideoBytes: Максимальный размер входного видеопотока в байтах.
//   - maxAudioBytes: Максимальный размер входного аудиопотока в байтах.
//   - maxDuration: Максимальная продолжительность медиафайлов, поддерживаемая этим путем
//     обработки.
//
// @returns:
//   - result1: Созданная или загруженная аудиозапись расшифровки.
func NewTranscriptionAudio(objects RecordingObjectReader, binary, tempRoot string, maxVideoBytes, maxAudioBytes int64, maxDuration int) *TranscriptionAudio {
	return &TranscriptionAudio{objects: objects, binary: binary, tempRoot: tempRoot, maxVideoBytes: maxVideoBytes, maxAudioBytes: maxAudioBytes, maxDuration: maxDuration}
}

// Open открывает доступ к медиафайлам, записям или сеансам, необходимым для работы этого
// компонента.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - source: Медиа или источник контента, связанный с текущей операцией.
//
// @returns:
//   - result1: Полученный аудиофайл.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (a *TranscriptionAudio) Open(ctx context.Context, source domain.RecordingSource) (domain.Audio, error) {
	if a.objects == nil || a.binary == "" || a.maxVideoBytes < 1 || a.maxAudioBytes < 44 || a.maxDuration < 1 || source.DurationSec < 1 || source.DurationSec > a.maxDuration || source.SizeBytes > a.maxVideoBytes {
		return domain.Audio{}, &jobs.Error{Code: "audio_input_limit", Retryable: false}
	}
	root := a.tempRoot
	if root == "" {
		root = os.TempDir()
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return domain.Audio{}, &jobs.Error{Code: "audio_temp_unavailable", Retryable: true}
	}
	dir, err := os.MkdirTemp(root, "go-recorder-stt-")
	if err != nil {
		return domain.Audio{}, &jobs.Error{Code: "audio_temp_unavailable", Retryable: true}
	}
	var once sync.Once
	cleanup := func() { once.Do(func() { _ = os.RemoveAll(dir) }) }
	keep := false
	defer func() {
		if !keep {
			cleanup()
		}
	}()
	input, actual, err := a.objects.OpenRecording(ctx, source.ObjectKey, a.maxVideoBytes)
	if err != nil {
		return domain.Audio{}, &jobs.Error{Code: "audio_storage_unavailable", Retryable: true}
	}
	defer input.Close()
	path := filepath.Join(dir, "input.mp4")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return domain.Audio{}, &jobs.Error{Code: "audio_temp_unavailable", Retryable: true}
	}
	n, err := io.Copy(file, io.LimitReader(input, a.maxVideoBytes+1))
	closeErr := file.Close()
	if err != nil || closeErr != nil || n != actual || n > a.maxVideoBytes {
		return domain.Audio{}, &jobs.Error{Code: "audio_download_failed", Retryable: true}
	}
	output := filepath.Join(dir, "audio.wav")
	_, err = runBounded(ctx, a.binary, "-nostdin", "-hide_banner", "-loglevel", "error", "-protocol_whitelist", "file,pipe", "-i", path, "-map", "0:a:0", "-vn", "-sn", "-dn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", "-t", strconv.Itoa(a.maxDuration), "-fs", strconv.FormatInt(a.maxAudioBytes, 10), output)
	if err != nil {
		return domain.Audio{}, &jobs.Error{Code: "audio_extract_failed", Retryable: ctx.Err() != nil}
	}
	stat, err := os.Stat(output)
	if err != nil || stat.Size() <= 44 || stat.Size() >= a.maxAudioBytes {
		return domain.Audio{}, &jobs.Error{Code: "audio_output_limit", Retryable: false}
	}
	reader, err := os.Open(output)
	if err != nil {
		return domain.Audio{}, &jobs.Error{Code: "audio_temp_unavailable", Retryable: true}
	}
	keep = true
	return domain.Audio{Reader: reader, Size: stat.Size(), ContentType: "audio/wav", Cleanup: func() { _ = reader.Close(); cleanup() }}, nil
}
