package composite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/janickiy/meet-space/internal/operations"

	"github.com/janickiy/meet-space/internal/domain/media"
)

// Source описывает один надежный источник медиаданных, используемый при компоновке.
type Source struct {
	Track  media.Track `json:"track"`
	File   string      `json:"file"`
	Offset float64     `json:"offset"`
	End    float64     `json:"end"`
}

// Chunk — блок становится надежным только после закрытия всех потоков и атомарной публикации его
// манифеста.
type Chunk struct {
	Mode        string   `json:"mode,omitempty"`
	StartedAtNS int64    `json:"startedAtNs,omitempty"`
	Index       int      `json:"index"`
	Duration    float64  `json:"duration"`
	Sources     []Source `json:"sources"`
	Layout      Layout   `json:"layout"`
}

// Composer создает комбинированную запись из независимых источников через FFmpeg вне цикла
// переадресации SFU.
type Composer struct {
	FFmpegPath string
	Width      int
	Height     int
	FPS        int
	Timeout    time.Duration
	slots      chan struct{}
}

// NewComposer создает компоновщик с зависимостями и ограничениями, необходимыми для сборки
// составной записи.
//
// @Parameters:
//   - path: Проверенный путь к локальной файловой системе или объекту.
//   - width: Ширина вывода в пикселях.
//   - height: Высота вывода в пикселях.
//   - fps: Частота кадров видео на выходе.
//   - concurrency: Максимальное количество одновременных операций обработки.
//
// @returns:
//   - result1: Созданный или загруженный компоновщик.
func NewComposer(path string, width, height, fps, concurrency int) *Composer {
	if path == "" {
		path = "ffmpeg"
	}
	if width == 0 {
		width = 1280
	}
	if height == 0 {
		height = 720
	}
	if fps == 0 {
		fps = 30
	}
	if concurrency < 1 {
		concurrency = 1
	}
	return &Composer{FFmpegPath: path, Width: width, Height: height, FPS: fps, Timeout: 2 * time.Minute, slots: make(chan struct{}, concurrency)}
}

// Compose — компоновщик объединяет архивные треки в синхронизированную составную запись.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - dir: Проверенный локальный каталог.
//   - chunk: Ограниченный входной фрагмент и его метаданные для временной шкалы.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Composer) Compose(ctx context.Context, dir string, chunk Chunk) error {
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-ctx.Done():
		return ctx.Err()
	}
	output := filepath.Join(dir, fmt.Sprintf("segment_%06d.mp4", chunk.Index))
	if info, err := os.Stat(output); err == nil && info.Size() > 1024 {
		return nil
	}
	args, err := c.Arguments(dir, chunk, output+".partial.mp4")
	if err != nil {
		return err
	}
	for _, source := range chunk.Sources {
		file, err := os.OpenFile(filepath.Join(dir, "sources", source.File), os.O_RDWR, 0)
		if err != nil {
			return err
		}
		err = file.Sync()
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	workCtx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	cmd := exec.CommandContext(workCtx, c.FFmpegPath, args...)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 3 * time.Second
	operations.FFmpegActive(1)
	defer operations.FFmpegActive(-1)
	var stderr boundedLog
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		operations.Event("ffmpeg_failed")
		return fmt.Errorf("compose segment %d: %w: %s", chunk.Index, err, stderr.String())
	}
	if err := os.Rename(output+".partial.mp4", output); err != nil {
		return err
	}
	return nil
}

