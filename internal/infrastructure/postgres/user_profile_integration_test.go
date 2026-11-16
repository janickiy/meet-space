package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
	"github.com/janickiy/meet-space/internal/infrastructure/security"
	authusecase "github.com/janickiy/meet-space/internal/usecase/auth"
)

func TestProfileBioMigrationPreservesExistingAccounts(t *testing.T) {
	db := auditDatabase(t, 0)
	dir := copyTableMigrations(t)
	if err := os.Remove(filepath.Join(dir, "000105_user_profile_bio.up.sql")); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db, dir); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	if err := db.Exec("INSERT INTO users(id,email,password_hash,display_name,updated_at) VALUES(?,?,'synthetic',NULL,'2026-01-01T00:00:00Z')", id, "old@example.invalid").Error; err != nil {
		t.Fatal(err)
	}
	var before struct {
		Email, PasswordHash string
		DisplayName         *string
		UpdatedAt           time.Time
	}
	if err := db.Table("users").Where("id=?", id).Take(&before).Error; err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db, tableMigrationDirectory()); err != nil {
		t.Fatal(err)
	}
	got, err := NewUserRepository(db).GetByID(context.Background(), id)
	if err != nil || got.Bio != "" || got.DisplayName != nil || got.Email != before.Email || got.PasswordHash != before.PasswordHash || !got.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("migration changed existing account: %v", err)
	}
	if err := db.Exec("UPDATE users SET bio=? WHERE id=?", strings.Repeat("🙂", 1000), id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE users SET bio=? WHERE id=?", strings.Repeat("🙂", 1001), id).Error; err == nil {
		t.Fatal("database accepted overlong bio")
	}
	if err := RunMigrations(db, tableMigrationDirectory()); err != nil {
		t.Fatal("migration not repeatable", err)
	}
}

func TestProfileBioAtomicPartialUpdatesAndAuthorization(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewUserRepository(db)
	id, avatar := uuid.NewString(), uuid.NewString()
	name := "Existing name"
	user := users.User{ID: id, Email: "profile@example.invalid", PasswordHash: "synthetic-not-a-login-hash", DisplayName: &name, AvatarVersion: &avatar, AuthVersion: 7}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	bio := "Первая\r\n🙂 Вторая"
	result, err := repo.UpdateProfile(ctx, id, users.UpdateProfileRequest{Bio: &bio}, 7)
	if err != nil || result.Bio != "Первая\n🙂 Вторая" || result.DisplayName == nil || *result.DisplayName != name || result.AvatarVersion == nil || *result.AvatarVersion != avatar || !result.UpdatedAt.After(user.UpdatedAt) {
		t.Fatalf("partial update changed unrelated fields or timestamp: %v", err)
	}
	newName, invalid := "New name", strings.Repeat("я", 1001)
	if _, err := repo.UpdateProfile(ctx, id, users.UpdateProfileRequest{DisplayName: &newName, Bio: &invalid}, 7); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("invalid combined patch accepted", err)
	}
	unchanged, err := repo.GetByID(ctx, id)
	if err != nil || *unchanged.DisplayName != name || unchanged.Bio != result.Bio {
		t.Fatal("invalid combined patch partially persisted", err)
	}
	// Independent patches may race; each must preserve the omitted field.
	newBio := "Concurrently edited bio"
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, patch := range []users.UpdateProfileRequest{{DisplayName: &newName}, {Bio: &newBio}} {
		wg.Add(1)
		go func(p users.UpdateProfileRequest) {
			defer wg.Done()
			<-start
			_, err := repo.UpdateProfile(ctx, id, p, 7)
			errs <- err
		}(patch)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	result, err = repo.GetByID(ctx, id)
	if err != nil || *result.DisplayName != newName || result.Bio != newBio {
		t.Fatal("concurrent partial patches lost data", err)
	}
	if _, err := repo.UpdateProfile(ctx, id, users.UpdateProfileRequest{Bio: &bio}, 6); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("stale auth version accepted", err)
	}
	if err := db.Exec("UPDATE users SET is_blocked=true WHERE id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateProfile(ctx, id, users.UpdateProfileRequest{Bio: &bio}, 7); !errors.Is(err, apperrors.ErrAccountBlocked) {
		t.Fatal("blocked user accepted", err)
	}
	if err := db.Exec("UPDATE users SET is_blocked=false WHERE id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE platform_settings SET values=values||'{"ldapEnabled":true}'::jsonb WHERE singleton`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateProfile(ctx, id, users.UpdateProfileRequest{Bio: &bio}, 7); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("inactive authentication mode accepted", err)
	}
	if err := db.Exec("UPDATE users SET auth_source='ldap',password_hash='' WHERE id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	empty := ""
	if _, err := repo.UpdateProfile(ctx, id, users.UpdateProfileRequest{DisplayName: &name, Bio: &empty}, 7); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("LDAP name editable", err)
	}
	result, err = repo.GetByID(ctx, id)
	if err != nil || *result.DisplayName != newName || result.Bio != newBio {
		t.Fatal("rejected LDAP combined patch partially persisted", err)
	}
	result, err = repo.UpdateProfile(ctx, id, users.UpdateProfileRequest{Bio: &empty}, 7)
	if err != nil || result.Bio != "" || *result.DisplayName != newName {
		t.Fatal("LDAP cannot clear own bio", err)
	}
}

func TestProfileBioOwnResponsesAndAccountErasure(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewUserRepository(db)
	user := users.User{ID: uuid.NewString(), Email: "privatebio@example.invalid", PasswordHash: "synthetic-not-a-login-hash"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	bio := "Private biography"
	if _, err := repo.UpdateProfile(ctx, user.ID, users.UpdateProfileRequest{Bio: &bio}, user.AuthVersion); err != nil {
		t.Fatal(err)
	}
	tokens, err := security.NewTokenService("synthetic-profile-secret-32-bytes-long")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := authusecase.NewService(repo, auditPasswords{}, tokens)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithSessions(NewAuthSessionRepository(db))
	login, err := svc.Login(ctx, users.LoginRequest{Email: user.Email, Password: "synthetic-password"})
	if err != nil {
		t.Fatal(err)
	}
	own, err := svc.Me(ctx, user.ID)
	if err != nil || own.Bio == nil || *own.Bio != bio || login.User.Bio == nil || *login.User.Bio != bio {
		t.Fatal("own auth responses lost bio", err)
	}
	account, err := svc.SessionAccount(ctx, login.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(account)
	if err != nil || strings.Contains(string(encoded), `"bio":`) || strings.Contains(string(encoded), bio) {
		t.Fatal("bio leaked through browser account picker", err)
	}
	// Shared history forces anonymization rather than physical removal.
	if err := db.Exec("INSERT INTO conferences(id,owner_id,title,invite_code,status,started_at,finished_at) VALUES(?,?,'Ended',?,'finished',now()-interval '1 hour',now())", uuid.NewString(), user.ID, strings.Repeat("z", 32)).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteAccount(ctx, user.ID, user.PasswordHash); err != nil {
		t.Fatal(err)
	}
	var deleted users.User
	if err := db.Where("id=?", user.ID).Take(&deleted).Error; err != nil {
		t.Fatal(err)
	}
	if deleted.DeletedAt == nil || deleted.Bio != "" {
		t.Fatal("account anonymization retained bio")
	}
}
