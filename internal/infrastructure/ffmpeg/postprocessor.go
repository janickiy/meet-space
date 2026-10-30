// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package ffmpeg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PostProcessor объединяет сегменты, проверяет выходные данные и создает окончательные
// артефакты записи.
type PostProcessor struct {
	ffmpegPath  string
	ffprobePath string
}

// Result содержит сгенерированные артефакты записи и метаданные их обработки.
type Result struct {
	FinalPath        string
	PreviewPath      string
	Segments         []Segment
	FinalSizeBytes   int64
	PreviewSizeBytes int64
	DurationSec      int
	FinalChecksum    string
	PreviewChecksum  string
}

// Segment содержит проверенные метаданные локальных сегментов записи.
type Segment struct {
	SeqNo          int
	Path           string
	FileName       string
	SizeBytes      int64
	ChecksumSHA256 string
}

// NewPostProcessor создает постпроцессор с зависимостями и ограничениями, необходимыми для
// записи и постобработки FFmpeg.
//
// @Parameters:
//   - ffmpegPath: Путь к настроенному исполняемому файлу FFmpeg.
//
// @returns:
//   - result1: Созданный или загруженный постпроцессор.
func NewPostProcessor(ffmpegPath string) *PostProcessor {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}

	return &PostProcessor{
		ffmpegPath:  ffmpegPath,
		ffprobePath: ffprobePath(ffmpegPath),
	}
}

// FinalizeComposite joins validated SFU recording segments and creates a preview.
// ctx bounds FFmpeg operations; recordDir contains the completed local segments.
// It returns artifact metadata only after output validation succeeds.
func (p *PostProcessor) FinalizeComposite(ctx context.Context, recordDir string) (Result, error) {
	segments, err := findSegments(recordDir)
	if err != nil {
		return Result{}, err
	}
	segments = p.usableSegments(ctx, segments)
	if len(segments) == 0 {
		return Result{}, fmt.Errorf("no non-empty record segments found in %s", recordDir)
	}

	listPath := filepath.Join(recordDir, "concat.txt")
	if err := writeConcatList(listPath, segments); err != nil {
		return Result{}, err
	}

	finalPath := filepath.Join(recordDir, "final.mp4")
	previewPath := filepath.Join(recordDir, "preview.jpg")
	if err := p.concatComposite(ctx, listPath, finalPath); err != nil {
		return Result{}, err
	}
	validated, err := p.ValidateOutput(ctx, finalPath, true)
	if err != nil {
		return Result{}, err
	}
	durationSec := int(validated.Duration + 0.5)
	if err := p.preview(ctx, finalPath, previewPath); err != nil {
		return Result{}, err
	}

	finalInfo, err := os.Stat(finalPath)
	if err != nil {
		return Result{}, fmt.Errorf("stat final video: %w", err)
	}
	previewInfo, err := os.Stat(previewPath)
	if err != nil {
		return Result{}, fmt.Errorf("stat preview: %w", err)
	}
	finalChecksum, err := checksumSHA256(finalPath)
	if err != nil {
		return Result{}, err
	}
	previewChecksum, err := checksumSHA256(previewPath)
	if err != nil {
		return Result{}, err
	}
	segmentInfos, err := segmentInfos(segments)
	if err != nil {
		return Result{}, err
	}

	return Result{
		FinalPath:        finalPath,
		PreviewPath:      previewPath,
		Segments:         segmentInfos,
		FinalSizeBytes:   finalInfo.Size(),
		PreviewSizeBytes: previewInfo.Size(),
		DurationSec:      durationSec,
		FinalChecksum:    finalChecksum,
		PreviewChecksum:  previewChecksum,
	}, nil
}

// Функция preview создает изображение для предварительного просмотра записи, используя
// проверенную продолжительность выходного файла.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - finalPath: Путь к итоговому сгенерированному медиаконтенту.
//   - previewPath: Путь к созданному изображению для предварительного просмотра.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *PostProcessor) preview(ctx context.Context, finalPath string, previewPath string) error {
	if err := p.previewAt(ctx, finalPath, previewPath, "1"); err == nil {
		return nil
	}

	return p.previewAt(ctx, finalPath, previewPath, "0")
}

// previewAt извлекает изображение для предварительного просмотра записи по выбранной
// ограниченной временной метке.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - finalPath: Путь к итоговому сгенерированному медиаконтенту.
//   - previewPath: Путь к созданному изображению для предварительного просмотра.
//   - second: Временная метка, используемая для выбора кадра для предварительного просмотра.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *PostProcessor) previewAt(ctx context.Context, finalPath string, previewPath string, second string) error {
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	args := []string{
		"-hide_banner",
		"-y",
		"-ss", second,
		"-i", finalPath,
		"-frames:v", "1",
		"-q:v", "2",
		previewPath,
	}
	output, err := runBounded(runCtx, p.ffmpegPath, args...)
	if err != nil {
		return fmt.Errorf("create preview: %w; ffmpeg=%s", err, strings.TrimSpace(string(output)))
	}
	if info, err := os.Stat(previewPath); err != nil || info.Size() == 0 {
		return fmt.Errorf("preview frame was not produced at %s seconds", second)
	}

	return nil
}

