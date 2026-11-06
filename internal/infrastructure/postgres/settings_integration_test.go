package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/settings"
	"github.com/janickiy/meet-space/internal/infrastructure/security"
	httptransport "github.com/janickiy/meet-space/internal/transport/http"
	adminusecase "github.com/janickiy/meet-space/internal/usecase/admin"
	settingsusecase "github.com/janickiy/meet-space/internal/usecase/settings"
	"gorm.io/gorm"
)

func settingsFixture(t *testing.T) (*gorm.DB, *settingsusecase.Service, context.Context, admin.AuditInput, string) {
	t.Helper()
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewAdminRepository(db)
	a, err := repo.Bootstrap(ctx, admin.Administrator{ID: uuid.NewString(), Username: "operator", Name: "Operator", PasswordHash: "synthetic", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	if err := repo.CreateSession(ctx, admin.Session{ID: uuid.NewString(), AdministratorID: a.ID, TokenHash: hash, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}, a.PasswordHash, admin.AuditInput{}); err != nil {
		t.Fatal(err)
	}
	cipher, err := security.NewProviderTokens(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("s", 32))))
	if err != nil {
		t.Fatal(err)
	}
	defaults := domain.Values{AttachmentMaxBytes: 10 << 20, MediaMaxPeers: 10, MediaMaxRooms: 100, RecordingRetentionDays: 7, SMTPPort: 465, SMTPTLSMode: "tls"}
	svc := settingsusecase.NewService(NewSettingsRepository(db), defaults, "", cipher, "https://meet.example.invalid")
	return db, svc, admin.WithAuthorization(ctx, a.ID, hash), admin.AuditInput{AdministratorID: a.ID, Username: a.Username}, hash
}
func settingsPatch(t *testing.T, raw string) domain.Patch {
	t.Helper()
	input, err := settingsusecase.DecodePatch(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	return input
}
func TestAuditPlatformSettingsPersistEncryptedAndAuditKeysOnly(t *testing.T) {
	db, svc, ctx, actor, _ := settingsFixture(t)
	initial, err := svc.Admin(ctx)
	if err != nil || initial.Revision != 0 {
		t.Fatal("missing settings defaults", err)
	}
	view, err := svc.Update(ctx, settingsPatch(t, `{"revision":0,"changes":{"attachmentMaxBytes":20971520,"smtpEnabled":true,"smtpHost":"smtp.example.invalid","smtpUsername":"sender","smtpPassword":"PrivateSyntheticSMTPValue","smtpFrom":"sender@example.invalid","emailVerificationRequired":true}}`), actor)
	if err != nil || view.Revision != 1 {
		t.Fatal("settings update failed", err)
	}
	state, err := NewSettingsRepository(db).Load(context.Background())
	if err != nil || state.Revision != 1 || state.SMTPPasswordCiphertext == "" {
		t.Fatal("settings not persisted", err)
	}
	if strings.Contains(string(state.Overrides), "PrivateSyntheticSMTPValue") || strings.Contains(state.SMTPPasswordCiphertext, "PrivateSyntheticSMTPValue") || strings.Contains(string(state.Overrides), "smtpPassword") {
		t.Fatal("plaintext password persisted")
	}
	read, err := svc.Snapshot(ctx)
	if err != nil || read.AttachmentMaxBytes != 20971520 || !read.EmailVerificationRequired {
		t.Fatal("saved values are not effective", err)
	}
	credentials, err := svc.SMTP(ctx)
	if err != nil || credentials.Password != "PrivateSyntheticSMTPValue" {
		t.Fatal("encrypted credential not usable", err)
	}
	ciphertext := state.SMTPPasswordCiphertext
	if _, err := svc.Update(ctx, settingsPatch(t, `{"revision":1,"changes":{"smtpPassword":"","smtpPort":587,"smtpTLSMode":"starttls"}}`), actor); err != nil {
		t.Fatal(err)
	}
	state, err = NewSettingsRepository(db).Load(ctx)
	if err != nil || state.SMTPPasswordCiphertext != ciphertext {
		t.Fatal("blank password replaced existing ciphertext", err)
	}
	events, err := NewAdminRepository(db).AuditLog(ctx, admin.Query{Action: "settings.update"})
	if err != nil || events.Total != 2 {
		t.Fatal("missing settings audit", err)
	}
	for _, event := range events.Items {
		var metadata map[string]any
		if err := json.Unmarshal(event.Metadata, &metadata); err != nil || len(metadata) != 1 {
			t.Fatal("unexpected audit metadata")
		}
		if _, ok := metadata["fields"].([]any); !ok {
			t.Fatal("audit did not retain only changed keys")
		}
	}
	encoded, _ := json.Marshal([]any{view, events})
	for _, forbidden := range []string{"PrivateSyntheticSMTPValue", ciphertext, "smtp.example.invalid", "sender@example.invalid"} {
		// Public host/from are allowed in settings, but never in audit metadata.
		if forbidden == "PrivateSyntheticSMTPValue" || forbidden == ciphertext {
			if strings.Contains(string(encoded), forbidden) {
				t.Fatal("secret exposed by JSON")
			}
		} else {
			auditJSON, _ := json.Marshal(events)
			if strings.Contains(string(auditJSON), forbidden) {
				t.Fatal("audit contains setting values")
			}
		}
	}
}
func TestAuditPlatformSettingsConcurrentRevisionOnlyOneWins(t *testing.T) {
	db, svc, ctx, actor, _ := settingsFixture(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, raw := range []string{`{"revision":0,"changes":{"mediaMaxPeers":20}}`, `{"revision":0,"changes":{"mediaMaxPeers":30}}`} {
		input := settingsPatch(t, raw)
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, err := svc.Update(ctx, input, actor); results <- err }()
	}
	close(start)
	wg.Wait()
	close(results)
	ok, conflicts := 0, 0
	for err := range results {
		if err == nil {
			ok++
		} else if errors.Is(err, apperrors.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || conflicts != 1 {
		t.Fatalf("concurrent saves success=%d conflict=%d", ok, conflicts)
	}
	state, err := NewSettingsRepository(db).Load(ctx)
	if err != nil || state.Revision != 1 {
		t.Fatal("revision incremented incorrectly", err)
	}
	events, err := NewAdminRepository(db).AuditLog(ctx, admin.Query{Action: "settings.update"})
	if err != nil || events.Total != 1 {
		t.Fatal("conflicting transaction wrote audit", err)
	}
}
func TestAuditPlatformSettingsRevokedSessionAndAuditFailureRollback(t *testing.T) {
	db, svc, ctx, actor, hash := settingsFixture(t)
	if err := NewAdminRepository(db).RevokeSession(context.Background(), hash, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, settingsPatch(t, `{"revision":0,"changes":{"mediaMaxPeers":20}}`), actor); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("revoked administrator changed settings", err)
	}
	state, err := NewSettingsRepository(db).Load(context.Background())
	if err != nil || state.Revision != 0 {
		t.Fatal("revoked write persisted", err)
	}
	if err := db.Exec(`CREATE FUNCTION reject_settings_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='settings.update' THEN RAISE EXCEPTION 'synthetic audit failure'; END IF; RETURN NEW; END; $$`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_settings_audit BEFORE INSERT ON administrator_audit_log FOR EACH ROW EXECUTE FUNCTION reject_settings_audit()`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(context.Background(), settingsPatch(t, `{"revision":0,"changes":{"mediaMaxPeers":20}}`), actor); err == nil {
		t.Fatal("audit failure ignored")
	}
	state, err = NewSettingsRepository(db).Load(context.Background())
	if err != nil || state.Revision != 0 || string(state.Overrides) != "{}" {
		t.Fatal("failed audit left partially saved settings", err)
	}
}

type settingsHTTPResources struct {
	httptransport.AdminResources
	service *settingsusecase.Service
}

func (r settingsHTTPResources) Query(ctx context.Context, _ string, _ admin.Query) (any, error) {
	return r.service.Admin(ctx)
}
func (r settingsHTTPResources) UpdateSettings(ctx context.Context, raw json.RawMessage, audit admin.AuditInput) (any, error) {
	input, err := settingsusecase.DecodePatch(raw)
	if err != nil {
		return nil, err
	}
	return r.service.Update(ctx, input, audit)
}
func TestAuditPlatformSettingsHTTPContract(t *testing.T) {
	db, svc, ctx, actor, _ := settingsFixture(t)
	token := strings.Repeat("A", 43)
	adminRepo := NewAdminRepository(db)
	if err := adminRepo.CreateSession(context.Background(), admin.Session{ID: uuid.NewString(), AdministratorID: actor.AdministratorID, TokenHash: adminusecase.HashToken(token), CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}, "synthetic", actor); err != nil {
		t.Fatal(err)
	}
	auth, err := adminusecase.NewService(adminRepo, adminTestPasswords{})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router, err := httptransport.NewAdminRouter(auth, config.AdminConfig{Enabled: true, PublicURL: "https://settings.invalid:8081"}, settingsHTTPResources{service: svc}, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, body, csrf string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, admin.CookiePath+"/settings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://settings.invalid:8081")
		req.Header.Set("X-CSRF-Token", csrf)
		req.AddCookie(&http.Cookie{Name: admin.CookieName, Value: token})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	get := request("GET", "", "")
	if get.Code != 200 {
		t.Fatal("GET settings failed", get.Code)
	}
	var view admin.Settings
	if err := json.Unmarshal(get.Body.Bytes(), &view); err != nil || view.Revision != 0 {
		t.Fatal("unexpected settings schema", err)
	}
	if res := request("PATCH", `{"revision":0,"changes":{"mediaMaxPeers":20}}`, ""); res.Code != 403 {
		t.Fatal("missing CSRF accepted", res.Code)
	}
	csrf := adminusecase.CSRFToken(token)
	if res := request("PATCH", `{"revision":0,"changes":{"registration":false}}`, csrf); res.Code != 400 {
		t.Fatal("read-only settings edited", res.Code)
	}
	if res := request("PATCH", `{"revision":0,"changes":{"mediaMaxPeers":20}}`, csrf); res.Code != 200 {
		t.Fatal("settings PATCH failed", res.Code)
	}
	if res := request("PATCH", `{"revision":0,"changes":{"mediaMaxPeers":30}}`, csrf); res.Code != 409 {
		t.Fatal("stale settings accepted", res.Code)
	}
	values, err := svc.Snapshot(ctx)
	if err != nil || values.Revision != 1 || values.MediaMaxPeers != 20 {
		t.Fatal("HTTP settings not effective", err)
	}
}
