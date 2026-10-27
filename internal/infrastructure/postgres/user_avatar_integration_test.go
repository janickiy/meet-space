package postgres

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
)

// TestAuditProfileAvatars covers persistence, replacement, deletion and erasure
// in the same empty disposable database guard as other audit regressions.
func TestAuditProfileAvatars(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewUserRepository(db)
	actor, other := uuid.NewString(), uuid.NewString()
	for _, id := range []string{actor, other} {
		if err := db.Create(&users.User{ID: id, Email: id + "@example.invalid", PasswordHash: "synthetic"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	data := []byte("synthetic service-normalized thumbnail")
	first, err := repo.SaveAvatar(ctx, actor, data)
	if err != nil || first.AvatarVersion == nil {
		t.Fatalf("save error=%v version=%v", err, first.AvatarVersion)
	}
	read, err := repo.ReadAvatar(ctx, other, actor)
	if err != nil || !bytes.Equal(read, data) {
		t.Fatalf("registered reader: error=%v", err)
	}
	if _, err := repo.ReadAvatar(ctx, uuid.NewString(), actor); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("unknown reader=%v", err)
	}
	second, err := repo.SaveAvatar(ctx, actor, []byte("replacement"))
	if err != nil || second.AvatarVersion == nil || *second.AvatarVersion == *first.AvatarVersion {
		t.Fatalf("replacement=%v", err)
	}
	unchanged, err := repo.GetByID(ctx, other)
	if err != nil || unchanged.AvatarVersion != nil {
		t.Fatal("another account was changed")
	}
	cleared, err := repo.SaveAvatar(ctx, actor, nil)
	if err != nil || cleared.AvatarVersion != nil {
		t.Fatalf("clear=%v", err)
	}
	if _, err := repo.ReadAvatar(ctx, other, actor); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("deleted content=%v", err)
	}
	if _, err := repo.SaveAvatar(ctx, actor, data); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewPersonalRepository(db).GetOrCreate(ctx, actor, other); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteAccount(ctx, actor, "synthetic"); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Table("user_avatars").Where("user_id=?", actor).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("erased photos=%d error=%v", count, err)
	}
	if _, err := repo.ReadAvatar(ctx, other, actor); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("erased content=%v", err)
	}
}
