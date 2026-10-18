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

// PostProcessor combines segments, validates output and creates final recording artifacts.
type PostProcessor struct {
	ffmpegPath  string
	ffprobePath string
}

// Result contains generated recording artifacts and their processing metadata.
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

// Segment contains validated local recording-segment metadata.
type Segment struct {
	SeqNo          int
	Path           string
	FileName       string
	SizeBytes      int64
	ChecksumSHA256 string
}

// NewPostProcessor constructs the post processor with the dependencies and limits required for
// FFmpeg recording and post-processing.
//
// Parameters:
//   - ffmpegPath: Path to the configured FFmpeg executable.
//
// Returns:
//   - result 1: The constructed or loaded post processor.
func NewPostProcessor(ffmpegPath string) *PostProcessor {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}

	return &PostProcessor{
		ffmpegPath:  ffmpegPath,
		ffprobePath: ffprobePath(ffmpegPath),
	}
}

// Finalize combines usable recording segments and prepares the final file and preview.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - recordDir: Validated local directory of the recording job.
//
// Returns:
//   - result 1: The resulting result.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *PostProcessor) Finalize(ctx context.Context, recordDir string) (Result, error) {
	return p.finalize(ctx, recordDir, false)
}

// FinalizeComposite finalizes composite output and prepares the recording artifacts for
// publication.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - recordDir: Validated local directory of the recording job.
//
// Returns:
//   - result 1: The resulting result.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *PostProcessor) FinalizeComposite(ctx context.Context, recordDir string) (Result, error) {
	return p.finalize(ctx, recordDir, true)
}

// finalize executes the shared recording finalization path and validates its generated
// artifacts.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - recordDir: Validated local directory of the recording job.
//   - composite: Whether this output is a composite conference recording.
//
// Returns:
//   - result 1: The resulting result.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *PostProcessor) finalize(ctx context.Context, recordDir string, composite bool) (Result, error) {
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
	var concatErr error
	if composite {
		concatErr = p.concatComposite(ctx, listPath, finalPath)
	} else {
		concatErr = p.concat(ctx, listPath, finalPath)
	}
	if concatErr != nil {
		return Result{}, concatErr
	}
	var durationSec int
	if composite {
		validated, err := p.ValidateOutput(ctx, finalPath, true)
		if err != nil {
			return Result{}, err
		}
		durationSec = int(validated.Duration + 0.5)
	}
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
	if !composite {
		durationSec, _ = p.durationSec(ctx, finalPath)
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

// concat concatenates the ordered recording segments into one output file.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - listPath: Path of the validated FFmpeg concat input list.
//   - finalPath: Path of the final generated recording artifact.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *PostProcessor) concat(ctx context.Context, listPath string, finalPath string) error {
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	args := []string{
		"-hide_banner",
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-b:v", "6M",
		"-maxrate", "6M",
		"-bufsize", "12M",
		"-r", "30",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-movflags", "+faststart",
		finalPath,
	}
	output, err := runBounded(runCtx, p.ffmpegPath, args...)
	if err != nil {
		return fmt.Errorf("concat final video: %w; ffmpeg=%s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

// preview creates the recording preview image using the validated output duration.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - finalPath: Path of the final generated recording artifact.
//   - previewPath: Path of the generated preview image.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *PostProcessor) preview(ctx context.Context, finalPath string, previewPath string) error {
	if err := p.previewAt(ctx, finalPath, previewPath, "1"); err == nil {
		return nil
	}

	return p.previewAt(ctx, finalPath, previewPath, "0")
}

// previewAt extracts a recording preview image at the selected bounded timestamp.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - finalPath: Path of the final generated recording artifact.
//   - previewPath: Path of the generated preview image.
//   - second: Timestamp used to select the preview frame.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// durationSec reads the media duration rounded to the recording metadata representation.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - path: Validated local filesystem or object path.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *PostProcessor) durationSec(ctx context.Context, path string) (int, error) {
	value, err := p.durationFloat(ctx, path)
	if err != nil {
		return 0, err
	}

	return int(value + 0.5), nil
}

// durationFloat probes the media duration with fractional-second precision.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - path: Validated local filesystem or object path.
//
// Returns:
//   - result 1: The resulting float64.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// usableSegments filters local recording segments to those usable for final media output.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - segments: Ordered transcript or media segments.
//
// Returns:
//   - result 1: The collected string values.
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

// findSegments finds the local segment files belonging to the requested recording.
//
// Parameters:
//   - recordDir: Validated local directory of the recording job.
//
// Returns:
//   - result 1: The collected string values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// segmentInfos builds the ordered segment metadata used by finalization.
//
// Parameters:
//   - paths: Validated local paths included in this operation.
//
// Returns:
//   - result 1: The collected segment values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// segmentSeqNo extracts the sequence number used to order a recording segment.
//
// Parameters:
//   - fileName: Segment filename from which metadata is extracted.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func segmentSeqNo(fileName string) (int, error) {
	trimmed := strings.TrimSuffix(strings.TrimPrefix(fileName, "segment_"), filepath.Ext(fileName))
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("parse segment seq no %q: %w", fileName, err)
	}

	return value, nil
}

// writeConcatList writes the validated FFmpeg concat input list for the selected segments.
//
// Parameters:
//   - path: Validated local filesystem or object path.
//   - segments: Ordered transcript or media segments.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func writeConcatList(path string, segments []string) error {
	var b strings.Builder
	for _, segment := range segments {
		b.WriteString("file '")
		b.WriteString(strings.ReplaceAll(segment, "'", "'\\''"))
		b.WriteString("'\n")
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// checksumSHA256 streams file bytes into a SHA-256 digest for artifact integrity checks.
//
// Parameters:
//   - path: Validated local filesystem or object path.
//
// Returns:
//   - result 1: Hex-encoded SHA-256 of the file bytes.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// ffprobePath resolves the ffprobe executable associated with the configured FFmpeg binary.
//
// Parameters:
//   - ffmpegPath: Path to the configured FFmpeg executable.
//
// Returns:
//   - result 1: Resolved path of the configured ffprobe executable.
func ffprobePath(ffmpegPath string) string {
	if ffmpegPath == "ffmpeg" {
		return "ffprobe"
	}
	return filepath.Join(filepath.Dir(ffmpegPath), "ffprobe")
}
