package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"github.com/janickiy/meet-space/internal/domain/settings"
	"github.com/janickiy/meet-space/internal/domain/users"
	"github.com/janickiy/meet-space/internal/infrastructure/security"
	authusecase "github.com/janickiy/meet-space/internal/usecase/auth"
)

type verificationPolicy struct{ values settings.Values }

func (p *verificationPolicy) Snapshot(context.Context) (settings.Values, error) { return p.values, nil }

type verificationTokens struct{}

func (verificationTokens) Issue(string) (string, error) { return "synthetic-access-token", nil }

type verificationMail struct{ message integrations.EmailMessage }

func (m *verificationMail) Send(_ context.Context, message integrations.EmailMessage) error {
	m.message = message
	return nil
}
func verificationToken(t *testing.T, message integrations.EmailMessage) string {
	t.Helper()
	for _, word := range strings.Fields(message.Text) {
		if strings.HasPrefix(word, "https://") {
			u, err := url.Parse(word)
			if err != nil {
				t.Fatal(err)
			}
			if u.RawQuery != "" || u.Path != "/verify-email" {
				t.Fatal("token leaked into request URL")
			}
			params, _ := url.ParseQuery(u.Fragment)
			return params.Get("token")
		}
	}
	t.Fatal("verification link missing")
	return ""
}
func TestEmailVerificationLifecycle(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	policy := &verificationPolicy{values: settings.Values{EmailVerificationRequired: true, SMTPEnabled: true}}
	repo := NewEmailVerificationRepository(db)
	cipher, err := security.NewProviderTokens(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("v", 32))))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := authusecase.NewService(NewUserRepository(db), adminTestPasswords{}, verificationTokens{})
	if err != nil {
		t.Fatal(err)
	}
	svc.WithEmailVerification(policy, repo, cipher)
	view, err := svc.Register(ctx, users.RegisterRequest{Email: "verify@example.invalid", Password: "SyntheticPassword2468"})
	if err != nil || !view.EmailVerificationRequired {
		t.Fatalf("pending registration: %+v %v", view, err)
	}
	credentials := users.LoginRequest{Email: view.Email, Password: "SyntheticPassword2468"}
	if _, err = svc.Login(ctx, credentials); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("unverified login allowed", err)
	}
	wrong := credentials
	wrong.Password = "IncorrectPassword2468"
	if _, err = svc.Login(ctx, wrong); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("bad password disclosed verification state", err)
	}
	jobRepo := NewJobRepository(db)
	job, found, err := jobRepo.Claim(ctx, authusecase.VerificationJobKind, time.Minute)
	if err != nil || !found {
		t.Fatal("verification job not queued", err)
	}
	mail := &verificationMail{}
	sender := authusecase.VerificationSender{Repo: repo, Reader: policy, Cipher: cipher, Email: mail, PublicURL: "https://verify.example.invalid"}
	if err = sender.Handle(ctx, job); err != nil {
		t.Fatal(err)
	}
	token := verificationToken(t, mail.message)
	if len(token) != 43 {
		t.Fatal("unexpected verification entropy")
	}
	var stored string
	if err = db.Raw(`SELECT token_ciphertext FROM email_verifications WHERE user_id=?::uuid`, view.ID).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, token) || strings.Contains(string(job.Payload), token) {
		t.Fatal("verification credential stored as plaintext")
	}
	if _, err = svc.Register(ctx, users.RegisterRequest{Email: view.Email, Password: "SyntheticPassword2468"}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("duplicate registration", err)
	}
	if err = svc.ResendVerification(ctx, view.Email); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Table("background_jobs").Where("kind=?", authusecase.VerificationJobKind).Count(&count)
	if count != 1 {
		t.Fatal("resend cooldown bypassed", count)
	}
	// One minute is no longer enough: the first link remains valid while a
	// repeat request is acknowledged without queuing another message.
	if err = db.Exec(`UPDATE email_verifications SET created_at=clock_timestamp()-INTERVAL '90 seconds' WHERE user_id=?::uuid`, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = svc.ResendVerification(ctx, view.Email); err != nil {
		t.Fatal(err)
	}
	if err = db.Table("background_jobs").Where("kind=?", authusecase.VerificationJobKind).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("verification resend allowed before two minutes", count, err)
	}
	var initialVerificationID string
	if err = db.Raw(`SELECT id FROM email_verifications WHERE user_id=?::uuid`, view.ID).Scan(&initialVerificationID).Error; err != nil || initialVerificationID != job.EntityID {
		t.Fatal("cooldown replaced the original verification link", err)
	}
	policy.values.EmailVerificationRequired = false
	if _, err = svc.Login(ctx, credentials); err != nil {
		t.Fatal("disabled policy still blocks login", err)
	}
	legacy, err := svc.Register(ctx, users.RegisterRequest{Email: "existing@example.invalid", Password: "SyntheticPassword2468"})
	if err != nil {
		t.Fatal(err)
	}
	policy.values.EmailVerificationRequired = true
	if _, err = svc.Login(ctx, users.LoginRequest{Email: legacy.Email, Password: "SyntheticPassword2468"}); err != nil {
		t.Fatal("existing account locked", err)
	}
	if err = db.Exec(`UPDATE email_verifications SET created_at=clock_timestamp()-INTERVAL '2 minutes' WHERE user_id=?::uuid`, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = svc.ResendVerification(ctx, view.Email); err != nil {
		t.Fatal(err)
	}
	if err = db.Table("background_jobs").Where("kind=?", authusecase.VerificationJobKind).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("verification resend not queued after two minutes", count, err)
	}
	if err = svc.VerifyEmail(ctx, token); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("rotated token accepted", err)
	}
	next, found, err := jobRepo.Claim(ctx, authusecase.VerificationJobKind, time.Minute)
	if err != nil || !found {
		t.Fatal(err)
	}
	if err = sender.Handle(ctx, next); err != nil {
		t.Fatal(err)
	}
	token = verificationToken(t, mail.message)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- svc.VerifyEmail(ctx, token) }()
	}
	wg.Wait()
	close(results)
	success, invalid := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, apperrors.ErrInvalidInput) {
			invalid++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || invalid != 1 {
		t.Fatal("verification not one-time", success, invalid)
	}
	if _, err = svc.Login(ctx, credentials); err != nil {
		t.Fatal("verified login failed", err)
	}
	if err = sender.Handle(ctx, next); !errors.Is(err, jobs.ErrSkip) {
		t.Fatal("consumed link sent again", err)
	}
	var user users.User
	if err = db.First(&user, "id=?::uuid", view.ID).Error; err != nil {
		t.Fatal(err)
	}
	if user.EmailVerifiedAt == nil {
		t.Fatal("verified state missing")
	}
	data, _ := json.Marshal(user.View())
	if strings.Contains(string(data), "token") {
		t.Fatal("token exposed by user view")
	}
}
func TestEmailVerificationExpiry(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	policy := &verificationPolicy{values: settings.Values{EmailVerificationRequired: true, SMTPEnabled: true}}
	repo := NewEmailVerificationRepository(db)
	cipher, _ := security.NewProviderTokens(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32))))
	svc, _ := authusecase.NewService(NewUserRepository(db), adminTestPasswords{}, verificationTokens{})
	svc.WithEmailVerification(policy, repo, cipher)
	user, err := svc.Register(ctx, users.RegisterRequest{Email: "expired@example.invalid", Password: "SyntheticPassword2468"})
	if err != nil {
		t.Fatal(err)
	}
	job, ok, err := NewJobRepository(db).Claim(ctx, authusecase.VerificationJobKind, time.Minute)
	if !ok || err != nil {
		t.Fatal(err)
	}
	mail := &verificationMail{}
	sender := authusecase.VerificationSender{Repo: repo, Reader: policy, Cipher: cipher, Email: mail, PublicURL: "https://verify.example.invalid"}
	if err = sender.Handle(ctx, job); err != nil {
		t.Fatal(err)
	}
	token := verificationToken(t, mail.message)
	if err = db.Exec(`UPDATE email_verifications SET created_at=clock_timestamp()-INTERVAL '2 days',expires_at=clock_timestamp()-INTERVAL '1 day' WHERE user_id=?::uuid`, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = svc.VerifyEmail(ctx, token); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("expired token accepted", err)
	}
	if err = sender.Handle(ctx, job); !errors.Is(err, jobs.ErrSkip) {
		t.Fatal("expired link delivered", err)
	}
}

