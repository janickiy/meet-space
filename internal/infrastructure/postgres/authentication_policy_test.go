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
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/settings"
	"github.com/janickiy/meet-space/internal/domain/users"
	authusecase "github.com/janickiy/meet-space/internal/usecase/auth"
	"gorm.io/gorm"
)

func TestAuthenticationModeSwitchRevokesUserSessionsAndPreservesAdmin(t *testing.T) {
	db, _, ctx, actor, _ := settingsFixture(t)
	repo, sessions := NewUserRepository(db), NewAuthSessionRepository(db)
	user, err := repo.Create(ctx, users.User{ID: uuid.NewString(), Email: "mode@example.invalid", PasswordHash: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	newSession := func() users.AuthSession {
		return users.AuthSession{ID: uuid.NewString(), UserID: user.ID, TokenHash: strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", ""), CreatedAt: time.Now().UTC(), ExpectedAuthSource: "local", ExpectedPasswordHash: user.PasswordHash, ExpectedAuthVersion: user.AuthVersion}
	}
	session := newSession()
	if err = sessions.Create(ctx, session); err != nil {
		t.Fatal(err)
	}
	if _, err = sessions.GetByHash(ctx, session.TokenHash); err != nil {
		t.Fatal(err)
	}
	legacy := strings.Repeat("c", 64)
	if err = db.Create(&authLegacyToken{TokenHash: legacy, UserID: user.ID}).Error; err != nil {
		t.Fatal(err)
	}
	settingsRepo := NewSettingsRepository(db)
	// Используем настоящий репозиторий с авторизацией администратора и аудитом.
	if _, err = settingsRepo.Save(ctx, settings.SaveInput{ExpectedRevision: 0, Overrides: json.RawMessage(`{"ldapEnabled":true}`), Fields: []string{"ldapEnabled"}}, actor); err != nil {
		t.Fatal(err)
	}
	var stored users.AuthSession
	if err = db.Take(&stored, "id=?", session.ID).Error; err != nil || stored.RevokedAt == nil {
		t.Fatal("local session was not revoked", err)
	}
	if _, err = sessions.GetByHash(ctx, session.TokenHash); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("local refresh still usable in LDAP mode", err)
	}
	if _, err = sessions.GetByID(ctx, session.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("local access still usable in LDAP mode", err)
	}
	if err = sessions.Create(ctx, newSession()); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("stale local login crossed mode switch", err)
	}
	if err = sessions.AuthorizeLegacy(ctx, legacy, user.ID); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("legacy local token crossed mode switch", err)
	}
	// Обратное переключение доступно прежней административной сессии.
	if _, err = settingsRepo.Save(ctx, settings.SaveInput{ExpectedRevision: 1, Overrides: json.RawMessage(`{"ldapEnabled":false}`), Fields: []string{"ldapEnabled"}}, actor); err != nil {
		t.Fatal("admin lost access after mode switch", err)
	}
	if _, err = sessions.GetByHash(ctx, session.TokenHash); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("old session resurrected", err)
	}
	if err = sessions.AuthorizeLegacy(ctx, legacy, user.ID); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("legacy token resurrected", err)
	}
	user, err = repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = sessions.Create(ctx, newSession()); err != nil {
		t.Fatal("fresh local login rejected after switching back", err)
	}
}

func TestAuthenticationModeGuardsLocalRegistrationAndRecoveryTransactions(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	usersRepo := NewUserRepository(db)
	user, err := usersRepo.Create(ctx, users.User{ID: uuid.NewString(), Email: "recover-mode@example.invalid", PasswordHash: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	recoveryRepo, verificationRepo := NewPasswordRecoveryRepository(db), NewEmailVerificationRepository(db)
	recovery := authusecase.PasswordRecovery{ID: uuid.NewString(), UserID: user.ID, TokenHash: strings.Repeat("d", 64), TokenCiphertext: "synthetic", ExpiresAt: time.Now().Add(time.Hour)}
	if err = recoveryRepo.Request(ctx, user, recovery); err != nil {
		t.Fatal(err)
	}
	pending := users.User{ID: uuid.NewString(), Email: "pending-mode@example.invalid", PasswordHash: "synthetic", EmailVerificationRequired: true}
	verification := authusecase.Verification{ID: uuid.NewString(), UserID: pending.ID, TokenHash: strings.Repeat("e", 64), TokenCiphertext: "synthetic", ExpiresAt: time.Now().Add(time.Hour)}
	if _, err = verificationRepo.CreatePending(ctx, pending, verification); err != nil {
		t.Fatal(err)
	}
	jobs := NewJobRepository(db)
	recoveryJob, found, err := jobs.Claim(ctx, authusecase.PasswordRecoveryJobKind, time.Minute)
	if err != nil || !found {
		t.Fatal("recovery job missing", err)
	}
	verificationJob, found, err := jobs.Claim(ctx, authusecase.VerificationJobKind, time.Minute)
	if err != nil || !found {
		t.Fatal("verification job missing", err)
	}
	if err = db.Exec(`UPDATE platform_settings SET values='{"ldapEnabled":true}'::jsonb WHERE singleton`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = usersRepo.Create(ctx, users.User{ID: uuid.NewString(), Email: "blocked-mode@example.invalid", PasswordHash: "synthetic"}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("local account created in LDAP mode", err)
	}
	if err = recoveryRepo.Request(ctx, user, recovery); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("local recovery requested in LDAP mode", err)
	}
	if _, err = recoveryRepo.Consume(ctx, recovery.TokenHash, "changed"); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("local password changed in LDAP mode", err)
	}
	if _, err = recoveryRepo.Delivery(ctx, recoveryJob); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("queued recovery delivered in LDAP mode", err)
	}
	if _, err = verificationRepo.CreatePending(ctx, pending, verification); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("pending account created in LDAP mode", err)
	}
	if err = verificationRepo.Resend(ctx, pending, verification); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("verification resent in LDAP mode", err)
	}
	if err = verificationRepo.Consume(ctx, verification.TokenHash); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("local email verified in LDAP mode", err)
	}
	if _, err = verificationRepo.Delivery(ctx, verificationJob); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("queued verification delivered in LDAP mode", err)
	}
	current, err := usersRepo.GetByID(ctx, user.ID)
	if err != nil || current.PasswordHash != "synthetic" {
		t.Fatal("denied reset mutated user", err)
	}
	current, err = usersRepo.GetByID(ctx, pending.ID)
	if err != nil || current.EmailVerifiedAt != nil {
		t.Fatal("denied verification mutated user", err)
	}
}

