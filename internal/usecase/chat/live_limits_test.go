package chat

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/settings"
)

type attachmentSettings struct {
	limit int64
	err   error
}

func (s *attachmentSettings) Snapshot(context.Context) (settings.Values, error) {
	return settings.Values{AttachmentMaxBytes: s.limit}, s.err
}

type attachmentPolicyRepo struct {
	Repository
	attachment           domain.Attachment
	aborted, initialized bool
}

func (r *attachmentPolicyRepo) InitAttachment(_ context.Context, _, _ string, request domain.InitRequest) (domain.Attachment, bool, error) {
	r.initialized = true
	return domain.Attachment{Size: request.Size}, true, nil
}
func (r *attachmentPolicyRepo) ClaimUpload(context.Context, string, string, string, string) (domain.Attachment, error) {
	return r.attachment, nil
}
func (r *attachmentPolicyRepo) AbortUpload(context.Context, string, string) error {
	r.aborted = true
	return nil
}
func (r *attachmentPolicyRepo) CompleteUpload(context.Context, string, string, string, string, string) (domain.Attachment, error) {
	return r.attachment, nil
}
func (r *attachmentPolicyRepo) AttachmentForFinalize(context.Context, string, string, string) (domain.Attachment, error) {
	return r.attachment, nil
}
func (r *attachmentPolicyRepo) DownloadAttachment(context.Context, string, string, string) (domain.Attachment, error) {
	return r.attachment, nil
}

type attachmentPolicyStorage struct {
	Storage
	writes int
	size   int64
}

func (s *attachmentPolicyStorage) PutAttachment(_ context.Context, _ string, r io.Reader, size int64, _, _ string) error {
	n, err := io.Copy(io.Discard, r)
	if err != nil {
		return err
	}
	if n != size {
		return errors.New("wrong upload size")
	}
	s.writes++
	return nil
}
func (s *attachmentPolicyStorage) StatAttachment(context.Context, string) (int64, string, string, error) {
	return s.size, "text/plain", "", nil
}
func (s *attachmentPolicyStorage) AttachmentDownloadURL(context.Context, string, string, time.Duration) (string, error) {
	return "https://example.invalid/signed", nil
}

func TestLiveAttachmentPolicyMetadataBodyAndExistingDownload(t *testing.T) {
	ctx := context.Background()
	policy := &attachmentSettings{limit: 20 << 20}
	repo := &attachmentPolicyRepo{}
	storage := &attachmentPolicyStorage{}
	svc := (&Service{repo: repo, storage: storage, uploads: make(chan struct{}, 4)}).WithSettings(policy)
	request := domain.InitRequest{ClientRequestID: uuid.NewString(), Filename: "notes.txt", MimeType: "text/plain", Size: (10 << 20) + 1}
	if _, _, err := svc.InitAttachment(ctx, "user", "scope", request); err != nil {
		t.Fatal("raised metadata limit rejected", err)
	}
	repo.attachment = domain.Attachment{Filename: request.Filename, MimeType: request.MimeType, Size: request.Size}
	body := bytes.Repeat([]byte("a"), int(request.Size))
	if _, err := svc.Upload(ctx, "user", "scope", "attachment", bytes.NewReader(body)); err != nil || storage.writes != 1 {
		t.Fatal("raised body limit rejected", err)
	}
	policy.limit = 1024
	if _, _, err := svc.InitAttachment(ctx, "user", "scope", request); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("lowered metadata limit ignored", err)
	}
	if _, err := svc.Upload(ctx, "user", "scope", "attachment", bytes.NewReader(body)); !errors.Is(err, apperrors.ErrInvalidInput) || storage.writes != 1 {
		t.Fatal("lowered upload limit ignored", err)
	}
	storage.size = request.Size
	if _, err := svc.FinalizeAttachment(ctx, "user", "scope", "attachment"); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("lowered finalize limit ignored", err)
	}
	if _, _, err := svc.Download(ctx, "user", "scope", "attachment"); err != nil {
		t.Fatal("existing attachment became unreadable", err)
	}
	repo.attachment.Size = 1024
	if _, err := svc.Upload(ctx, "user", "scope", "attachment", bytes.NewReader(bytes.Repeat([]byte("x"), 1025))); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("oversized body with valid metadata accepted", err)
	}
	policy.err = errors.New("settings unavailable")
	repo.initialized = false
	if _, _, err := svc.InitAttachment(ctx, "user", "scope", request); !errors.Is(err, apperrors.ErrUnavailable) || repo.initialized {
		t.Fatal("failed settings read did not close new operation", err)
	}
}
