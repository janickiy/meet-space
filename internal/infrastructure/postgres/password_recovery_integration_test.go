package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"github.com/janickiy/meet-space/internal/domain/settings"
	"github.com/janickiy/meet-space/internal/domain/users"
	"github.com/janickiy/meet-space/internal/infrastructure/security"
	authusecase "github.com/janickiy/meet-space/internal/usecase/auth"
	"gorm.io/gorm"
)

type recoveryFixture struct {
	db      *gorm.DB
	service *authusecase.Service
	reader  *verificationPolicy
	sender  authusecase.PasswordRecoverySender
	mail    *verificationMail
	jobs    *JobRepository
	user    users.View
	tokens  *security.TokenService
}

func recoveryTestFixture(t *testing.T) *recoveryFixture {
	t.Helper()
	db := auditDatabase(t)
	values := settings.Values{SMTPEnabled: true, SMTPHost: "smtp.example.invalid", SMTPPort: 465, SMTPUsername: "synthetic-sender", SMTPFrom: "sender@example.invalid", SMTPTLSMode: "tls", SMTPPasswordConfigured: true}
	reader := &verificationPolicy{values: values}
	cipher, err := security.NewProviderTokens(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("r", 32))))
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := security.NewTokenService("synthetic-recovery-test-secret-32bytes")
	if err != nil {
		t.Fatal(err)
	}
	service, err := authusecase.NewService(NewUserRepository(db), adminTestPasswords{}, tokens)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewPasswordRecoveryRepository(db)
	service.WithSessions(NewAuthSessionRepository(db)).WithPasswordRecovery(reader, repo, cipher)
	user, err := service.Register(context.Background(), users.RegisterRequest{Email: "recover@example.invalid", Password: "OriginalPassword123"})
	if err != nil {
		t.Fatal(err)
	}
	mail := &verificationMail{}
	sender := authusecase.PasswordRecoverySender{Repo: repo, Reader: reader, Cipher: cipher, Email: mail, PublicURL: "https://trusted.example.invalid"}
	return &recoveryFixture{db: db, service: service, reader: reader, sender: sender, mail: mail, jobs: NewJobRepository(db), user: user, tokens: tokens}
}
func recoveryLink(t *testing.T, message integrations.EmailMessage) string {
	t.Helper()
	if message.To != "recover@example.invalid" {
		t.Fatal("email sent to wrong recipient")
	}
	for _, word := range strings.Fields(message.Text) {
		if !strings.HasPrefix(word, "https://") {
			continue
		}
		value, err := url.Parse(word)
		if err != nil || value.Scheme != "https" || value.Host != "trusted.example.invalid" || value.Path != "/reset-password" || value.RawQuery != "" {
			t.Fatal("reset link uses untrusted origin or query")
		}
		fragment, err := url.ParseQuery(value.Fragment)
		if err != nil {
			t.Fatal(err)
		}
		token := fragment.Get("token")
		if len(token) != 43 || !strings.Contains(message.HTML, token) {
			t.Fatal("reset link missing entropy or HTML counterpart")
		}
		return token
	}
	t.Fatal("reset link absent")
	return ""
}
func (f *recoveryFixture) request(t *testing.T) (string, jobs.Job) {
	t.Helper()
	ctx := context.Background()
	if err := f.service.PasswordRecovery(ctx, " RECOVER@EXAMPLE.INVALID "); err != nil {
		t.Fatal(err)
	}
	job, found, err := f.jobs.Claim(ctx, authusecase.PasswordRecoveryJobKind, time.Minute)
	if err != nil || !found {
		t.Fatal("recovery not queued", err)
	}
	if err = f.sender.Handle(ctx, job); err != nil {
		t.Fatal(err)
	}
	return recoveryLink(t, f.mail.message), job
}
func TestPasswordRecoveryLifecycleAndRevocation(t *testing.T) {
	f := recoveryTestFixture(t)
	ctx := context.Background()
	var observed atomic.Int32
	f.service.WithPasswordResetObserver(func(id string) {
		if id == f.user.ID {
			observed.Add(1)
		}
	})
	old, err := f.service.Login(ctx, users.LoginRequest{Email: f.user.Email, Password: "OriginalPassword123"})
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := f.tokens.Issue(f.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	token, job := f.request(t)
	var row authusecase.PasswordRecovery
	if err = f.db.Table("password_recoveries").Where("user_id=?::uuid", f.user.ID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(row.TokenCiphertext, token) || strings.Contains(string(job.Payload), token) || row.TokenHash == token {
		t.Fatal("credential stored in plaintext")
	}
	if remaining := time.Until(row.ExpiresAt); remaining < 59*time.Minute || remaining > time.Hour {
		t.Fatal("reset expiry not one hour")
	}
	for _, password := range []string{"short", strings.Repeat("z", 129)} {
		if err = f.service.ResetPassword(ctx, token, password); !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Fatal("password bounds ignored", err)
		}
	}
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() { defer wait.Done(); results <- f.service.ResetPassword(ctx, token, "ReplacementPassword456") }()
	}
	wait.Wait()
	close(results)
	success, invalid := 0, 0
	for err = range results {
		if err == nil {
			success++
		} else if errors.Is(err, apperrors.ErrInvalidInput) {
			invalid++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || invalid != 1 || observed.Load() != 1 {
		t.Fatal("reset not single-use or observer incorrect")
	}
	for _, bearer := range []string{old.AccessToken, legacy} {
		if _, _, _, _, err = f.service.VerifyAuthorization(ctx, bearer); !errors.Is(err, apperrors.ErrUnauthorized) {
			t.Fatal("old bearer survived reset", err)
		}
	}
	if _, err = f.service.Refresh(ctx, old.SessionToken); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("old refresh survived reset", err)
	}
	if _, err = f.service.Login(ctx, users.LoginRequest{Email: f.user.Email, Password: "OriginalPassword123"}); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("old password survived", err)
	}
	fresh, err := f.service.Login(ctx, users.LoginRequest{Email: f.user.Email, Password: "ReplacementPassword456"})
	if err != nil || fresh.AccessToken == "" {
		t.Fatal("new login failed", err)
	}
	if _, _, _, _, err = f.service.VerifyAuthorization(ctx, fresh.AccessToken); err != nil {
		t.Fatal("new bearer invalid", err)
	}
	if err = f.sender.Handle(ctx, job); !errors.Is(err, jobs.ErrSkip) {
		t.Fatal("consumed reset delivered", err)
	}
	var ciphertext string
	f.db.Raw(`SELECT token_ciphertext FROM password_recoveries WHERE user_id=?::uuid`, f.user.ID).Scan(&ciphertext)
	if ciphertext != "" {
		t.Fatal("consumed encrypted credential retained")
	}
	notice, found, err := f.jobs.Claim(ctx, authusecase.PasswordChangedJobKind, time.Minute)
	if err != nil || !found {
		t.Fatal("change notice not durable", err)
	}
	if err = f.sender.Handle(ctx, notice); err != nil {
		t.Fatal(err)
	}
	if f.mail.message.To != f.user.Email || strings.Contains(f.mail.message.Text, token) || strings.Contains(f.mail.message.Text, "ReplacementPassword456") || !strings.Contains(f.mail.message.Subject, "изменён") {
		t.Fatal("unsafe or missing password-change notification")
	}
}
func TestPasswordRecoveryCooldownRotationExpiryAndEligibility(t *testing.T) {
	f := recoveryTestFixture(t)
	ctx := context.Background()
	oldToken, oldJob := f.request(t)
	for i := 0; i < 3; i++ {
		if err := f.service.PasswordRecovery(ctx, f.user.Email); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	f.db.Table("background_jobs").Where("kind=?", authusecase.PasswordRecoveryJobKind).Count(&count)
	if count != 1 {
		t.Fatal("per-account cooldown bypassed")
	}
	var initial authusecase.PasswordRecovery
	if err := f.db.Table("password_recoveries").Where("user_id=?::uuid", f.user.ID).Take(&initial).Error; err != nil {
		t.Fatal(err)
	}
	// A request after one minute must still preserve the existing link and job.
	if err := f.db.Exec(`UPDATE password_recoveries SET created_at=clock_timestamp()-INTERVAL '90 seconds' WHERE user_id=?::uuid`, f.user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.service.PasswordRecovery(ctx, f.user.Email); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Table("background_jobs").Where("kind=?", authusecase.PasswordRecoveryJobKind).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("password recovery allowed before two minutes", count, err)
	}
	var unchanged authusecase.PasswordRecovery
	if err := f.db.Table("password_recoveries").Where("user_id=?::uuid", f.user.ID).Take(&unchanged).Error; err != nil || unchanged.ID != initial.ID || unchanged.TokenHash != initial.TokenHash {
		t.Fatal("cooldown replaced the original recovery link", err)
	}
	if err := f.service.PasswordRecovery(ctx, "missing@example.invalid"); err != nil {
		t.Fatal("unknown email disclosed", err)
	}
	if err := f.db.Exec(`UPDATE password_recoveries SET created_at=clock_timestamp()-INTERVAL '2 minutes' WHERE user_id=?::uuid`, f.user.ID).Error; err != nil {
		t.Fatal(err)
	}
	token, _ := f.request(t)
	if err := f.db.Table("background_jobs").Where("kind=?", authusecase.PasswordRecoveryJobKind).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("password recovery not queued after two minutes", count, err)
	}
	if token == oldToken {
		t.Fatal("reset token reused")
	}
	if err := f.service.ResetPassword(ctx, oldToken, "AnotherPassword123"); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("old rotated token accepted", err)
	}
	if err := f.sender.Handle(ctx, oldJob); !errors.Is(err, jobs.ErrSkip) {
		t.Fatal("obsolete email delivered", err)
	}
	if err := f.db.Exec(`UPDATE users SET is_blocked=true WHERE id=?::uuid`, f.user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.service.PasswordRecovery(ctx, f.user.Email); !errors.Is(err, apperrors.ErrAccountBlocked) {
		t.Fatal("blocked recovery accepted", err)
	}
	if err := f.service.ResetPassword(ctx, token, "AnotherPassword123"); !errors.Is(err, apperrors.ErrAccountBlocked) {
		t.Fatal("blocked reset accepted", err)
	}
	f.db.Exec(`UPDATE users SET is_blocked=false WHERE id=?::uuid`, f.user.ID)
	f.db.Exec(`UPDATE password_recoveries SET created_at=clock_timestamp()-INTERVAL '2 hours',expires_at=clock_timestamp()-INTERVAL '1 hour' WHERE user_id=?::uuid`, f.user.ID)
	if err := f.service.ResetPassword(ctx, token, "AnotherPassword123"); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("expired reset accepted", err)
	}
	for _, bad := range []string{"", token + "=", strings.Repeat("a", 43)} {
		if err := f.service.ResetPassword(ctx, bad, "AnotherPassword123"); !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Fatal("invalid reset accepted", err)
		}
	}
	f.reader.values.SMTPEnabled = false
	for _, email := range []string{f.user.Email, "missing@example.invalid"} {
		if err := f.service.PasswordRecovery(ctx, email); !errors.Is(err, apperrors.ErrUnavailable) {
			t.Fatal("SMTP disabled response", err)
		}
	}
}