// Одновременный выход по старому JWT и смена режима раньше брали legacy/SID
// в обратном порядке. Удерживаем SID, чтобы воспроизвести конфликт без случайных задержек.
func TestAuthenticationModeSwitchAndLegacyLogoutDoNotDeadlock(t *testing.T) {
	db, _, baseCtx, actor, _ := settingsFixture(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(6)
	ctx, cancel := context.WithTimeout(baseCtx, 8*time.Second)
	defer cancel()
	user, err := NewUserRepository(db).Create(ctx, users.User{ID: uuid.NewString(), Email: "mode-race@example.invalid", PasswordHash: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	session := users.AuthSession{ID: uuid.NewString(), UserID: user.ID, TokenHash: strings.Repeat("f", 64), CreatedAt: time.Now().UTC(), ExpectedAuthSource: "local", ExpectedPasswordHash: user.PasswordHash}
	if err = NewAuthSessionRepository(db).Create(ctx, session); err != nil {
		t.Fatal(err)
	}
	legacyHash := strings.Repeat("9", 64)
	if err = db.Create(&authLegacyToken{TokenHash: legacyHash, UserID: user.ID, SessionID: &session.ID}).Error; err != nil {
		t.Fatal(err)
	}
	blocker := db.WithContext(ctx).Begin()
	if blocker.Error != nil {
		t.Fatal(blocker.Error)
	}
	var workers sync.WaitGroup
	defer func() { cancel(); _ = blocker.Rollback().Error; workers.Wait() }()
	if err = blocker.Exec("SELECT id FROM auth_sessions WHERE id=? FOR UPDATE", session.ID).Error; err != nil {
		t.Fatal(err)
	}

	savePID, logoutPID := make(chan int, 1), make(chan int, 1)
	saveDone, logoutDone := make(chan error, 1), make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		saveDone <- db.WithContext(ctx).Connection(func(conn *gorm.DB) error {
			var pid int
			if err := conn.Raw("SELECT pg_backend_pid()").Scan(&pid).Error; err != nil {
				return err
			}
			savePID <- pid
			_, err := NewSettingsRepository(conn).Save(ctx, settings.SaveInput{ExpectedRevision: 0, Overrides: json.RawMessage(`{"ldapEnabled":true}`), Fields: []string{"ldapEnabled"}}, actor)
			return err
		})
	}()
	var pid int
	select {
	case pid = <-savePID:
	case err := <-saveDone:
		t.Fatalf("settings save failed before obtaining connection: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitAuthenticationDatabaseLock(t, ctx, db, pid)

	workers.Add(1)
	go func() {
		defer workers.Done()
		logoutDone <- db.WithContext(ctx).Connection(func(conn *gorm.DB) error {
			var pid int
			if err := conn.Raw("SELECT pg_backend_pid()").Scan(&pid).Error; err != nil {
				return err
			}
			logoutPID <- pid
			return NewAuthSessionRepository(conn).RevokeLegacy(ctx, legacyHash, user.ID)
		})
	}()
	select {
	case pid = <-logoutPID:
	case err := <-logoutDone:
		t.Fatalf("logout failed before obtaining connection: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitAuthenticationDatabaseLock(t, ctx, db, pid)
	if err = blocker.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	for _, result := range []struct {
		name string
		done <-chan error
	}{{"mode switch", saveDone}, {"legacy logout", logoutDone}} {
		select {
		case err := <-result.done:
			if err != nil {
				t.Fatalf("%s failed: %v", result.name, err)
			}
		case <-ctx.Done():
			t.Fatalf("%s did not finish: %v", result.name, ctx.Err())
		}
	}
	var legacy authLegacyToken
	if err = db.Take(&legacy, "token_hash=?", legacyHash).Error; err != nil || legacy.RevokedAt == nil {
		t.Fatal("legacy token remained active", err)
	}
	var stored users.AuthSession
	if err = db.Take(&stored, "id=?", session.ID).Error; err != nil || stored.RevokedAt == nil {
		t.Fatal("linked session remained active", err)
	}
}

func waitAuthenticationDatabaseLock(t *testing.T, ctx context.Context, db *gorm.DB, pid int) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := db.WithContext(ctx).Raw("SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=? AND wait_event_type='Lock')", pid).Scan(&waiting).Error; err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("transaction did not reach expected row lock:", ctx.Err())
		}
	}
}
