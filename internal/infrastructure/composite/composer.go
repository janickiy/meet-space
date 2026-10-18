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

// Source describes one durable media source used by composite assembly.
type Source struct {
	Track  media.Track `json:"track"`
	File   string      `json:"file"`
	Offset float64     `json:"offset"`
	End    float64     `json:"end"`
}

// Chunk becomes durable only after every stream closes and its manifest is atomically published.
type Chunk struct {
	Mode        string   `json:"mode,omitempty"`
	StartedAtNS int64    `json:"startedAtNs,omitempty"`
	Index       int      `json:"index"`
	Duration    float64  `json:"duration"`
	Sources     []Source `json:"sources"`
	Layout      Layout   `json:"layout"`
}

// Composer builds the combined recording from independent sources through FFmpeg outside the SFU
// forwarding loop.
type Composer struct {
	FFmpegPath string
	Width      int
	Height     int
	FPS        int
	Timeout    time.Duration
	slots      chan struct{}
}

// NewComposer constructs the composer with the dependencies and limits required for composite
// recording assembly.
//
// Parameters:
//   - path: Validated local filesystem or object path.
//   - width: Output width in pixels.
//   - height: Output height in pixels.
//   - fps: Output video frame rate.
//   - concurrency: Maximum number of simultaneous processing operations.
//
// Returns:
//   - result 1: The constructed or loaded composer.
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

// Compose assembles the archived tracks into a synchronized composite recording.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - dir: Validated local directory.
//   - chunk: Bounded input chunk and its timeline metadata.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// Arguments builds the validated FFmpeg argument list for the planned composite output.
//
// Parameters:
//   - dir: Validated local directory.
//   - chunk: Bounded input chunk and its timeline metadata.
//   - output: Destination or generated result required by this operation.
//
// Returns:
//   - result 1: The collected string values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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
		// Browsers can change VP8 dimensions mid-stream. Reinitializing the entire filter graph resets
		// timestamps and overlay state, especially in FFmpeg 6, and can hide a healthy camera. The scale
		// filter accepts changed inputs while preserving fixed composite output dimensions.
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
		// Variable-frame-rate and static browser sources keep their last frame visible while active.
		// Source.End comes only from an explicit track.end event, so frame repetition cannot revive an
		// ended camera or screen.
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

// Recover recovers usable recording artifacts from the locally archived source tracks.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - dir: Validated local directory.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// boundedLog retains bounded process diagnostics without unbounded memory growth.
type boundedLog struct {
	mu   sync.Mutex
	data []byte
}

// Write accepts output bytes through the io.Writer contract while enforcing the component's
// buffering rules.
//
// Parameters:
//   - p: Input bytes consumed by this operation.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (b *boundedLog) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if len(b.data) > 8192 {
		b.data = b.data[len(b.data)-8192:]
	}
	return len(p), nil
}

// String returns the buffered diagnostic text in its bounded string representation.
//
// Returns:
//   - result 1: Bounded buffered diagnostic output.
func (b *boundedLog) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.data) }
