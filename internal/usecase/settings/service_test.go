package settings

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/settings"
	"github.com/janickiy/meet-space/internal/infrastructure/security"
)

type testRepository struct {
	state  domain.State
	err    error
	saves  int
	fields []string
}

func (r *testRepository) Load(context.Context) (domain.State, error) { return r.state, r.err }
func (r *testRepository) Save(_ context.Context, in domain.SaveInput, _ admin.AuditInput) (domain.State, error) {
	if r.state.Revision != in.ExpectedRevision {
		return domain.State{}, apperrors.ErrConflict
	}
	r.saves++
	r.fields = in.Fields
	r.state = domain.State{Revision: r.state.Revision + 1, Overrides: in.Overrides, SMTPPasswordCiphertext: in.SMTPPasswordCiphertext}
	return r.state, nil
}
func testDefaults() domain.Values {
	return domain.Values{AttachmentMaxBytes: 10 << 20, MediaMaxPeers: 10, MediaMaxRooms: 100, RecordingRetentionDays: 7, SMTPPort: 465, SMTPTLSMode: "tls"}
}
func testCipher(t *testing.T) *security.ProviderTokens {
	t.Helper()
	cipher, err := security.NewProviderTokens(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32))))
	if err != nil {
		t.Fatal(err)
	}
	return cipher
}
func testPatch(t *testing.T, raw string) domain.Patch {
	t.Helper()
	p, err := DecodePatch(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestSMTPEncryptedWriteOnlyAndBlankPreserves(t *testing.T) {
	repo := &testRepository{state: domain.State{Overrides: json.RawMessage(`{}`)}}
	svc := NewService(repo, testDefaults(), "", testCipher(t), "https://meet.example.invalid")
	ctx := context.Background()
	view, err := svc.Update(ctx, testPatch(t, `{"revision":0,"changes":{"smtpEnabled":true,"smtpHost":"smtp.example.invalid","smtpUsername":"sender","smtpPassword":"PrivateSyntheticSMTPValue","smtpFrom":"MeetSpace <sender@example.invalid>"}}`), admin.AuditInput{})
	if err != nil {
		t.Fatal(err)
	}
	view, err = svc.Update(ctx, testPatch(t, `{"revision":1,"changes":{"emailVerificationRequired":true}}`), admin.AuditInput{})
	if err != nil {
		t.Fatal(err)
	}
	if view.Revision != 2 || repo.state.SMTPPasswordCiphertext == "" || strings.Contains(repo.state.SMTPPasswordCiphertext, "PrivateSyntheticSMTPValue") || strings.Contains(string(repo.state.Overrides), "smtpPassword") {
		t.Fatal("password not isolated in encrypted storage")
	}
	encoded, _ := json.Marshal(view)
	if strings.Contains(string(encoded), "PrivateSyntheticSMTPValue") || strings.Contains(string(encoded), repo.state.SMTPPasswordCiphertext) {
		t.Fatal("settings response leaked a secret")
	}
	passwordItemFound := false
	for _, item := range view.Items {
		if item.Key == "smtpPassword" {
			passwordItemFound = true
			if item.Value != nil || !item.Configured || !item.Secret {
				t.Fatal("unsafe password metadata")
			}
		}
	}
	if !passwordItemFound {
		t.Fatal("missing password metadata")
	}
	ciphertext := repo.state.SMTPPasswordCiphertext
	if _, err := svc.Update(ctx, testPatch(t, `{"revision":2,"changes":{"smtpPassword":"","smtpUsername":"next-sender"}}`), admin.AuditInput{}); err != nil {
		t.Fatal(err)
	}
	if repo.state.SMTPPasswordCiphertext != ciphertext {
		t.Fatal("blank password erased or rewrote secret")
	}
	smtp, err := svc.SMTP(ctx)
	if err != nil || !smtp.Enabled || smtp.Password != "PrivateSyntheticSMTPValue" || smtp.Username != "next-sender" {
		t.Fatal("SMTP reader did not resolve current credentials", err)
	}
	encoded, _ = json.Marshal(smtp)
	if strings.Contains(string(encoded), "PrivateSyntheticSMTPValue") {
		t.Fatal("SMTP credentials JSON exposes password")
	}
	if _, err := svc.Update(ctx, testPatch(t, `{"revision":3,"changes":{"smtpEnabled":false}}`), admin.AuditInput{}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("SMTP disabled while verification required", err)
	}
	if _, err := svc.Update(ctx, testPatch(t, `{"revision":3,"changes":{"smtpEnabled":false,"emailVerificationRequired":false}}`), admin.AuditInput{}); err != nil {
		t.Fatal(err)
	}
}
func TestSettingsRejectUnknownUnsafeAndConflictingInput(t *testing.T) {
	for _, raw := range []string{`{}`, `{"changes":{"mediaMaxPeers":20}}`, `{"revision":null,"changes":{"mediaMaxPeers":20}}`, `{"revision":0.5,"changes":{"mediaMaxPeers":20}}`, `{"revision":0,"changes":{}}`, `{"revision":0,"changes":{"mediaMaxPeers":20},"unexpected":true}`, `null`} {
		if _, err := DecodePatch(json.RawMessage(raw)); !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Fatal("malformed patch accepted")
		}
	}
	for _, changes := range []string{`{"unknownKey":1}`, `{"revision":1}`, `{"smtpPasswordConfigured":true}`, `{"mediaMaxPeers":101}`, `{"mediaMaxRooms":10001}`, `{"attachmentMaxBytes":1023}`, `{"attachmentMaxBytes":104857601}`, `{"recordingRetentionDays":0}`, `{"smtpPort":0}`, `{"smtpPort":"465"}`, `{"smtpHost":"https://smtp.invalid"}`, `{"smtpTLSMode":"none"}`, `{"smtpFrom":"a@invalid,b@invalid"}`, `{"smtpEnabled":true}`, `{"emailVerificationRequired":true}`, `{"mediaMaxPeers":null}`, `{"mediaMaxPeers":1.2}`} {
		repo := &testRepository{state: domain.State{Overrides: json.RawMessage(`{}`)}}
		svc := NewService(repo, testDefaults(), "", testCipher(t), "https://meet.example.invalid")
		_, err := svc.Update(context.Background(), testPatch(t, `{"revision":0,"changes":`+changes+`}`), admin.AuditInput{})
		if !errors.Is(err, apperrors.ErrInvalidInput) || repo.saves != 0 {
			t.Fatal("unsafe settings saved", err)
		}
	}
	repo := &testRepository{state: domain.State{Revision: 2, Overrides: json.RawMessage(`{}`)}}
	svc := NewService(repo, testDefaults(), "", nil, "https://meet.example.invalid")
	if _, err := svc.Update(context.Background(), testPatch(t, `{"revision":1,"changes":{"mediaMaxPeers":20}}`), admin.AuditInput{}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("stale revision accepted", err)
	}
	if _, err := svc.Update(context.Background(), testPatch(t, `{"revision":2,"changes":{"smtpPort":587}}`), admin.AuditInput{}); !errors.Is(err, apperrors.ErrInvalidInput) || !strings.Contains(err.Error(), "PROVIDER_TOKEN_ENCRYPTION_KEY") {
		t.Fatal("missing key has no actionable error")
	}
}
func TestSettingsRuntimeReadsDoNotDecryptOrMaskDBFailure(t *testing.T) {
	ctx := context.Background()
	repo := &testRepository{state: domain.State{Overrides: json.RawMessage(`{"emailVerificationRequired":true,"attachmentMaxBytes":2048}`), SMTPPasswordCiphertext: "opaque-ciphertext"}}
	svc := NewService(repo, testDefaults(), "", nil, "https://meet.example.invalid")
	values, err := svc.Snapshot(ctx)
	if err != nil || values.AttachmentMaxBytes != 2048 || !values.EmailVerificationRequired {
		t.Fatal("worker public snapshot incorrectly depended on SMTP credentials", err)
	}
	if _, err := svc.SMTP(ctx); !errors.Is(err, apperrors.ErrUnavailable) {
		t.Fatal("mail reader accepted incomplete configuration", err)
	}
	repo.err = context.DeadlineExceeded
	if _, err := svc.Snapshot(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("DB failure silently replaced by defaults", err)
	}
}
func TestSettingsPreserveExistingEnvironmentSMTPForLimits(t *testing.T) {
	d := testDefaults()
	d.SMTPEnabled = true
	d.SMTPHost = "smtp.invalid"
	d.SMTPUsername = "sender"
	d.SMTPFrom = "sender@example.invalid"
	repo := &testRepository{state: domain.State{Overrides: json.RawMessage(`{}`)}}
	svc := NewService(repo, d, "PrivateEnvironmentSMTP", nil, "https://meet.example.invalid")
	if _, err := svc.Update(context.Background(), testPatch(t, `{"revision":0,"changes":{"mediaMaxPeers":20}}`), admin.AuditInput{}); err != nil {
		t.Fatal("unrelated limit edit required decryption key", err)
	}
	smtp, err := svc.SMTP(context.Background())
	if err != nil || smtp.Password != "PrivateEnvironmentSMTP" {
		t.Fatal("existing configured SMTP was lost", err)
	}
}

func TestSMTPWrongEncryptionKeyFailsClosedAndReplacementRecovers(t *testing.T) {
	repo := &testRepository{state: domain.State{Overrides: json.RawMessage(`{}`)}}
	svc := NewService(repo, testDefaults(), "", testCipher(t), "https://meet.example.invalid")
	if _, err := svc.Update(context.Background(), testPatch(t, `{"revision":0,"changes":{"smtpEnabled":true,"smtpHost":"smtp.invalid","smtpUsername":"sender","smtpPassword":"PreviousSyntheticSecret","smtpFrom":"sender@example.invalid"}}`), admin.AuditInput{}); err != nil {
		t.Fatal(err)
	}
	rotated, err := security.NewProviderTokens(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("b", 32))))
	if err != nil {
		t.Fatal(err)
	}
	svc = NewService(repo, testDefaults(), "", rotated, "https://meet.example.invalid")
	if _, err := svc.Snapshot(context.Background()); err != nil {
		t.Fatal("public snapshot decrypted credential", err)
	}
	if _, err := svc.SMTP(context.Background()); !errors.Is(err, apperrors.ErrUnavailable) {
		t.Fatal("wrong key did not stop delivery", err)
	}
	if _, err := svc.Update(context.Background(), testPatch(t, `{"revision":1,"changes":{"smtpHost":"new-smtp.invalid"}}`), admin.AuditInput{}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("unusable encrypted credential enabled", err)
	}
	if _, err := svc.Update(context.Background(), testPatch(t, `{"revision":1,"changes":{"smtpPassword":"ReplacementSyntheticSecret"}}`), admin.AuditInput{}); err != nil {
		t.Fatal("password replacement could not recover key change", err)
	}
	smtp, err := svc.SMTP(context.Background())
	if err != nil || smtp.Password != "ReplacementSyntheticSecret" {
		t.Fatal("replacement secret not usable", err)
	}
}

