// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/directory"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"github.com/janickiy/meet-space/internal/domain/settings"
	"github.com/janickiy/meet-space/internal/domain/users"
	"github.com/janickiy/meet-space/internal/infrastructure/security"
	authusecase "github.com/janickiy/meet-space/internal/usecase/auth"
	"gorm.io/gorm"
)

func ldapAuditDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db := auditDatabase(t)
	if err := db.Exec(`UPDATE platform_settings SET values=jsonb_set(values,'{ldapEnabled}','true'::jsonb),revision=1 WHERE singleton`).Error; err != nil {
		t.Fatal(err)
	}
	return db
}
func ldapAuditNow(t *testing.T, db *gorm.DB) time.Time {
	t.Helper()
	var now time.Time
	if err := db.Raw("SELECT clock_timestamp()").Scan(&now).Error; err != nil {
		t.Fatal(err)
	}
	return now
}
func ldapAuditEntry() directory.Entry {
	return directory.Entry{ExternalID: uuid.NewString(), Login: "alice", Email: "alice@example.invalid", DisplayName: "Алиса"}
}
func ldapAuditSession(user users.User) users.AuthSession {
	return users.AuthSession{ID: uuid.NewString(), UserID: user.ID, TokenHash: strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", ""), CreatedAt: time.Now().UTC(), ExpectedAuthSource: "ldap", ExpectedAuthVersion: user.AuthVersion, ExpectedLDAPRevision: 1}
}

// A deleted local account and an active LDAP identity both may have no local
// password; deleted_at, rather than an empty password, controls visibility.
func TestLDAPAccountLifecycleDistinguishesDeletedLocalAndLDAP(t *testing.T) {
	db := auditDatabase(t)
	local, deleted := uuid.NewString(), uuid.NewString()
	if err := db.Exec(`INSERT INTO users(id,email,password_hash,deleted_at)
	 VALUES(?,?,'synthetic',NULL),(?,?,'',clock_timestamp())`, local, "local@example.invalid", deleted, "deleted@example.invalid").Error; err != nil {
		t.Fatal(err)
	}
	repo := NewUserRepository(db)
	if user, err := repo.GetByID(context.Background(), local); err != nil || user.AuthSource != "local" || user.DeletedAt != nil {
		t.Fatalf("local lifecycle: %+v %v", user, err)
	}
	if _, err := repo.GetByID(context.Background(), deleted); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("deleted user became available: %v", err)
	}
	ldap := users.User{ID: uuid.NewString(), Email: "ldap@example.invalid", AuthSource: "ldap"}
	if err := db.Create(&ldap).Error; err != nil {
		t.Fatal(err)
	}
	if user, err := repo.GetByID(context.Background(), ldap.ID); err != nil || user.PasswordHash != "" || user.DeletedAt != nil {
		t.Fatalf("LDAP empty hash marked deleted: %+v %v", user, err)
	}
}

// Привязка сохраняет UUID, не объединяет совпадающие email и атомарно откатывает конфликтующий снимок.
func TestLDAPIdentityAndAtomicCollision(t *testing.T) {
	db := ldapAuditDatabase(t)
	ctx := context.Background()
	repo := NewDirectoryRepository(db)
	entry := ldapAuditEntry()
	user, err := repo.UpsertLDAP(ctx, entry, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !user.IsLDAP() || user.PasswordHash != "" || user.LDAPRevision != 1 {
		t.Fatalf("invalid LDAP profile: %+v", user)
	}
	entry.Email = "renamed@example.invalid"
	entry.DisplayName = "Новое имя"
	renamed, err := repo.UpsertLDAP(ctx, entry, 1)
	if err != nil || renamed.ID != user.ID {
		t.Fatalf("stable external ID lost: %v", err)
	}
	local := users.User{ID: uuid.NewString(), Email: "local@example.invalid", PasswordHash: "synthetic"}
	if err := db.Create(&local).Error; err != nil {
		t.Fatal(err)
	}
	collision := directory.Entry{ExternalID: uuid.NewString(), Login: "local", Email: local.Email, DisplayName: "Local"}
	if _, err = repo.UpsertLDAP(ctx, collision, 1); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("automatic linking accepted: %v", err)
	}
	entry.DisplayName = "Откатить имя"
	if _, err = repo.ApplyLDAPSnapshot(ctx, []directory.Entry{entry, collision}, 1, ldapAuditNow(t, db)); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("collision snapshot accepted: %v", err)
	}
	saved, err := NewUserRepository(db).GetByID(ctx, user.ID)
	if err != nil || saved.DisplayName == nil || *saved.DisplayName != "Новое имя" {
		t.Fatalf("snapshot partially committed: %+v %v", saved, err)
	}
	var linked int64
	if err := db.Table("ldap_identities").Where("user_id=?", local.ID).Count(&linked).Error; err != nil || linked != 0 {
		t.Fatalf("local user linked: %d %v", linked, err)
	}
	var advanced bool
	if err := db.Raw("SELECT last_snapshot_at IS NOT NULL FROM ldap_directory_state").Scan(&advanced).Error; err != nil || advanced {
		t.Fatalf("failed snapshot advanced watermark: %v %v", advanced, err)
	}
}