func TestEmailVerificationPolicyRevokesPendingDurableAuthorization(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	policy := &verificationPolicy{values: settings.Values{EmailVerificationRequired: true, SMTPEnabled: true}}
	repo := NewEmailVerificationRepository(db)
	cipher, _ := security.NewProviderTokens(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("z", 32))))
	tokens, err := security.NewTokenService(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := authusecase.NewService(NewUserRepository(db), adminTestPasswords{}, tokens)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithSessions(NewAuthSessionRepository(db)).WithEmailVerification(policy, repo, cipher)
	user, err := svc.Register(ctx, users.RegisterRequest{Email: "durable@example.invalid", Password: "SyntheticPassword2468"})
	if err != nil {
		t.Fatal(err)
	}
	policy.values.EmailVerificationRequired = false
	session, err := svc.Login(ctx, users.LoginRequest{Email: user.Email, Password: "SyntheticPassword2468"})
	if err != nil {
		t.Fatal(err)
	}
	policy.values.EmailVerificationRequired = true
	if _, err = svc.Refresh(ctx, session.SessionToken); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("pending refresh allowed", err)
	}
	if _, _, _, _, err = svc.VerifyAuthorization(ctx, session.AccessToken); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("pending HTTP/WS authorization allowed", err)
	}
	if _, _, _, err = svc.VerifySession(session.AccessToken); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("pending websocket token allowed", err)
	}
	var v authusecase.Verification
	if err = db.Table("email_verifications").Where("user_id=?::uuid", user.ID).Take(&v).Error; err != nil {
		t.Fatal(err)
	}
	token, err := cipher.Decrypt(v.TokenCiphertext, "email-verification:"+v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.VerifyEmail(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Refresh(ctx, session.SessionToken); err != nil {
		t.Fatal("verified refresh rejected", err)
	}
}