type recoveryPausedPasswords struct {
	started, release chan struct{}
	once             sync.Once
}

func (p *recoveryPausedPasswords) Hash(v string) (string, error) { return adminTestPasswords{}.Hash(v) }
func (p *recoveryPausedPasswords) Verify(v, h string) (bool, error) {
	p.once.Do(func() { close(p.started); <-p.release })
	return adminTestPasswords{}.Verify(v, h)
}
func TestPasswordRecoveryRejectsLoginVerifiedBeforeReset(t *testing.T) {
	f := recoveryTestFixture(t)
	ctx := context.Background()
	token, _ := f.request(t)
	passwords := &recoveryPausedPasswords{started: make(chan struct{}), release: make(chan struct{})}
	service, err := authusecase.NewService(NewUserRepository(f.db), passwords, f.tokens)
	if err != nil {
		t.Fatal(err)
	}
	service.WithSessions(NewAuthSessionRepository(f.db))
	result := make(chan error, 1)
	go func() {
		_, err := service.Login(ctx, users.LoginRequest{Email: f.user.Email, Password: "OriginalPassword123"})
		result <- err
	}()
	<-passwords.started
	if err = f.service.ResetPassword(ctx, token, "ReplacementPassword456"); err != nil {
		t.Fatal(err)
	}
	close(passwords.release)
	if err = <-result; !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("in-flight old-password login survived reset", err)
	}
}
