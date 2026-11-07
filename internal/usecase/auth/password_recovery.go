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
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	platformsettings "github.com/janickiy/meet-space/internal/domain/settings"
	"github.com/janickiy/meet-space/internal/domain/users"
)

const PasswordRecoveryJobKind = "auth.password_recovery"
const PasswordChangedJobKind = "auth.password_changed"
const PasswordRecoveryTTL = time.Hour

type PasswordRecovery struct {
	ID, UserID, TokenHash, TokenCiphertext string
	ExpiresAt                              time.Time
}
type PasswordRecoveryDelivery struct {
	PasswordRecovery
	Email string
}
type PasswordRecoveryRepository interface {
	Request(context.Context, users.User, PasswordRecovery) error
	Consume(context.Context, string, string) (string, error)
	Delivery(context.Context, jobs.Job) (PasswordRecoveryDelivery, error)
	ChangedDelivery(context.Context, jobs.Job) (string, error)
}
type passwordRecoveryOptions struct {
	reader  platformsettings.Reader
	repo    PasswordRecoveryRepository
	cipher  VerificationCipher
	onReset func(string)
}

func (s *Service) WithPasswordRecovery(reader platformsettings.Reader, repo PasswordRecoveryRepository, cipher VerificationCipher) *Service {
	s.recovery = &passwordRecoveryOptions{reader: reader, repo: repo, cipher: cipher}
	return s
}

// WithPasswordResetObserver closes this replica's sockets immediately. Other
// replicas enforce the same durable revocation during authorization checks.
func (s *Service) WithPasswordResetObserver(observer func(string)) *Service {
	if s.recovery != nil {
		s.recovery.onReset = observer
	}
	return s
}
func recoverySMTPReady(v platformsettings.Values) bool {
	return v.SMTPEnabled && v.SMTPHost != "" && v.SMTPUsername != "" && v.SMTPFrom != "" && v.SMTPPasswordConfigured && v.SMTPPort > 0 && v.SMTPPort <= 65535 && (v.SMTPTLSMode == "tls" || v.SMTPTLSMode == "starttls")
}

// PasswordRecovery queues a durable email without disclosing whether a normal
// account exists. Blocked accounts keep their explicitly requested 403 response.
func (s *Service) PasswordRecovery(ctx context.Context, email string) error {
	email = users.NormalizeEmail(email)
	if err := users.ValidateEmail(email); err != nil {
		return err
	}
	user, lookupErr := s.repository.GetByEmail(ctx, email)
	if lookupErr != nil && !errors.Is(lookupErr, apperrors.ErrNotFound) {
		return lookupErr
	}
	if lookupErr == nil && user.AccessBlocked() {
		return apperrors.ErrAccountBlocked
	}
	unavailable := apperrors.New(apperrors.ErrUnavailable, "password recovery unavailable")
	if s.recovery == nil || s.recovery.reader == nil || s.recovery.repo == nil || s.recovery.cipher == nil {
		return unavailable
	}
	values, err := s.recovery.reader.Snapshot(ctx)
	if err != nil {
		return err
	}
	if !recoverySMTPReady(values) {
		return unavailable
	}
	// Run random generation, hashing and encryption for missing accounts too.
	value := PasswordRecovery{ID: uuid.NewString(), UserID: user.ID, ExpiresAt: time.Now().UTC().Add(PasswordRecoveryTTL)}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(token))
	value.TokenHash = hex.EncodeToString(digest[:])
	value.TokenCiphertext, err = s.recovery.cipher.Encrypt(token, "password-recovery:"+value.ID)
	if err != nil {
		return unavailable
	}
	if lookupErr != nil || user.GuestConferenceID != nil || user.PasswordHash == "" || user.IsLDAP() || user.DeletedAt != nil {
		return nil
	}
	return s.recovery.repo.Request(ctx, user, value)
}
func (s *Service) ResetPassword(ctx context.Context, token, password string) error {
	if s.recovery == nil || s.recovery.repo == nil {
		return apperrors.ErrUnavailable
	}
	if len(token) != 43 {
		return apperrors.New(apperrors.ErrInvalidInput, "invalid or expired password reset link")
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(raw) != 32 {
		return apperrors.New(apperrors.ErrInvalidInput, "invalid or expired password reset link")
	}
	length := utf8.RuneCountInString(password)
	if !utf8.ValidString(password) || length < users.MinPasswordCharacters || length > users.MaxPasswordCharacters {
		return apperrors.New(apperrors.ErrInvalidInput, "password must contain 8 to 128 characters")
	}
	hash, err := s.hashPassword(ctx, password)
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(token))
	userID, err := s.recovery.repo.Consume(ctx, hex.EncodeToString(digest[:]), hash)
	if err != nil {
		return err
	}
	if s.recovery.onReset != nil {
		s.recovery.onReset(userID)
	}
	return nil
}

