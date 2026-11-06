package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/config"
	domain "github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/records"
	httptransport "github.com/janickiy/meet-space/internal/transport/http"
	adminusecase "github.com/janickiy/meet-space/internal/usecase/admin"
)

type delayedAdminBody struct {
	reader  io.Reader
	entered chan struct{}
	release chan struct{}
	waiting bool
}

func (b *delayedAdminBody) Read(p []byte) (int, error) {
	if !b.waiting {
		b.waiting = true
		close(b.entered)
		<-b.release
	}
	return b.reader.Read(p)
}
func (*delayedAdminBody) Close() error { return nil }

func TestAuditAdminRevokeDuringBodyCannotCreateAccount(t *testing.T) {
	for _, event := range []string{"logout", "block", "block-unblock", "password-reset", "delete", "expire"} {
		t.Run(event, func(t *testing.T) { testAdminRevocationDuringBody(t, event) })
	}
}

func testAdminRevocationDuringBody(t *testing.T, event string) {
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewAdminRepository(db)
	svc, err := adminusecase.NewService(repo, adminTestPasswords{})
	if err != nil {
		t.Fatal(err)
	}
	a, err := svc.Create(ctx, domain.CreateRequest{Username: "operator", Password: "SyntheticPassword123"}, domain.AuditInput{}, true)
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.Create(ctx, domain.CreateRequest{Username: "other", Password: "SyntheticPassword123"}, domain.AuditInput{}, false)
	if err != nil {
		t.Fatal(err)
	}
	login, err := svc.Login(ctx, a.Username, "SyntheticPassword123", domain.AuditInput{IP: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router, err := httptransport.NewAdminRouter(svc, config.AdminConfig{Enabled: true, PublicURL: "https://audit.invalid:8081"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := &delayedAdminBody{reader: strings.NewReader(`{"username":"unexpected","name":"Unexpected","password":"SyntheticPassword123"}`), entered: make(chan struct{}), release: make(chan struct{})}
	req := httptest.NewRequest("POST", domain.CookiePath+"/administrators", nil)
	req.Body = body
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://audit.invalid:8081")
	req.Header.Set("X-CSRF-Token", login.CSRFToken)
	req.AddCookie(&http.Cookie{Name: domain.CookieName, Value: login.Token})
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { router.ServeHTTP(rec, req); close(done) }()
	select {
	case <-body.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not reach body reader")
	}
	actor := domain.AuditInput{AdministratorID: other.ID, Username: other.Username}
	switch event {
	case "logout":
		err = svc.Logout(ctx, login.Token, domain.AuditInput{AdministratorID: a.ID, Username: a.Username})
	case "block", "block-unblock":
		blocked := "blocked"
		_, err = svc.Update(ctx, a.ID, domain.UpdateRequest{Status: &blocked}, actor)
		if err == nil && event == "block-unblock" {
			active := "active"
			_, err = svc.Update(ctx, a.ID, domain.UpdateRequest{Status: &active}, actor)
		}
	case "password-reset":
		err = svc.Password(ctx, a.ID, "", "ChangedPassword123", actor, false)
	case "delete":
		err = svc.Delete(ctx, a.ID, actor)
	case "expire":
		err = db.Model(&domain.Session{}).Where("administrator_id=?", a.ID).Update("expires_at", time.Now().Add(-time.Second)).Error
	}
	close(body.release)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not finish")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked request created administrator: HTTP %d", rec.Code)
	}
	if _, err = repo.ByUsername(ctx, "unexpected"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("revoked session persisted a new identity", err)
	}
}

func TestAuditAdminCanonicalSelfMutationGuards(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewAdminRepository(db)
	const self = "aabbccdd-1122-4334-8556-aabbccddeeff"
	for _, a := range []domain.Administrator{{ID: self, Username: "first", Name: "First", Status: "active", PasswordHash: "hash:SyntheticPassword123"}, {ID: uuid.NewString(), Username: "other", Name: "Other", Status: "active", PasswordHash: "hash:SyntheticPassword123"}} {
		if _, err := repo.Create(ctx, a, domain.AuditInput{}); err != nil {
			t.Fatal(err)
		}
	}
	audit := domain.AuditInput{AdministratorID: self, Username: "first"}
	blocked := "blocked"
	for _, alias := range []string{strings.ToUpper(self), "{" + self + "}"} {
		if _, err := repo.Update(ctx, alias, domain.UpdateRequest{Status: &blocked}, audit); !errors.Is(err, apperrors.ErrConflict) {
			t.Fatalf("self block bypass: %v", err)
		}
		if err := repo.Delete(ctx, alias, audit); !errors.Is(err, apperrors.ErrConflict) {
			t.Fatalf("self delete bypass: %v", err)
		}
		if err := repo.ChangePassword(ctx, alias, "new-hash", "", audit); !errors.Is(err, apperrors.ErrUnauthorized) {
			t.Fatalf("self password bypass: %v", err)
		}
	}
}

func TestAuditAdminLoginPreservesDatabaseTimeout(t *testing.T) {
	db := auditDatabase(t)
	repo := NewAdminRepository(db)
	ctx := context.Background()
	a := domain.Administrator{ID: uuid.NewString(), Username: "locked", Name: "Locked", PasswordHash: "synthetic", Status: "active"}
	if _, err := repo.Create(ctx, a, domain.AuditInput{}); err != nil {
		t.Fatal(err)
	}
	held := db.Begin()
	if held.Error != nil {
		t.Fatal(held.Error)
	}
	defer held.Rollback()
	if err := held.Exec("SELECT id FROM administrators WHERE id=? FOR UPDATE", a.ID).Error; err != nil {
		t.Fatal(err)
	}
	bounded, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	err := repo.CreateSession(bounded, domain.Session{ID: uuid.NewString(), AdministratorID: a.ID, TokenHash: strings.Repeat("f", 64), CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}, a.PasswordHash, domain.AuditInput{})
	if errors.Is(err, apperrors.ErrUnauthorized) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("database timeout misclassified as authentication failure: %v", err)
	}
}

func TestAuditAdminLoginCleanupSkipsLockedExpiredSessions(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewAdminRepository(db)
	owner := domain.Administrator{ID: uuid.NewString(), Username: "owner", Name: "Owner", PasswordHash: "synthetic", Status: "active"}
	login := domain.Administrator{ID: uuid.NewString(), Username: "login", Name: "Login", PasswordHash: "synthetic", Status: "active"}
	for _, a := range []domain.Administrator{owner, login} {
		if _, err := repo.Create(ctx, a, domain.AuditInput{}); err != nil {
			t.Fatal(err)
		}
	}
	expired := domain.Session{ID: uuid.NewString(), AdministratorID: owner.ID, TokenHash: strings.Repeat("d", 64), CreatedAt: time.Now().Add(-24 * time.Hour), ExpiresAt: time.Now().Add(-time.Hour)}
	if err := db.Create(&expired).Error; err != nil {
		t.Fatal(err)
	}
	held := db.Begin()
	if held.Error != nil {
		t.Fatal(held.Error)
	}
	defer held.Rollback()
	if err := held.Exec("SELECT id FROM administrator_sessions WHERE id=? FOR UPDATE", expired.ID).Error; err != nil {
		t.Fatal(err)
	}
	bounded, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	err := repo.CreateSession(bounded, domain.Session{ID: uuid.NewString(), AdministratorID: login.ID, TokenHash: strings.Repeat("c", 64), CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}, login.PasswordHash, domain.AuditInput{})
	if err != nil {
		t.Fatalf("login blocked on unrelated expired session cleanup: %v", err)
	}
}

// Verification pauses after Login has read the previous account state. The
// subsequent transaction must reject that stale state after a security change.
type delayedAdminPasswords struct {
	entered chan struct{}
	release chan struct{}
}

func (p delayedAdminPasswords) Hash(v string) (string, error) { return "hash:" + v, nil }
func (p delayedAdminPasswords) Verify(v, h string) (bool, error) {
	close(p.entered)
	<-p.release
	return h == "hash:"+v, nil
}
func TestAuditAdminLoginCannotSurviveSecurityChange(t *testing.T) {
	for _, event := range []string{"password-reset", "block", "delete"} {
		t.Run(event, func(t *testing.T) {
			db := auditDatabase(t)
			ctx := context.Background()
			repo := NewAdminRepository(db)
			p := delayedAdminPasswords{entered: make(chan struct{}), release: make(chan struct{})}
			svc, err := adminusecase.NewService(repo, p)
			if err != nil {
				t.Fatal(err)
			}
			a, err := svc.Create(ctx, domain.CreateRequest{Username: "operator", Password: "SyntheticPassword123"}, domain.AuditInput{}, true)
			if err != nil {
				t.Fatal(err)
			}
			other, err := svc.Create(ctx, domain.CreateRequest{Username: "other", Password: "SyntheticPassword123"}, domain.AuditInput{}, false)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				_, e := svc.Login(ctx, a.Username, "SyntheticPassword123", domain.AuditInput{IP: "192.0.2.1"})
				done <- e
			}()
			select {
			case <-p.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("login did not reach password verification")
			}
			actor := domain.AuditInput{AdministratorID: other.ID, Username: other.Username}
			switch event {
			case "password-reset":
				err = svc.Password(ctx, a.ID, "", "ChangedPassword123", actor, false)
			case "block":
				blocked := "blocked"
				_, err = svc.Update(ctx, a.ID, domain.UpdateRequest{Status: &blocked}, actor)
			case "delete":
				err = svc.Delete(ctx, a.ID, actor)
			}
			close(p.release)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case err = <-done:
				if !errors.Is(err, apperrors.ErrUnauthorized) {
					t.Fatalf("stale login accepted: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("login remained blocked after password work finished")
			}
			var count int64
			if err = db.Model(&domain.Session{}).Where("administrator_id=?", a.ID).Count(&count).Error; err != nil || count != 0 {
				t.Fatal("stale login persisted a session", err)
			}
		})
	}
}

func TestAuditAdminConcurrentLogoutDoesNotDeadlock(t *testing.T) {
	db := auditDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	repo := NewAdminRepository(db)
	svc, err := adminusecase.NewService(repo, adminTestPasswords{})
	if err != nil {
		t.Fatal(err)
	}
	a, err := svc.Create(ctx, domain.CreateRequest{Username: "operator", Password: "SyntheticPassword123"}, domain.AuditInput{}, true)
	if err != nil {
		t.Fatal(err)
	}
	login, err := svc.Login(ctx, a.Username, "SyntheticPassword123", domain.AuditInput{IP: "192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx = domain.WithAuthorization(ctx, a.ID, adminusecase.HashToken(login.Token))
	const requests = 8
	results := make(chan error, requests)
	start := make(chan struct{})
	for i := 0; i < requests; i++ {
		go func() {
			<-start
			results <- svc.Logout(ctx, login.Token, domain.AuditInput{AdministratorID: a.ID, Username: a.Username})
		}()
	}
	close(start)
	success, revoked := 0, 0
	for i := 0; i < requests; i++ {
		switch err := <-results; {
		case err == nil:
			success++
		case errors.Is(err, apperrors.ErrUnauthorized):
			revoked++
		default:
			t.Fatalf("concurrent logout failed or deadlocked: %v", err)
		}
	}
	if success != 1 || revoked != requests-1 {
		t.Fatalf("logout results success=%d revoked=%d", success, revoked)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if n := sqlDB.Stats().InUse; n != 0 {
		t.Fatalf("logout retained %d DB connections", n)
	}
}

func TestAuditAdminCancelledMutationReleasesLocks(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewAdminRepository(db)
	held := db.Begin()
	if held.Error != nil {
		t.Fatal(held.Error)
	}
	defer held.Rollback()
	if err := adminLock(held); err != nil {
		t.Fatal(err)
	}
	bounded, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	a := domain.Administrator{ID: uuid.NewString(), Username: "operator", Name: "Operator", PasswordHash: "synthetic", Status: "active"}
	if _, err := repo.Create(bounded, a, domain.AuditInput{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("mutation ignored cancellation: %v", err)
	}
	if err := held.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, a, domain.AuditInput{}); err != nil {
		t.Fatalf("canceled mutation retained lock or account: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if n := sqlDB.Stats().InUse; n != 0 {
		t.Fatalf("canceled mutation retained %d DB connections", n)
	}
}

func TestAuditAdminLoginLimitRotationBounded(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewAdminRepository(db)
	for i := 0; i < 100; i++ {
		allowed, err := repo.LoginAllowed(ctx, "192.0.2.1", uuid.NewString())
		if err != nil {
			t.Fatal(err)
		}
		if allowed != (i < 30) {
			t.Fatalf("IP rotation limit attempt=%d allowed=%v", i, allowed)
		}
	}
	var buckets int64
	if err := db.Table("administrator_login_limits").Count(&buckets).Error; err != nil || buckets != 31 {
		t.Fatalf("username rotation grew rate buckets: count=%d err=%v", buckets, err)
	}
	t.Log("100 rotating usernames from one IP retained exactly 31 rate buckets (1 IP + 30 usernames)")
}

func TestAuditAdminLoginLimitCleanupSkipsLockedRows(t *testing.T) {
	db := auditDatabase(t)
	repo := NewAdminRepository(db)
	const key = "unrelated-expired-bucket"
	if err := db.Exec("INSERT INTO administrator_login_limits(key_hash,window_start,attempts) VALUES (?,now()-interval '2 days',1)", key).Error; err != nil {
		t.Fatal(err)
	}
	held := db.Begin()
	if held.Error != nil {
		t.Fatal(held.Error)
	}
	defer held.Rollback()
	if err := held.Exec("SELECT key_hash FROM administrator_login_limits WHERE key_hash=? FOR UPDATE", key).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if allowed, err := repo.LoginAllowed(ctx, "192.0.2.1", "operator"); err != nil || !allowed {
		t.Fatalf("rate cleanup blocked unrelated login: allowed=%v err=%v", allowed, err)
	}
}

func TestAuditAdminSessionCountIsBounded(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewAdminRepository(db)
	a := domain.Administrator{ID: uuid.NewString(), Username: "operator", Name: "Operator", PasswordHash: "synthetic", Status: "active"}
	if _, err := repo.Create(ctx, a, domain.AuditInput{}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		s := domain.Session{ID: uuid.NewString(), AdministratorID: a.ID, TokenHash: adminusecase.HashToken(uuid.NewString()), CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
		if err := repo.CreateSession(ctx, s, a.PasswordHash, domain.AuditInput{}); err != nil {
			t.Fatal(err)
		}
	}
	var sessions int64
	if err := db.Model(&domain.Session{}).Count(&sessions).Error; err != nil || sessions != 20 {
		t.Fatalf("session cap exceeded: count=%d err=%v", sessions, err)
	}
	t.Log("25 successful session creations retained exactly 20 sessions")
}

type auditFailureResources struct {
	httptransport.AdminResources
	repo *AdminRepository
}

func (r auditFailureResources) DeleteRecording(ctx context.Context, id string, audit domain.AuditInput) error {
	_, err := r.repo.DeleteAdminRecording(ctx, id, audit, func(context.Context, records.Record) error { return errors.New("private-storage-error-never-log") })
	return err
}
func TestAuditAdminFailedMutationsAreRecordedWithoutSecrets(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewAdminRepository(db)
	svc, err := adminusecase.NewService(repo, adminTestPasswords{})
	if err != nil {
		t.Fatal(err)
	}
	a, err := svc.Create(ctx, domain.CreateRequest{Username: "operator", Password: "SyntheticPassword123"}, domain.AuditInput{}, true)
	if err != nil {
		t.Fatal(err)
	}
	login, err := svc.Login(ctx, a.Username, "SyntheticPassword123", domain.AuditInput{IP: "192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	recordID := uuid.NewString()
	if err := db.Exec("INSERT INTO record(uuid,conference_id,mode,source_type,transport_type,status,quality_mode) VALUES(?,?,'legacy','conference','webrtc','ready','auto')", recordID, uuid.NewString()).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router, err := httptransport.NewAdminRouter(svc, config.AdminConfig{Enabled: true, PublicURL: "https://audit.invalid:8081"}, auditFailureResources{repo: repo}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path, body, action string
		status                     int
	}{
		{"POST", "/administrators", `{"username":"operator","password":"SyntheticPassword123"}`, "administrator.create_failed", 409},
		{"POST", "/auth/password", `{"currentPassword":"PrivateWrongPassword123","password":"PrivateNewPassword123"}`, "administrator.password_failed", 401},
		{"DELETE", "/recordings/" + recordID, `{}`, "recording.delete_failed", 500},
	} {
		req := httptest.NewRequest(tc.method, domain.CookiePath+tc.path+"?secret=never-capture-query", strings.NewReader(tc.body))
		req.Header.Set("Origin", "https://audit.invalid:8081")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", login.CSRFToken)
		req.AddCookie(&http.Cookie{Name: domain.CookieName, Value: login.Token})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("%s response=%d expected=%d", tc.action, rec.Code, tc.status)
		}
		events, err := repo.AuditLog(ctx, domain.Query{Action: tc.action})
		if err != nil || events.Total != 1 {
			t.Fatalf("%s failure not durably audited: count=%d err=%v", tc.action, events.Total, err)
		}
		var metadata map[string]any
		if err := json.Unmarshal(events.Items[0].Metadata, &metadata); err != nil || len(metadata) != 1 || metadata["httpStatus"] != float64(tc.status) {
			t.Fatalf("unexpected failure metadata: %v", err)
		}
	}
	all, err := repo.AuditLog(ctx, domain.Query{PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(all)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{login.Token, login.CSRFToken, "SyntheticPassword123", "PrivateWrongPassword123", "PrivateNewPassword123", "private-storage-error-never-log", "never-capture-query"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("failure audit exposed sensitive input")
		}
	}
	var successful int64
	if err := db.Table("administrator_audit_log").Where("action='recording.delete'").Count(&successful).Error; err != nil || successful != 0 {
		t.Fatal("rolled-back deletion incorrectly logged as successful", err)
	}
}