// Удаление из каталога отзывает SID; возвращение в каталог не снимает ручную блокировку.
func TestLDAPRevocationAndManualBlock(t *testing.T) {
	db := ldapAuditDatabase(t)
	ctx := context.Background()
	repo := NewDirectoryRepository(db)
	entry := ldapAuditEntry()
	user, err := repo.UpsertLDAP(ctx, entry, 1)
	if err != nil {
		t.Fatal(err)
	}
	sessions := NewAuthSessionRepository(db)
	session := ldapAuditSession(user)
	if err := sessions.Create(ctx, session); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.GetByID(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	out, err := repo.ApplyLDAPSnapshot(ctx, nil, 1, ldapAuditNow(t, db))
	if err != nil || out.Disabled != 1 || len(out.DisabledUserIDs) != 1 {
		t.Fatalf("missing account not disabled: %+v %v", out, err)
	}
	if _, err := sessions.GetByID(ctx, session.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("disabled SID accepted: %v", err)
	}
	if err := db.Exec("UPDATE users SET is_blocked=true WHERE id=?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApplyLDAPSnapshot(ctx, []directory.Entry{entry}, 1, ldapAuditNow(t, db)); err != nil {
		t.Fatal(err)
	}
	saved, err := NewUserRepository(db).GetByID(ctx, user.ID)
	if err != nil || saved.LDAPDisabled || !saved.IsBlocked {
		t.Fatalf("manual block lost: %+v %v", saved, err)
	}
	if err := sessions.Create(ctx, ldapAuditSession(saved)); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatalf("manual block bypass: %v", err)
	}
	if err := db.Exec("UPDATE users SET is_blocked=false WHERE id=?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	old, err := sessions.GetByID(ctx, session.ID)
	if err == nil && old.RevokedAt == nil {
		t.Fatal("old SID resurrected")
	}
	if err := sessions.Create(ctx, session); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatalf("old LDAP bind survived version change: %v", err)
	}
	fresh := ldapAuditSession(saved)
	if err := sessions.Create(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE platform_settings SET values=jsonb_set(values,'{ldapEnabled}','false'::jsonb),revision=2`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.GetByID(ctx, fresh.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("disabled provider SID accepted: %v", err)
	}
}

// Устаревшие снимки, арендованные задания и старые настройки не могут изменять актуальную авторизацию.
func TestLDAPStaleScanAndSessionGuards(t *testing.T) {
	db := ldapAuditDatabase(t)
	ctx := context.Background()
	repo := NewDirectoryRepository(db)
	entry := ldapAuditEntry()
	scanStart := ldapAuditNow(t, db)
	user, err := repo.UpsertLDAP(ctx, entry, 1)
	if err != nil {
		t.Fatal(err)
	}
	stale := entry
	stale.DisplayName = "Старое имя"
	stale.Disabled = true
	if _, err := repo.ApplyLDAPSnapshot(ctx, []directory.Entry{stale}, 1, scanStart); err != nil {
		t.Fatal(err)
	}
	saved, err := NewUserRepository(db).GetByID(ctx, user.ID)
	if err != nil || saved.LDAPDisabled || *saved.DisplayName != entry.DisplayName {
		t.Fatalf("old scan overwrote JIT: %+v %v", saved, err)
	}
	newEntry := directory.Entry{ExternalID: uuid.NewString(), Login: "bob", Email: "bob@example.invalid", DisplayName: "Боб"}
	if _, err := repo.ApplyLDAPSnapshot(ctx, []directory.Entry{newEntry}, 1, scanStart.Add(-time.Second)); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("older snapshot imported stale identity: %v", err)
	}
	lost := directory.WithSyncLease(ctx, jobs.Job{ID: uuid.NewString(), LeaseToken: uuid.NewString()})
	if _, err := repo.ApplyLDAPSnapshot(lost, []directory.Entry{newEntry}, 1, ldapAuditNow(t, db)); !errors.Is(err, jobs.ErrLeaseLost) {
		t.Fatalf("lost lease applied: %v", err)
	}
	var count int64
	if err := db.Table("ldap_identities").Where("external_id=?", newEntry.ExternalID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rejected scan changed users: %d %v", count, err)
	}
	if err := db.Exec("UPDATE platform_settings SET revision=2").Error; err != nil {
		t.Fatal(err)
	}
	if err := NewAuthSessionRepository(db).Create(ctx, ldapAuditSession(user)); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("stale configuration issued SID: %v", err)
	}
}

type ldapAuditProvider struct{ user users.User }

func (p ldapAuditProvider) Authenticate(context.Context, string, string) (users.User, error) {
	return p.user, nil
}

// LDAP использует обычные SID и запрещает обход через локальный пароль, изменение имени и удаление аккаунта.
func TestLDAPAuthLocalBoundary(t *testing.T) {
	db := ldapAuditDatabase(t)
	ctx := context.Background()
	entry := ldapAuditEntry()
	user, err := NewDirectoryRepository(db).UpsertLDAP(ctx, entry, 1)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := security.NewTokenService("synthetic-ldap-audit-secret-32-bytes")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := authusecase.NewService(NewUserRepository(db), auditPasswords{}, tokens)
	if err != nil {
		t.Fatal(err)
	}
	policy := &verificationPolicy{values: settings.Values{LDAPEnabled: true}}
	auth.WithSessions(NewAuthSessionRepository(db)).WithLDAP(ldapAuditProvider{user}).WithAuthenticationPolicy(policy)
	login, err := auth.LDAPLogin(ctx, users.LDAPLoginRequest{Login: entry.Login, Password: "synthetic-password"})
	if err != nil {
		t.Fatal(err)
	}
	id, _, sid, _, err := auth.VerifyAuthorization(ctx, login.AccessToken)
	if err != nil || id != user.ID || sid == "" {
		t.Fatalf("LDAP SID invalid: %s %s %v", id, sid, err)
	}
	if _, err := auth.Refresh(ctx, login.SessionToken); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.UpdateProfile(ctx, user.ID, users.UpdateProfileRequest{DisplayName: "Изменить"}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("LDAP rename permitted: %v", err)
	}
	if err := auth.DeleteAccount(ctx, user.ID, "anything"); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("LDAP deletion permitted: %v", err)
	}
	// Даже случайно заполненный локальный хеш не превращает корпоративный аккаунт в локальный.
	if err := db.Exec("UPDATE users SET password_hash='synthetic-not-a-login-hash' WHERE id=?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	policy.values.LDAPEnabled = false
	if _, err := auth.Login(ctx, users.LoginRequest{Email: entry.Email, Password: "synthetic-password"}); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatalf("LDAP local-password bypass: %v", err)
	}
	policy.values.LDAPEnabled = true
	recovery := NewPasswordRecoveryRepository(db)
	value := authusecase.PasswordRecovery{ID: uuid.NewString(), TokenHash: strings.Repeat("a", 64), TokenCiphertext: "synthetic", ExpiresAt: time.Now().Add(time.Hour)}
	if err := recovery.Request(ctx, user, value); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("password recovery allowed in LDAP-only mode", err)
	}
	var count int64
	if err := db.Table("password_recoveries").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("LDAP password reset queued: %d %v", count, err)
	}
	if err := db.Exec("UPDATE users SET ldap_disabled=true WHERE id=?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := auth.VerifyAuthorization(ctx, login.AccessToken); !errors.Is(err, apperrors.ErrUnauthorized) && !errors.Is(err, apperrors.ErrAccountBlocked) {
		t.Fatalf("disabled LDAP JWT accepted: %v", err)
	}
}