// durationFloat определяет продолжительность медиафайла с точностью до долей секунды.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - path: Проверенный путь к локальной файловой системе или объекту.
//
// @returns:
//   - result1: Результирующее значение float64.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *PostProcessor) durationFloat(ctx context.Context, path string) (float64, error) {
	runCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	args := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "json",
		path,
	}
	output, err := runBounded(runCtx, p.ffprobePath, args...)
	if err != nil {
		return 0, err
	}
	var payload struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return 0, err
	}
	value, err := strconv.ParseFloat(payload.Format.Duration, 64)
	if err != nil {
		return 0, err
	}

	return value, nil
}

// usableSegments фильтрует локальные сегменты записи, оставляя только те, которые можно
// использовать для финального вывода медиаданных.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - segments: Упорядоченные сегменты медиаданных.
//
// @returns:
//   - result1: Собранные строковые значения.
func (p *PostProcessor) usableSegments(ctx context.Context, segments []string) []string {
	result := make([]string, 0, len(segments))
	for _, segment := range segments {
		duration, err := p.durationFloat(ctx, segment)
		if err != nil || duration <= 0 {
			continue
		}
		result = append(result, segment)
	}

	return result
}

// findSegments находит локальные файлы сегментов, относящиеся к запрошенной записи.
//
// @Parameters:
//   - recordDir: Проверенный локальный каталог задания по записи.
//
// @returns:
//   - result1: Собранные строковые значения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func findSegments(recordDir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(recordDir, "segment_*.*"))
	if err != nil {
		return nil, fmt.Errorf("glob segments: %w", err)
	}
	sort.Strings(matches)

	segments := make([]string, 0, len(matches))
	for _, path := range matches {
		if strings.Contains(filepath.Base(path), ".partial") {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat segment %s: %w", path, err)
		}
		if info.IsDir() || info.Size() == 0 {
			continue
		}
		segments = append(segments, path)
	}

	return segments, nil
}

// segmentInfos формирует упорядоченные метаданные сегментов, используемые при финализации.
//
// @Parameters:
//   - paths: Проверенные локальные пути, задействованные в этой операции.
//
// @returns:
//   - result1: Собранные значения сегментов.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func segmentInfos(paths []string) ([]Segment, error) {
	result := make([]Segment, 0, len(paths))
	for index, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat segment %s: %w", path, err)
		}
		checksum, err := checksumSHA256(path)
		if err != nil {
			return nil, err
		}
		seqNo := index
		if parsed, err := segmentSeqNo(info.Name()); err == nil {
			seqNo = parsed
		}
		result = append(result, Segment{
			SeqNo:          seqNo,
			Path:           path,
			FileName:       info.Name(),
			SizeBytes:      info.Size(),
			ChecksumSHA256: checksum,
		})
	}

	return result, nil
}

// segmentSeqNo извлекает порядковый номер, используемый для упорядочивания сегмента записи.
//
// @Parameters:
//   - fileName: Имя файла сегмента, из которого извлекаются метаданные.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func segmentSeqNo(fileName string) (int, error) {
	trimmed := strings.TrimSuffix(strings.TrimPrefix(fileName, "segment_"), filepath.Ext(fileName))
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("parse segment seq no %q: %w", fileName, err)
	}

	return value, nil
}

// writeConcatList записывает проверенный входной список FFmpeg concat для выбранных сегментов.
//
// @Parameters:
//   - path: Проверенный путь к локальной файловой системе или объекту.
//   - segments: Упорядоченные сегменты медиаданных.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func writeConcatList(path string, segments []string) error {
	var b strings.Builder
	for _, segment := range segments {
		b.WriteString("file '")
		b.WriteString(strings.ReplaceAll(segment, "'", "'\\''"))
		b.WriteString("'\n")
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// checksumSHA256 преобразует байты файла в дайджест SHA-256 для проверки целостности артефакта.
//
// @Parameters:
//   - path: Проверенный путь к локальной файловой системе или объекту.
//
// @returns:
//   - result1: Шестнадцатеричная кодировка SHA-256 байтов файла.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func checksumSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s for checksum: %w", path, err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("checksum %s: %w", path, err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ffprobePath определяет исполняемый файл ffprobe, связанный с настроенным двоичным файлом
// FFmpeg.
//
// @Parameters:
//   - ffmpegPath: Путь к настроенному исполняемому файлу FFmpeg.
//
// @returns:
//   - result1: Определённый путь к настроенному исполняемому файлу ffprobe.
func ffprobePath(ffmpegPath string) string {
	if ffmpegPath == "ffmpeg" {
		return "ffprobe"
	}
	return filepath.Join(filepath.Dir(ffmpegPath), "ffprobe")
}
