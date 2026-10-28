package profile

import (
	"context"
	"io"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/avatar"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/users"
)

type AvatarRepository interface {
	GetByID(context.Context, string) (users.User, error)
	SaveAvatar(context.Context, string, []byte) (users.User, error)
	ReadAvatar(context.Context, string, string) ([]byte, error)
}

type AvatarService struct {
	repo    AvatarRepository
	uploads chan struct{}
}

// NewAvatarService shares a two-decoder budget across all profile uploads.
// repo persists thumbnails and checks the authenticated reader's account.
func NewAvatarService(repo AvatarRepository) *AvatarService {
	return &AvatarService{repo: repo, uploads: make(chan struct{}, 2)}
}

// Put replaces only actor's own avatar with normalized body of contentType.
// ctx controls the deadline; guest and deleted accounts cannot upload images.
func (s *AvatarService) Put(ctx context.Context, actor, contentType string, body io.Reader) (users.View, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := s.account(ctx, actor); err != nil {
		return users.View{}, err
	}
	if err := ctx.Err(); err != nil {
		return users.View{}, err
	}
	select {
	case s.uploads <- struct{}{}:
		defer func() { <-s.uploads }()
	default:
		return users.View{}, apperrors.ErrRateLimited
	}
	data, err := avatar.Normalize(body, contentType)
	if err != nil {
		return users.View{}, err
	}
	user, err := s.repo.SaveAvatar(ctx, actor, data)
	return user.View(), err
}

// Delete removes actor's photo without affecting the name or other users.
func (s *AvatarService) Delete(ctx context.Context, actor string) (users.View, error) {
	if err := s.account(ctx, actor); err != nil {
		return users.View{}, err
	}
	user, err := s.repo.SaveAvatar(ctx, actor, nil)
	return user.View(), err
}

// Get returns target's current photo to a registered, authenticated actor.
// Profile photos are directory-visible, like display names, not public assets.
func (s *AvatarService) Get(ctx context.Context, actor, target string) ([]byte, error) {
	if _, err := chat.UUID(target); err != nil {
		return nil, err
	}
	if err := s.account(ctx, actor); err != nil {
		return nil, err
	}
	return s.repo.ReadAvatar(ctx, actor, target)
}

// account validates actor before any decoding or database mutation occurs.
func (s *AvatarService) account(ctx context.Context, actor string) error {
	if _, err := chat.UUID(actor); err != nil {
		return err
	}
	user, err := s.repo.GetByID(ctx, actor)
	if err != nil {
		return err
	}
	if user.GuestConferenceID != nil || user.PasswordHash == "" {
		return apperrors.ErrForbidden
	}
	return nil
}
