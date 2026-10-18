package composite

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// TimelineOrigin выбирает общую точку отсчета медиавремени, используемую для выравнивания
// составных дорожек записи.
//
// @Parameters:
//   - dir: Проверенный локальный каталог.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func TimelineOrigin(dir string) (int64, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "chunk_000000.json"))
	if err != nil {
		return 0, err
	}
	var chunk Chunk
	if json.Unmarshal(raw, &chunk) != nil {
		return 0, fmt.Errorf("invalid timeline manifest")
	}
	return chunk.StartedAtNS, nil
}

// TrackFragment идентифицирует закрытый фрагмент исходной дорожки и его положение на временной
// шкале записи.
type TrackFragment struct {
	TrackInstanceID string `json:"trackInstanceId"`
	ParticipantID   string `json:"participantId"`
	Source          string `json:"source"`
	Kind            string `json:"kind"`
	File            string `json:"file"`
	StartMS         int64  `json:"startMs"`
	EndMS           int64  `json:"endMs"`
}

// TrackManifest описывает отдельные экземпляры дорожек без объединения различных соединений.
// Его источник совпадает с аудиомиксом; в исходных файлах используются форматы Ogg/Opus и
// IVF/VP8.
type TrackManifest struct {
	Version   int             `json:"version"`
	Fragments []TrackFragment `json:"fragments"`
}

// ArchiveTracks считывает записанный архив дорожек в ограниченные метаданные для последующей
// компоновки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - dir: Проверенный локальный каталог.
//   - maximum: Максимально допустимый размер или количество символов.
//
// @returns:
//   - result1: Путь к завершенному локальному архиву дорожек.
//   - result2: Размер архива в байтах.
//   - result3: Шестнадцатеричное представление SHA-256 байтов архива.
//   - result4: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func ArchiveTracks(ctx context.Context, dir string, maximum int64) (string, int64, string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "chunk_*.json"))
	if err != nil {
		return "", 0, "", err
	}
	if len(paths) == 0 || len(paths) > 50000 || maximum <= 0 {
		return "", 0, "", fmt.Errorf("archive limits")
	}
	sort.Strings(paths)
	manifest := TrackManifest{Version: 1, Fragments: []TrackFragment{}}
	files := map[string]bool{}
	var offset float64
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", 0, "", err
		}
		if len(raw) > 256<<10 {
			return "", 0, "", fmt.Errorf("chunk metadata limit")
		}
		var chunk Chunk
		if json.Unmarshal(raw, &chunk) != nil {
			return "", 0, "", fmt.Errorf("invalid chunk metadata")
		}
		for _, source := range chunk.Sources {
			if filepath.Base(source.File) != source.File || source.File == "." || len(manifest.Fragments) >= 100000 {
				return "", 0, "", fmt.Errorf("invalid archive source")
			}
			file := "sources/" + source.File
			files[file] = true
			manifest.Fragments = append(manifest.Fragments, TrackFragment{TrackInstanceID: source.Track.ID, ParticipantID: source.Track.ParticipantID, Source: string(source.Track.Source), Kind: string(source.Track.Kind), File: file, StartMS: int64((offset + source.Offset) * 1000), EndMS: int64((offset + source.End) * 1000)})
		}
		offset += chunk.Duration
	}
	output, err := os.CreateTemp(dir, "tracks-*.zip")
	if err != nil {
		return "", 0, "", err
	}
	keep := false
	defer func() {
		output.Close()
		if !keep {
			os.Remove(output.Name())
		}
	}()
	hash := sha256.New()
	archive := zip.NewWriter(io.MultiWriter(output, hash))
	entry, err := archive.CreateHeader(&zip.FileHeader{Name: "manifest.json", Method: zip.Store})
	if err != nil {
		return "", 0, "", err
	}
	if err = json.NewEncoder(entry).Encode(manifest); err != nil {
		return "", 0, "", err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var total int64
	for _, name := range names {
		if err = ctx.Err(); err != nil {
			return "", 0, "", err
		}
		path := filepath.Join(dir, filepath.FromSlash(name))
		stat, err := os.Lstat(path)
		if err != nil {
			return "", 0, "", err
		}
		total += stat.Size()
		if !stat.Mode().IsRegular() || total > maximum {
			return "", 0, "", fmt.Errorf("archive source size/type limit")
		}
		source, err := os.Open(path)
		if err != nil {
			return "", 0, "", err
		}
		entry, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			source.Close()
			return "", 0, "", err
		}
		n, copyErr := io.Copy(entry, &archiveReader{ctx: ctx, source: io.LimitReader(source, stat.Size()+1)})
		closeErr := source.Close()
		if copyErr != nil || closeErr != nil || n != stat.Size() {
			return "", 0, "", fmt.Errorf("archive source changed")
		}
	}
	if err = archive.Close(); err != nil {
		return "", 0, "", err
	}
	if err = output.Sync(); err != nil {
		return "", 0, "", err
	}
	stat, err := output.Stat()
	if err != nil {
		return "", 0, "", err
	}
	if err = output.Close(); err != nil {
		return "", 0, "", err
	}
	keep = true
	return output.Name(), stat.Size(), hex.EncodeToString(hash.Sum(nil)), nil
}

// archiveReader проверяет отмену операций между ограниченными операциями чтения большого
// исходного файла.
type archiveReader struct {
	ctx    context.Context
	source io.Reader
}

// Read — прочитанные копии имеют размер, не превышающий размер буфера назначения, и извлекаются из
// потока, при этом сохраняются ограничения на количество прочитанных данных и возможность
// отмены операции.
//
// @Parameters:
//   - p: Целевой байтовый буфер заполняется данными из потока.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *archiveReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(p)
}
