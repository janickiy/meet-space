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
	// UploadFile uploads a local recording artifact and returns its stored object metadata.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Server-generated private object-storage key.
	//   - argument 3: Validated local filesystem or object path.
	//   - argument 4: Declared or detected MIME type of the content.
	//
	// Returns:
	//   - result 1: The resulting uploaded object.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	UploadFile(context.Context, string, string, string) (s3storage.UploadedObject, error)
	// RemovePrefix removes objects only within the validated server-owned recording prefix.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Server-controlled object or key prefix.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	RemovePrefix(context.Context, string) error
}

type compositeArtifactRepository interface {
	// SaveCompositeArtifacts commits composite artifacts only for the recording generation still
	// owned by the worker.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Signed credential or operation-ownership token to verify.
	//   - argument 4: Whether this is the final chunk and the archive must be closed.
	//   - argument 5: Optional preview-file metadata associated with the recording.
	//   - argument 6: Ordered transcript or media segments.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SaveCompositeArtifacts(context.Context, string, string, records.RecordFile, *records.RecordFile, []records.RecordSegment) error
}

// compositeArtifactPublisher binds publication dependencies, not capture or leases.
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

// compositeArtifactPublication owns one attempt's remote staging prefix and ZIP.
// The recording goroutine calls Publish once and defers Close until its operation
// ends, including post-commit events and local cleanup. It is not shared.
type compositeArtifactPublication struct {
	publisher compositeArtifactPublisher
	command   compositeArtifactCommand
	base      string
	archive   string
	armed     bool
	committed bool
	closed    bool
}

// begin creates the artifact-publication state bound to the current recording command.
//
// Parameters:
//   - command: Validated internal recording or media command.
//
// Returns:
//   - result 1: The constructed or loaded composite artifact publication.
func (p compositeArtifactPublisher) begin(command compositeArtifactCommand) *compositeArtifactPublication {
	return &compositeArtifactPublication{publisher: p, command: command}
}

// Publish stages immutable artifacts and commits their metadata under the lease.
// Close owns rollback on every pre-commit failure; no retry is performed here.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *compositeArtifactPublication) Publish(ctx context.Context) error {
	if p.publisher.storage == nil {
		return fmt.Errorf("recording storage is unavailable")
	}
	record, result := p.command.record, p.command.result
	// A late former owner can neither overwrite nor clean another attempt's files.
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

// Close is idempotent and preserves the former deferred cleanup order: ZIP first,
// then best-effort rollback of this attempt's prefix, only before a fenced commit.
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