// Arguments формирует валидированный список аргументов FFmpeg для запланированного составного
// вывода.
//
// @Parameters:
//   - dir: Проверенный локальный каталог.
//   - chunk: Ограниченный входной фрагмент и его метаданные для временной шкалы.
//   - output: Целевой или сгенерированный результат, необходимый для выполнения этой операции.
//
// @returns:
//   - result1: Собранные строковые значения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Composer) Arguments(dir string, chunk Chunk, output string) ([]string, error) {
	if chunk.Duration <= 0 || chunk.Duration > 31 {
		return nil, fmt.Errorf("invalid composite chunk duration")
	}
	if chunk.Mode == "audio_only" || chunk.Mode == "individual_tracks" {
		return c.audioArguments(dir, chunk, output)
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-filter_complex_threads", "1"}
	tracks := make([]media.Track, 0, len(chunk.Sources))
	index := make(map[string]int, len(chunk.Sources))
	for i, source := range chunk.Sources {
		if filepath.Base(source.File) != source.File || source.File == "." {
			return nil, fmt.Errorf("invalid source file")
		}
		// Браузеры могут изменять размеры VP8 в процессе работы.  При повторной инициализации
		// всего графа фильтров сбрасываются временные метки и состояние наложения, особенно в
		// FFmpeg 6, что может привести к скрытию исправной камеры.  Фильтр масштабирования
		// принимает измененные входные данные, сохраняя фиксированные размеры составного
		// вывода.
		args = append(args, "-threads", "1", "-reinit_filter", "0", "-i", filepath.Join(dir, "sources", source.File))
		tracks = append(tracks, source.Track)
		index[source.Track.ID] = i
	}
	layout := GridLayout(tracks, c.Width, c.Height)
	if chunk.Mode == "screen_focus" && layout.Name == "screen" {
		layout.Tiles = []Tile{{TrackID: layout.Tiles[0].TrackID, Width: layout.Width, Height: layout.Height}}
	}
	duration := strconv.FormatFloat(chunk.Duration, 'f', 6, 64)
	graph := []string{fmt.Sprintf("color=c=0x101827:s=%dx%d:r=%d:d=%s[base]", layout.Width, layout.Height, c.FPS, duration)}
	previous := "base"
	for n, tile := range layout.Tiles {
		i := index[tile.TrackID]
		source := chunk.Sources[i]
		graph = append(graph, fmt.Sprintf("[%d:v]setpts=PTS-STARTPTS+%.6f/TB,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=0x101827,setsar=1[v%d]", i, source.Offset, tile.Width, tile.Height, tile.Width, tile.Height, n))
		// Источники с переменной частотой кадров и статические браузеры сохраняют последний
		// кадр, пока активны.  Source.End поступает только из явного события track.end, поэтому
		// повторная передача кадров не может оживить отключенную камеру или экран.
		graph = append(graph, fmt.Sprintf("[%s][v%d]overlay=%d:%d:eof_action=repeat:repeatlast=1:enable='between(t,%.6f,%.6f)'[mix%d]", previous, n, tile.X, tile.Y, source.Offset, source.End, n))
		previous = fmt.Sprintf("mix%d", n)
	}
	graph = append(graph, fmt.Sprintf("[%s]fps=%d,format=yuv420p,trim=duration=%s,setpts=PTS-STARTPTS[vout]", previous, c.FPS, duration))
	graph = append(graph, fmt.Sprintf("anullsrc=r=48000:cl=stereo,atrim=duration=%s[silence]", duration))
	audioLabels := "[silence]"
	audioCount := 1
	for i, source := range chunk.Sources {
		if source.Track.Kind != media.KindAudio {
			continue
		}
		delay := int(max(0, source.Offset)*1000 + 0.5)
		graph = append(graph, fmt.Sprintf("[%d:a]aresample=48000:async=1:first_pts=0,aformat=sample_fmts=fltp:channel_layouts=stereo,atrim=start=%.6f,asetpts=PTS-STARTPTS,adelay=%d:all=1,apad,atrim=duration=%s[a%d]", i, max(0, -source.Offset), delay, duration, i))
		audioLabels += fmt.Sprintf("[a%d]", i)
		audioCount++
	}
	graph = append(graph, fmt.Sprintf("%samix=inputs=%d:duration=first:dropout_transition=0:normalize=1,alimiter=limit=0.95:latency=1[aout]", audioLabels, audioCount))
	args = append(args, "-filter_complex", strings.Join(graph, ";"), "-map", "[vout]", "-map", "[aout]", "-t", duration, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-threads", "2", "-r", strconv.Itoa(c.FPS), "-g", strconv.Itoa(c.FPS*2), "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2", "-movflags", "+faststart", output)
	return args, nil
}

// Функция Recover восстанавливает пригодные для использования артефакты записи из локально
// заархивированных исходных треков.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - dir: Проверенный локальный каталог.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Composer) Recover(ctx context.Context, dir string) error {
	paths, err := filepath.Glob(filepath.Join(dir, "chunk_*.json"))
	if err != nil {
		return err
	}
	sort.Strings(paths)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var chunk Chunk
		if err := json.Unmarshal(data, &chunk); err != nil {
			return err
		}
		if err := c.Compose(ctx, dir, chunk); err != nil {
			return err
		}
	}
	return nil
}

// boundedLog сохраняет ограниченную диагностику процесса без неограниченного роста потребления
// памяти.
type boundedLog struct {
	mu   sync.Mutex
	data []byte
}

// Функция Write принимает выходные байты через контракт io.Writer, соблюдая правила буферизации
// компонента.
//
// @Parameters:
//   - p: Входные байты, потребляемые этой операцией.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (b *boundedLog) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if len(b.data) > 8192 {
		b.data = b.data[len(b.data)-8192:]
	}
	return len(p), nil
}

// String возвращает буферизованный диагностический текст в виде ограниченной строки.
//
// @returns:
//   - result1: Ограниченный буферизованный диагностический вывод.
func (b *boundedLog) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.data) }
