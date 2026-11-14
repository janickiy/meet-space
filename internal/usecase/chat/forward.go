package chat

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/chat"
)

type ForwardRepository interface {
	PrepareForward(context.Context, string, string, domain.ForwardRequest, string) (domain.ForwardPlan, error)
	CommitForward(context.Context, string, string, domain.ForwardRequest, string, []domain.Message, map[string]string) ([]domain.Message, bool, error)
}

type ForwardStorage interface {
	CopyAttachment(context.Context, string, string) error
}

func (s *Service) Forward(ctx context.Context, user, target string, request domain.ForwardRequest) ([]domain.Message, bool, error) {
	request, fingerprint, err := domain.NormalizeForward(request)
	if err != nil {
		return nil, false, err
	}
	repo, ok := s.repo.(ForwardRepository)
	if !ok {
		return nil, false, apperrors.ErrUnavailable
	}
	op, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	plan, err := repo.PrepareForward(op, user, target, request, fingerprint)
	if err != nil {
		return nil, false, err
	}
	if plan.Existing != nil {
		return plan.Existing, false, nil
	}
	select {
	case s.uploads <- struct{}{}:
		defer func() { <-s.uploads }()
	default:
		return nil, false, apperrors.New(apperrors.ErrUnavailable, "forwarding capacity is busy")
	}
	copied := make(map[string]string)
	for _, message := range plan.Messages {
		for _, attachment := range message.Attachments {
			a, err := s.copyForwardAttachment(op, user, target, request.ClientRequestID, attachment)
			if err != nil {
				return nil, false, err
			}
			copied[attachment.ID] = a.ID
		}
	}
	result, created, err := repo.CommitForward(op, user, target, request, fingerprint, plan.Messages, copied)
	if err == nil && created {
		for _, message := range result {
			s.publish(ctx, "chat.message.created", message)
		}
	}
	return result, created, err
}

func (s *Service) copyForwardAttachment(ctx context.Context, user, target, batch string, source domain.Attachment) (result domain.Attachment, err error) {
	storage, ok := s.storage.(ForwardStorage)
	if !ok {
		return result, apperrors.ErrUnavailable
	}
	a, _, err := s.InitAttachment(ctx, user, target, domain.InitRequest{ClientRequestID: domain.ForwardAttachmentRequestID(batch, source.ID), Filename: source.Filename, MimeType: source.MimeType, Size: source.Size})
	if err != nil {
		return result, err
	}
	if a.Status == "attached" || a.Status == "ready" {
		if a.Checksum != source.Checksum {
			return result, apperrors.ErrConflict
		}
		return a, nil
	}
	token := uuid.NewString()
	a, err = s.repo.ClaimUpload(ctx, user, target, a.ID, token)
	if err != nil {
		return result, err
	}
	defer func() {
		if err != nil && a.UploadedAt == nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = s.repo.AbortUpload(cleanup, a.ID, token)
		}
	}()
	if a.UploadedAt == nil {
		if err = storage.CopyAttachment(ctx, source.ObjectKey, a.ObjectKey); err != nil {
			return result, apperrors.ErrUnavailable
		}
		a, err = s.repo.CompleteUpload(ctx, user, target, a.ID, token, source.Checksum)
		if err != nil {
			return result, err
		}
	}
	if a.Checksum != source.Checksum {
		return result, apperrors.ErrConflict
	}
	return s.FinalizeAttachment(ctx, user, target, a.ID)
}
