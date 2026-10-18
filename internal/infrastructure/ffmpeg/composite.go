package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// OutputValidation contains ffprobe checks of the generated media artifact.
type OutputValidation struct {
	Duration      float64 `json:"duration"`
	VideoDuration float64 `json:"videoDuration"`
	AudioDuration float64 `json:"audioDuration"`
	VideoCodec    string  `json:"videoCodec"`
	AudioCodec    string  `json:"audioCodec"`
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	Size          int64   `json:"size"`
}

// concatComposite concatenates validated composite fragments into the final recording artifact.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - listPath: Path of the validated FFmpeg concat input list.
//   - finalPath: Path of the final generated recording artifact.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *PostProcessor) concatComposite(ctx context.Context, listPath, finalPath string) error {
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	output, err := runBounded(runCtx, p.ffmpegPath, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-f", "concat", "-safe", "0", "-i", listPath, "-map", "0:v:0", "-map", "0:a:0", "-c", "copy", "-movflags", "+faststart", finalPath)
	if err != nil {
		return fmt.Errorf("concat composite: %w: %s", err, output)
	}
	return nil
}

// ValidateOutput checks that a produced media artifact is nonempty and can be probed.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - path: Validated local filesystem or object path.
//   - requireAudio: Whether the resulting media must contain an audio stream.
//
// Returns:
//   - result 1: The resulting output validation.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *PostProcessor) ValidateOutput(ctx context.Context, path string, requireAudio bool) (OutputValidation, error) {
	return p.validateOutput(ctx, path, requireAudio, true)
}

// validateOutput checks the generated media artifact before it is accepted as a successful
// result.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - path: Validated local filesystem or object path.
//   - requireAudio: Whether the resulting media must contain an audio stream.
//   - requireVideo: Whether the resulting media must contain a video stream.
//
// Returns:
//   - result 1: The resulting output validation.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *PostProcessor) validateOutput(ctx context.Context, path string, requireAudio, requireVideo bool) (OutputValidation, error) {
	result := OutputValidation{}
	info, err := os.Stat(path)
	if err != nil {
		return result, err
	}
	result.Size = info.Size()
	if info.Size() < 1024 {
		return result, fmt.Errorf("recording output is too small")
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	data, err := runBounded(runCtx, p.ffprobePath, "-v", "error", "-show_entries", "format=duration,format_name:stream=codec_type,codec_name,duration,width,height", "-of", "json", path)
	if err != nil {
		return result, fmt.Errorf("probe recording output: %w", err)
	}
	var probe struct {
		Format struct {
			Duration string `json:"duration"`
			Name     string `json:"format_name"`
		} `json:"format"`
		Streams []struct {
			Type     string `json:"codec_type"`
			Codec    string `json:"codec_name"`
			Duration string `json:"duration"`
			Width    int    `json:"width"`
			Height   int    `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return result, err
	}
	result.Duration, _ = strconv.ParseFloat(probe.Format.Duration, 64)
	if !strings.Contains(probe.Format.Name, "mp4") || result.Duration <= 0 || math.IsInf(result.Duration, 0) || math.IsNaN(result.Duration) {
		return result, fmt.Errorf("invalid MP4 duration or container")
	}
	for _, stream := range probe.Streams {
		duration, _ := strconv.ParseFloat(stream.Duration, 64)
		switch stream.Type {
		case "video":
			result.VideoCodec = stream.Codec
			result.VideoDuration = duration
			result.Width = stream.Width
			result.Height = stream.Height
		case "audio":
			result.AudioCodec = stream.Codec
			result.AudioDuration = duration
		}
	}
	if requireVideo && (result.VideoCodec != "h264" || result.Width <= 0 || result.Height <= 0 || result.VideoDuration <= 0) {
		return result, fmt.Errorf("composite output lacks valid H.264 video")
	}
	if requireAudio && (result.AudioCodec != "aac" || result.AudioDuration <= 0) {
		return result, fmt.Errorf("composite output lacks valid AAC audio")
	}
	if !requireVideo && result.VideoCodec != "" {
		return result, fmt.Errorf("audio output unexpectedly contains video")
	}
	if requireVideo && result.AudioDuration > 0 && math.Abs(result.VideoDuration-result.AudioDuration) > 0.35 {
		return result, fmt.Errorf("composite A/V duration mismatch: video %.3fs audio %.3fs", result.VideoDuration, result.AudioDuration)
	}
	return result, nil
}
