package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	platformsettings "github.com/janickiy/meet-space/internal/domain/settings"
	"github.com/janickiy/meet-space/internal/domain/users"
)

const VerificationJobKind = "auth.email_verification"
const VerificationTTL = 24 * time.Hour

type Verification struct {
	ID, UserID, TokenHash, TokenCiphertext string
	ExpiresAt                              time.Time
}
type VerificationDelivery struct {
	Verification
	Email string
}
type VerificationRepository interface {
	CreatePending(context.Context, users.User, Verification) (users.User, error)
	Resend(context.Context, users.User, Verification) error
	Consume(context.Context, string) error
	Delivery(context.Context, jobs.Job) (VerificationDelivery, error)
}
type VerificationCipher interface {
	Encrypt(string, string) (string, error)
	Decrypt(string, string) (string, error)
}
type verificationOptions struct {
	reader platformsettings.Reader
	repo   VerificationRepository
	cipher VerificationCipher
}

func (s *Service) WithEmailVerification(reader platformsettings.Reader, repo VerificationRepository, cipher VerificationCipher) *Service {
	s.verification = &verificationOptions{reader: reader, repo: repo, cipher: cipher}
	return s
}
func (s *Service) verificationEnabled(ctx context.Context) (bool, error) {
	if s.verification == nil {
		return false, nil
	}
	values, err := s.verification.reader.Snapshot(ctx)
	return values.EmailVerificationRequired, err
}
func (s *Service) enforceEmailVerification(ctx context.Context, user users.User) error {
	if !user.EmailVerificationRequired || user.EmailVerifiedAt != nil {
		return nil
	}
	enabled, err := s.verificationEnabled(ctx)
	if err != nil {
		return err
	}
	if enabled {
		return apperrors.New(apperrors.ErrForbidden, "email verification required")
	}
	return nil
}
func (s *Service) newVerification(userID string) (Verification, error) {
	value := Verification{ID: uuid.NewString(), UserID: userID, ExpiresAt: time.Now().UTC().Add(VerificationTTL)}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return value, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(token))
	value.TokenHash = hex.EncodeToString(digest[:])
	var err error
	if s.verification == nil || s.verification.cipher == nil {
		return value, apperrors.ErrUnavailable
	}
	value.TokenCiphertext, err = s.verification.cipher.Encrypt(token, "email-verification:"+value.ID)
	return value, err
}
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	if err := s.requireLocalAuthentication(ctx); err != nil {
		return err
	}
	if s.verification == nil {
		return apperrors.ErrUnavailable
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(raw) != 32 {
		return apperrors.New(apperrors.ErrInvalidInput, "invalid or expired verification link")
	}
	digest := sha256.Sum256([]byte(token))
	return s.verification.repo.Consume(ctx, hex.EncodeToString(digest[:]))
}
func (s *Service) ResendVerification(ctx context.Context, email string) error {
	if err := s.requireLocalAuthentication(ctx); err != nil {
		return err
	}
	if s.verification == nil {
		return apperrors.ErrUnavailable
	}
	email = users.NormalizeEmail(email)
	if users.ValidateEmail(email) != nil {
		return apperrors.ErrInvalidInput
	}
	enabled, err := s.verificationEnabled(ctx)
	if err != nil || !enabled {
		return err
	}
	user, err := s.repository.GetByEmail(ctx, email)
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if user.AccessBlocked() || user.GuestConferenceID != nil || !user.EmailVerificationRequired || user.EmailVerifiedAt != nil {
		return nil
	}
	value, err := s.newVerification(user.ID)
	if err != nil {
		return err
	}
	return s.verification.repo.Resend(ctx, user, value)
}

type VerificationSender struct {
	Repo      VerificationRepository
	Reader    platformsettings.Reader
	Cipher    VerificationCipher
	Email     integrations.EmailProvider
	PublicURL string
}

func (s VerificationSender) Handle(ctx context.Context, job jobs.Job) error {
	values, err := s.Reader.Snapshot(ctx)
	if err != nil {
		return err
	}
	if values.LDAPEnabled || !values.EmailVerificationRequired || !values.SMTPEnabled {
		return jobs.ErrSkip
	}
	delivery, err := s.Repo.Delivery(ctx, job)
	if errors.Is(err, apperrors.ErrNotFound) {
		return jobs.ErrSkip
	}
	if err != nil {
		return err
	}
	if s.Cipher == nil {
		return jobs.Error{Code: "verification_encryption_unavailable", Retryable: false}
	}
	token, err := s.Cipher.Decrypt(delivery.TokenCiphertext, "email-verification:"+delivery.ID)
	if err != nil {
		return jobs.Error{Code: "verification_encryption_unavailable", Retryable: false}
	}
	parsed, err := url.Parse(s.PublicURL)
	if err != nil || parsed.Host == "" || parsed.Scheme != "https" && parsed.Scheme != "http" {
		return jobs.Error{Code: "verification_origin_invalid"}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/verify-email"
	parsed.RawQuery = ""
	parsed.Fragment = "token=" + url.QueryEscape(token)
	link := parsed.String()
	return s.Email.Send(ctx, integrations.EmailMessage{IdempotencyKey: "verify-email:" + delivery.ID, To: delivery.Email, Subject: "Подтверждение регистрации MeetSpace", Text: fmt.Sprintf("Подтвердите email для входа в MeetSpace: %s\nСсылка действует 24 часа. Если вы не регистрировались, проигнорируйте письмо.", link), HTML: "<p>Подтвердите email для входа в MeetSpace.</p><p><a href=\"" + html.EscapeString(link) + "\">Подтвердить email</a></p><p>Ссылка действует 24 часа. Если вы не регистрировались, проигнорируйте письмо.</p>"})
}
