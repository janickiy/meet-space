package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FinalizeAudio converts the recorded input into the bounded audio artifact required by
// downstream processing.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - dir: Validated local directory.
//
// Returns:
//   - result 1: The resulting result.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *PostProcessor) FinalizeAudio(ctx context.Context, dir string) (Result, error) {
	paths, err := findSegments(dir)
	if err != nil {
		return Result{}, err
	}
	paths = p.usableSegments(ctx, paths)
	if len(paths) == 0 {
		return Result{}, fmt.Errorf("no audio segments")
	}
	list := filepath.Join(dir, "concat.txt")
	if err = writeConcatList(list, paths); err != nil {
		return Result{}, err
	}
	final := filepath.Join(dir, "final.mp4")
	op, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	if _, err = runBounded(op, p.ffmpegPath, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-f", "concat", "-safe", "0", "-i", list, "-map", "0:a:0", "-vn", "-c:a", "copy", "-movflags", "+faststart", final); err != nil {
		return Result{}, err
	}
	validated, err := p.validateOutput(ctx, final, true, false)
	if err != nil {
		return Result{}, err
	}
	stat, err := os.Stat(final)
	if err != nil {
		return Result{}, err
	}
	sum, err := checksumSHA256(final)
	if err != nil {
		return Result{}, err
	}
	segments, err := segmentInfos(paths)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	duration := int(validated.Duration + 0.5)
	return Result{FinalPath: final, FinalSizeBytes: stat.Size(), FinalChecksum: sum, DurationSec: duration, Segments: segments}, nil
}