func TestEmailVerificationNeedsEncryptionKeyWithEnvironmentSMTP(t *testing.T) {
	d := testDefaults()
	d.SMTPEnabled = true
	d.SMTPHost = "smtp.invalid"
	d.SMTPUsername = "sender"
	d.SMTPFrom = "sender@example.invalid"
	repo := &testRepository{state: domain.State{Overrides: json.RawMessage(`{}`)}}
	svc := NewService(repo, d, "SyntheticEnvironmentCredential", nil, "https://meet.example.invalid")
	_, err := svc.Update(context.Background(), testPatch(t, `{"revision":0,"changes":{"emailVerificationRequired":true}}`), admin.AuditInput{})
	if !errors.Is(err, apperrors.ErrInvalidInput) || !strings.Contains(err.Error(), "PROVIDER_TOKEN_ENCRYPTION_KEY") || repo.saves != 0 {
		t.Fatal("email verification enabled without token encryption key")
	}
}

func TestEmailVerificationPolicyRequiresPreviouslySavedSMTP(t *testing.T) {
	ctx := context.Background()
	repo := &testRepository{state: domain.State{Overrides: json.RawMessage(`{}`)}}
	svc := NewService(repo, testDefaults(), "", testCipher(t), "https://meet.example.invalid")
	for _, patch := range []string{
		`{"revision":0,"changes":{"emailVerificationRequired":true}}`,
		`{"revision":0,"changes":{"emailVerificationRequired":false}}`,
		`{"revision":0,"changes":{"smtpEnabled":true,"smtpHost":"smtp.invalid","smtpUsername":"sender","smtpPassword":"SyntheticCredential","smtpFrom":"sender@example.invalid","emailVerificationRequired":true}}`,
	} {
		if _, err := svc.Update(ctx, testPatch(t, patch), admin.AuditInput{}); !errors.Is(err, apperrors.ErrInvalidInput) || repo.saves != 0 {
			t.Fatal("verification edited without saved SMTP", err)
		}
	}
	if _, err := svc.Update(ctx, testPatch(t, `{"revision":0,"changes":{"smtpEnabled":true,"smtpHost":"smtp.invalid","smtpUsername":"sender","smtpPassword":"SyntheticCredential","smtpFrom":"sender@example.invalid"}}`), admin.AuditInput{}); err != nil {
		t.Fatal(err)
	}
	view, err := svc.Admin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range view.Items {
		if item.Key == "emailVerificationRequired" {
			found = true
			if item.Group != "general" || !item.Editable {
				t.Fatal("verification not editable in General")
			}
		}
	}
	if !found {
		t.Fatal("verification policy missing")
	}
	if _, err := svc.Update(ctx, testPatch(t, `{"revision":1,"changes":{"emailVerificationRequired":true}}`), admin.AuditInput{}); err != nil {
		t.Fatal("saved SMTP did not unlock verification", err)
	}
	if _, err := svc.Update(ctx, testPatch(t, `{"revision":2,"changes":{"smtpEnabled":false}}`), admin.AuditInput{}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("SMTP disabled with required verification", err)
	}
	if _, err := svc.Update(ctx, testPatch(t, `{"revision":2,"changes":{"emailVerificationRequired":false}}`), admin.AuditInput{}); err != nil {
		t.Fatal("verification could not be disabled with saved SMTP", err)
	}
	if _, err := svc.Update(ctx, testPatch(t, `{"revision":3,"changes":{"smtpEnabled":false}}`), admin.AuditInput{}); err != nil {
		t.Fatal("SMTP could not be disabled after verification", err)
	}
	if _, err := svc.Update(ctx, testPatch(t, `{"revision":4,"changes":{"mediaMaxPeers":20}}`), admin.AuditInput{}); err != nil {
		t.Fatal("unrelated edit was blocked by absent SMTP", err)
	}
}

