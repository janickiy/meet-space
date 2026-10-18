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

// RecordingObjectReader reads only private stored objects and never accepts external URLs.
type RecordingObjectReader interface {
	// OpenRecording opens the private recording audio or media stream using its stored artifact
	// identity.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Storage, cache or cryptographic key used by the operation.
	//   - argument 3: Maximum permitted size or number of characters.
	//
	// Returns:
	//   - result 1: The resulting read closer.
	//   - result 2: The resulting count, size or numeric value.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	OpenRecording(context.Context, string, int64) (io.ReadCloser, int64, error)
}

// TranscriptionAudio extracts WAV audio independently of recording finalization and media
// transport.
type TranscriptionAudio struct {
	objects                      RecordingObjectReader
	binary, tempRoot             string
	maxVideoBytes, maxAudioBytes int64
	maxDuration                  int
}

// NewTranscriptionAudio constructs the transcription audio with the dependencies and limits
// required for FFmpeg recording and post-processing.
//
// Parameters:
//   - objects: Private recording-object reader.
//   - binary: Path to the configured external executable.
//   - tempRoot: Root directory restricting temporary media files.
//   - maxVideoBytes: Maximum video input size in bytes.
//   - maxAudioBytes: Maximum audio input size in bytes.
//   - maxDuration: Maximum media duration accepted by this processing path.
//
// Returns:
//   - result 1: The constructed or loaded transcription audio.
func NewTranscriptionAudio(objects RecordingObjectReader, binary, tempRoot string, maxVideoBytes, maxAudioBytes int64, maxDuration int) *TranscriptionAudio {
	return &TranscriptionAudio{objects: objects, binary: binary, tempRoot: tempRoot, maxVideoBytes: maxVideoBytes, maxAudioBytes: maxAudioBytes, maxDuration: maxDuration}
}

// Open opens the scoped media, recording or session resource required by this component.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - source: Media or content source belonging to the current operation.
//
// Returns:
//   - result 1: The resulting audio.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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
