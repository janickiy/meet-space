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
	domain "github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	adminusecase "github.com/janickiy/meet-space/internal/usecase/admin"
)

type adminTestPasswords struct{}

func (adminTestPasswords) Hash(v string) (string, error)    { return "hash:" + v, nil }
func (adminTestPasswords) Verify(v, h string) (bool, error) { return h == "hash:"+v, nil }
func TestAuditAdminSessionsAndAudit(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewAdminRepository(db)
	svc, err := adminusecase.NewService(repo, adminTestPasswords{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.Create(ctx, domain.CreateRequest{Username: "ADMIN", Name: "Owner", Password: "SyntheticPassword123"}, domain.AuditInput{}, true)
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.Create(ctx, domain.CreateRequest{Username: "replacement", Password: "OtherPassword123"}, domain.AuditInput{}, true)
	if err != nil || again.ID != first.ID || again.Username != "admin" {
		t.Fatal("bootstrap replaced existing administrator", err)
	}
	actor := domain.AuditInput{AdministratorID: first.ID, Username: first.Username, IP: "127.0.0.1"}
	second, err := svc.Create(ctx, domain.CreateRequest{Username: "Second", Name: "Second", Password: "SyntheticPassword123"}, actor, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Create(ctx, domain.CreateRequest{Username: "SECOND", Password: "SyntheticPassword123"}, actor, false); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("case collision accepted: %v", err)
	}
	expired := domain.Session{ID: uuid.NewString(), AdministratorID: first.ID, TokenHash: strings.Repeat("e", 64), CreatedAt: time.Now().Add(-24 * time.Hour), ExpiresAt: time.Now().Add(-time.Hour)}
	if err = db.Create(&expired).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Authenticate(ctx, expired.TokenHash); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("expired session accepted", err)
	}
	login, err := svc.Login(ctx, "second", "SyntheticPassword123", domain.AuditInput{IP: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	var expiredCount int64
	if err = db.Model(&domain.Session{}).Where("id=?", expired.ID).Count(&expiredCount).Error; err != nil || expiredCount != 0 {
		t.Fatal("expired session not cleaned on login", err)
	}
	if a, err := svc.Authenticate(ctx, login.Token); err != nil || a.ID != second.ID {
		t.Fatal("session invalid", err)
	}
	if _, err := svc.Authenticate(ctx, "a-user-jwt-is-not-an-admin-session"); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("public token accepted", err)
	}
	blocked := "blocked"
	if _, err = svc.Update(ctx, second.ID, domain.UpdateRequest{Status: &blocked}, actor); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Authenticate(ctx, login.Token); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("blocked session accepted", err)
	}
	if _, err = svc.Login(ctx, "second", "SyntheticPassword123", domain.AuditInput{IP: "127.0.0.1"}); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("blocked login accepted", err)
	}
	active := "active"
	if _, err = svc.Update(ctx, second.ID, domain.UpdateRequest{Status: &active}, actor); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Authenticate(ctx, login.Token); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("old session revived after unblock", err)
	}
	login, err = svc.Login(ctx, "second", "SyntheticPassword123", domain.AuditInput{IP: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Password(ctx, second.ID, "", "NewPassword12345", actor, false); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Authenticate(ctx, login.Token); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("old session survived password reset", err)
	}
	oldHash := second.PasswordHash
	if err = repo.CreateSession(ctx, domain.Session{ID: uuid.NewString(), AdministratorID: second.ID, TokenHash: strings.Repeat("a", 64), CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}, oldHash, actor); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("raced stale login accepted", err)
	}
	actor.Metadata = map[string]any{"password": "never-log-this", "token": "never-log-this", "status": "active"}
	actor.Action = "test.metadata"
	if err = repo.Audit(ctx, actor); err != nil {
		t.Fatal(err)
	}
	events, err := repo.AuditLog(ctx, domain.Query{PageSize: 100})
	if err != nil || len(events.Items) < 8 {
		t.Fatal("audit missing", err)
	}
	raw, _ := json.Marshal(events)
	if strings.Contains(string(raw), "never-log-this") || strings.Contains(string(raw), "SyntheticPassword") {
		t.Fatal("audit exposed credentials")
	}
	if err = db.Exec("UPDATE administrator_audit_log SET action='forged'").Error; err == nil {
		t.Fatal("audit allowed UPDATE")
	}
	if err = db.Exec("DELETE FROM administrator_audit_log").Error; err == nil {
		t.Fatal("audit allowed DELETE")
	}
	if err = db.Exec("TRUNCATE administrator_audit_log").Error; err == nil {
		t.Fatal("audit allowed TRUNCATE")
	}
	if err = svc.Delete(ctx, second.ID, actor); err != nil {
		t.Fatal("delete failed, audit FK must not mutate history", err)
	}
	if err = svc.Delete(ctx, first.ID, actor); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("self deletion allowed", err)
	}
}
func TestAuditAdminConcurrentLastActive(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		deletes [2]bool
	}{
		{"block_block", [2]bool{false, false}}, {"delete_delete", [2]bool{true, true}}, {"block_delete", [2]bool{false, true}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			db := auditDatabase(t)
			ctx := context.Background()
			repo := NewAdminRepository(db)
			accounts := []domain.Administrator{{ID: uuid.NewString(), Username: "first", Name: "First", Status: "active", PasswordHash: "synthetic"}, {ID: uuid.NewString(), Username: "second", Name: "Second", Status: "active", PasswordHash: "synthetic"}}
			for _, a := range accounts {
				if _, err := repo.Create(ctx, a, domain.AuditInput{}); err != nil {
					t.Fatal(err)
				}
			}
			blocked := "blocked"
			start := make(chan struct{})
			results := make(chan error, 2)
			var wg sync.WaitGroup
			for i, a := range accounts {
				wg.Add(1)
				go func(i int, a domain.Administrator) {
					defer wg.Done()
					<-start
					var err error
					if scenario.deletes[i] {
						err = repo.Delete(ctx, a.ID, domain.AuditInput{})
					} else {
						_, err = repo.Update(ctx, a.ID, domain.UpdateRequest{Status: &blocked}, domain.AuditInput{})
					}
					results <- err
				}(i, a)
			}
			close(start)
			wg.Wait()
			close(results)
			success, conflict := 0, 0
			for err := range results {
				if err == nil {
					success++
				} else if errors.Is(err, apperrors.ErrConflict) {
					conflict++
				} else {
					t.Fatal(err)
				}
			}
			if success != 1 || conflict != 1 {
				t.Fatalf("concurrent results success=%d conflict=%d", success, conflict)
			}
			var count int64
			if err := db.Model(&domain.Administrator{}).Where("status='active'").Count(&count).Error; err != nil || count != 1 {
				t.Fatal("last administrator lost", count, err)
			}
		})
	}
}
func TestAuditAdminUserBlockingAndRateLimit(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewAdminRepository(db)
	id := uuid.NewString()
	sid := uuid.NewString()
	if err := db.Exec("INSERT INTO users(id,email,password_hash,display_name) VALUES (?,?,?,?)", id, "user@example.invalid", "synthetic", "User").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO auth_sessions(id,user_id) VALUES (?,?)", sid, id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO auth_legacy_tokens(token_hash,user_id) VALUES (?,?)", strings.Repeat("b", 64), id).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetUserStatus(ctx, id, "blocked", domain.AuditInput{}); err != nil {
		t.Fatal(err)
	}
	var state struct {
		IsBlocked         bool
		AuthInvalidBefore *time.Time
	}
	if err := db.Table("users").Select("is_blocked,auth_invalid_before").Where("id=?", id).Scan(&state).Error; err != nil || !state.IsBlocked || state.AuthInvalidBefore == nil {
		t.Fatal("block state not durable", err)
	}
	var active int64
	if err := db.Table("auth_sessions").Where("user_id=? AND revoked_at IS NULL", id).Count(&active).Error; err != nil || active != 0 {
		t.Fatal("session not revoked", err)
	}
	if _, err := repo.SetUserStatus(ctx, id, "active", domain.AuditInput{}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 11; i++ {
		allowed, err := repo.LoginAllowed(ctx, "192.0.2.5", "target")
		if err != nil {
			t.Fatal(err)
		}
		if allowed != (i < 10) {
			t.Fatalf("rate counter attempt%d allowed=%v", i, allowed)
		}
	}
}