func TestEmailVerificationPolicyRejectsUnusableSavedSMTPPassword(t *testing.T) {
	d := testDefaults()
	d.SMTPEnabled = true
	d.SMTPHost = "smtp.invalid"
	d.SMTPUsername = "sender"
	d.SMTPFrom = "sender@example.invalid"
	for _, cipher := range []Cipher{nil, testCipher(t)} {
		repo := &testRepository{state: domain.State{Overrides: json.RawMessage(`{"emailVerificationRequired":true}`), SMTPPasswordCiphertext: "unusable-ciphertext"}}
		svc := NewService(repo, d, "", cipher, "https://meet.example.invalid")
		if _, err := svc.Update(context.Background(), testPatch(t, `{"revision":0,"changes":{"emailVerificationRequired":false}}`), admin.AuditInput{}); !errors.Is(err, apperrors.ErrInvalidInput) || repo.saves != 0 {
			t.Fatal("verification policy edited with unusable saved credentials", err)
		}
	}
}

func TestSMTPActivationRequiresUsablePublicOrigin(t *testing.T) {
	for _, origin := range []string{"", "https://", "https://user:password@example.invalid", "https://example.invalid/app", "https://example.invalid?token=private"} {
		repo := &testRepository{state: domain.State{Overrides: json.RawMessage(`{}`)}}
		svc := NewService(repo, testDefaults(), "", testCipher(t), origin)
		_, err := svc.Update(context.Background(), testPatch(t, `{"revision":0,"changes":{"smtpEnabled":true,"smtpHost":"smtp.example.invalid","smtpUsername":"sender","smtpPassword":"SyntheticCredential","smtpFrom":"sender@example.invalid"}}`), admin.AuditInput{})
		if !errors.Is(err, apperrors.ErrInvalidInput) || repo.saves != 0 {
			t.Fatal("email enabled without public origin")
		}
	}
}
