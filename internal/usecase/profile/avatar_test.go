package profile

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"testing"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
)

const testActor = "00000000-0000-4000-8000-000000000001"

type avatarRepo struct {
	user    users.User
	data    []byte
	savedID string
}

func (r *avatarRepo) GetByID(context.Context, string) (users.User, error) { return r.user, nil }
func (r *avatarRepo) SaveAvatar(_ context.Context, id string, data []byte) (users.User, error) {
	r.savedID = id
	r.data = data
	return r.user, nil
}
func (r *avatarRepo) ReadAvatar(context.Context, string, string) ([]byte, error) { return r.data, nil }

type forbiddenBody struct{ reads int }

func (b *forbiddenBody) Read([]byte) (int, error) { b.reads++; return 0, io.EOF }

// TestAvatarChangesOwnAccount verifies mutation scope, normalization and deletion.
func TestAvatarChangesOwnAccount(t *testing.T) {
	repo := &avatarRepo{user: users.User{ID: testActor, PasswordHash: "synthetic"}}
	s := NewAvatarService(repo)
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 32, 16))); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(context.Background(), testActor, "image/png", &imageData); err != nil {
		t.Fatal(err)
	}
	if repo.savedID != testActor || len(repo.data) == 0 {
		t.Fatal("photo not scoped to actor")
	}
	if _, err := s.Delete(context.Background(), testActor); err != nil {
		t.Fatal(err)
	}
	if repo.data != nil {
		t.Fatal("photo not deleted")
	}
}

// TestAvatarDeniedBeforeDecode ensures guest and canceled requests cannot decode.
func TestAvatarDeniedBeforeDecode(t *testing.T) {
	conference := "00000000-0000-4000-8000-000000000002"
	for _, guest := range []bool{true, false} {
		t.Run(map[bool]string{true: "guest", false: "canceled"}[guest], func(t *testing.T) {
			repo := &avatarRepo{user: users.User{ID: testActor, PasswordHash: "synthetic"}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := apperrors.ErrForbidden
			if guest {
				repo.user.GuestConferenceID = &conference
			} else {
				cancel()
				want = context.Canceled
			}
			body := &forbiddenBody{}
			_, err := NewAvatarService(repo).Put(ctx, testActor, "image/png", body)
			if !errors.Is(err, want) || body.reads != 0 || repo.savedID != "" {
				t.Fatalf("err=%v reads=%d saved=%q", err, body.reads, repo.savedID)
			}
		})
	}
}

// TestAvatarDecodeBudget rejects excess work rather than accumulating uploads.
func TestAvatarDecodeBudget(t *testing.T) {
	s := NewAvatarService(&avatarRepo{user: users.User{ID: testActor, PasswordHash: "synthetic"}})
	s.uploads <- struct{}{}
	s.uploads <- struct{}{}
	body := &forbiddenBody{}
	if _, err := s.Put(context.Background(), testActor, "image/png", body); !errors.Is(err, apperrors.ErrRateLimited) || body.reads != 0 {
		t.Fatalf("error=%v reads=%d", err, body.reads)
	}
}