type PasswordRecoverySender struct {
	Repo      PasswordRecoveryRepository
	Reader    platformsettings.Reader
	Cipher    VerificationCipher
	Email     integrations.EmailProvider
	PublicURL string
}

func (s PasswordRecoverySender) Handle(ctx context.Context, job jobs.Job) error {
	values, err := s.Reader.Snapshot(ctx)
	if err != nil {
		return err
	}
	if !recoverySMTPReady(values) {
		return jobs.ErrSkip
	}
	if job.Kind == PasswordChangedJobKind {
		email, err := s.Repo.ChangedDelivery(ctx, job)
		if errors.Is(err, apperrors.ErrNotFound) {
			return jobs.ErrSkip
		}
		if err != nil {
			return err
		}
		return s.Email.Send(ctx, integrations.EmailMessage{IdempotencyKey: "password-changed:" + job.EntityID, To: email, Subject: "Пароль MeetSpace изменён", Text: "Пароль вашего аккаунта MeetSpace изменён. Все прежние сеансы завершены. Если это были не вы, восстановите пароль через сайт MeetSpace.", HTML: "<p>Пароль вашего аккаунта MeetSpace изменён. Все прежние сеансы завершены.</p><p>Если это были не вы, восстановите пароль через сайт MeetSpace.</p>"})
	}
	delivery, err := s.Repo.Delivery(ctx, job)
	if errors.Is(err, apperrors.ErrNotFound) {
		return jobs.ErrSkip
	}
	if err != nil {
		return err
	}
	if s.Cipher == nil {
		return jobs.Error{Code: "recovery_encryption_unavailable", Retryable: false}
	}
	token, err := s.Cipher.Decrypt(delivery.TokenCiphertext, "password-recovery:"+delivery.ID)
	if err != nil {
		return jobs.Error{Code: "recovery_encryption_unavailable", Retryable: false}
	}
	parsed, err := url.Parse(s.PublicURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Scheme != "https" && parsed.Scheme != "http" {
		return jobs.Error{Code: "recovery_origin_invalid", Retryable: false}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/reset-password"
	parsed.RawQuery = ""
	parsed.Fragment = "token=" + url.QueryEscape(token)
	link := parsed.String()
	return s.Email.Send(ctx, integrations.EmailMessage{IdempotencyKey: "password-recovery:" + delivery.ID, To: delivery.Email, Subject: "Восстановление пароля MeetSpace", Text: fmt.Sprintf("Для изменения пароля MeetSpace перейдите по ссылке: %s\nСсылка действует 1 час и может быть использована один раз. Если вы не запрашивали восстановление, проигнорируйте письмо.", link), HTML: "<p>Для изменения пароля MeetSpace перейдите по ссылке.</p><p><a href=\"" + html.EscapeString(link) + "\">Изменить пароль</a></p><p>Ссылка действует 1 час и может быть использована один раз. Если вы не запрашивали восстановление, проигнорируйте письмо.</p>"})
}
