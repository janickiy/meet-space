package personal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"io"
	"testing"

	"github.com/janickiy/meet-space/internal/domain/personal"
)

type avatarRepository struct {
	AssetRepository
	pending Asset
}

func (r *avatarRepository) BeginAvatar(_ context.Context, _, _ string, a Asset) error {
	r.pending = a
	return nil
}
func (r *avatarRepository) ActivateAvatar(_ context.Context, _, conversation, version string) (personal.Conversation, error) {
	return personal.Conversation{ID: conversation, Type: "group", AvatarVersion: &version}, nil
}

type avatarStorage struct {
	AssetStorage
	data          []byte
	typ, checksum string
}

func (s *avatarStorage) CheckAttachmentPrivacy(context.Context) error { return nil }
func (s *avatarStorage) PutPersonalAvatar(_ context.Context, _ string, r io.Reader, _ int64, typ, sum string) error {
	var err error
	s.data, err = io.ReadAll(r)
	s.typ = typ
	s.checksum = sum
	return err
}

// TestGroupAvatarUpload keeps metadata, content type and storage checksum aligned.
func TestGroupAvatarUpload(t *testing.T) {
	repo := &avatarRepository{}
	storage := &avatarStorage{}
	s, err := NewAssetService(context.Background(), repo, storage, nil)
	if err != nil {
		t.Fatal(err)
	}
	var input bytes.Buffer
	if err := png.Encode(&input, image.NewRGBA(image.Rect(0, 0, 900, 600))); err != nil {
		t.Fatal(err)
	}
	group, err := s.PutAvatar(context.Background(), "00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002", "image/png", &input)
	if err != nil {
		t.Fatal(err)
	}
	if group.AvatarVersion == nil || *group.AvatarVersion != repo.pending.Version || storage.typ != "image/jpeg" || repo.pending.ContentType != storage.typ || repo.pending.Size != int64(len(storage.data)) {
		t.Fatal("avatar metadata mismatch")
	}
	sum := sha256.Sum256(storage.data)
	if storage.checksum != hex.EncodeToString(sum[:]) || storage.checksum != repo.pending.Checksum {
		t.Fatal("avatar checksum mismatch")
	}
}
