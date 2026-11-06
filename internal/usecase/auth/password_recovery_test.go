package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	platformsettings "github.com/janickiy/meet-space/internal/domain/settings"
	"github.com/janickiy/meet-space/internal/domain/users"
	"github.com/janickiy/meet-space/internal/infrastructure/security"
)

type recoveryPolicy struct{ values platformsettings.Values }

func (p *recoveryPolicy) Snapshot(context.Context) (platformsettings.Values, error) {
	return p.values, nil
}

type recoveryRepositoryProbe struct {
	requests int
	value    PasswordRecovery
}

func (r *recoveryRepositoryProbe) Request(_ context.Context, _ users.User, value PasswordRecovery) error {
	r.requests++
	r.value = value
	return nil
}
func (r *recoveryRepositoryProbe) Consume(context.Context, string, string) (string, error) {
	return "", nil
}
func (r *recoveryRepositoryProbe) Delivery(context.Context, jobs.Job) (PasswordRecoveryDelivery, error) {
	return PasswordRecoveryDelivery{}, nil
}
func (r *recoveryRepositoryProbe) ChangedDelivery(context.Context, jobs.Job) (string, error) {
	return "", nil
}

type recoveryCipherProbe struct {
	calls int
	err   error
}

func (c *recoveryCipherProbe) Encrypt(string, string) (string, error) {
	c.calls++
	return "synthetic-ciphertext", c.err
}
func (c *recoveryCipherProbe) Decrypt(string, string) (string, error) { return "", nil }
func configuredRecoveryPolicy() *recoveryPolicy {
	return &recoveryPolicy{values: platformsettings.Values{SMTPEnabled: true, SMTPHost: "smtp.example.invalid", SMTPPort: 465, SMTPUsername: "sender", SMTPFrom: "sender@example.invalid", SMTPTLSMode: "tls", SMTPPasswordConfigured: true}}
}
func TestPasswordRecoveryConfiguredSMTPAndNonEnumeratingMainCryptoPath(t *testing.T) {
	for _, missing := range []bool{false, true} {
		repo := &blockedUsers{user: users.User{ID: "user", Email: "test@example.invalid", PasswordHash: "savedhash"}}
		if missing {
			repo.err = apperrors.ErrNotFound
		}
		service, err := NewService(repo, &blockingPasswords{}, auditTokens{})
		if err != nil {
			t.Fatal(err)
		}
		requestRepo := &recoveryRepositoryProbe{}
		cipher := &recoveryCipherProbe{}
		service.WithPasswordRecovery(configuredRecoveryPolicy(), requestRepo, cipher)
		if err = service.PasswordRecovery(context.Background(), " TEST@EXAMPLE.INVALID "); err != nil {
			t.Fatal(err)
		}
		expected := 1
		if missing {
			expected = 0
		}
		if cipher.calls != 1 || requestRepo.requests != expected || repo.lookedUp != "test@example.invalid" {
			t.Fatal("main cryptographic path differs or wrong account queued")
		}
		if !missing && (len(requestRepo.value.TokenHash) != 64 || requestRepo.value.TokenCiphertext != "synthetic-ciphertext") {
			t.Fatal("credential digest or encryption missing")
		}
	}
}
func TestPasswordRecoveryUnavailableWithoutCompleteSMTPAndCipher(t *testing.T) {
	var typedNil *security.ProviderTokens
	for _, test := range []struct {
		name   string
		change func(*recoveryPolicy)
		cipher VerificationCipher
	}{
		{name: "nil cipher"},
		{name: "typed nil cipher", cipher: typedNil},
		{name: "encryption failure", cipher: &recoveryCipherProbe{err: errors.New("synthetic-key-error")}},
		{name: "disabled SMTP", cipher: &recoveryCipherProbe{}, change: func(p *recoveryPolicy) { p.values.SMTPEnabled = false }},
		{name: "missing password", cipher: &recoveryCipherProbe{}, change: func(p *recoveryPolicy) { p.values.SMTPPasswordConfigured = false }},
		{name: "missing host", cipher: &recoveryCipherProbe{}, change: func(p *recoveryPolicy) { p.values.SMTPHost = "" }},
		{name: "missing username", cipher: &recoveryCipherProbe{}, change: func(p *recoveryPolicy) { p.values.SMTPUsername = "" }},
		{name: "missing from", cipher: &recoveryCipherProbe{}, change: func(p *recoveryPolicy) { p.values.SMTPFrom = "" }},
		{name: "invalid port", cipher: &recoveryCipherProbe{}, change: func(p *recoveryPolicy) { p.values.SMTPPort = 65536 }},
		{name: "invalid TLS", cipher: &recoveryCipherProbe{}, change: func(p *recoveryPolicy) { p.values.SMTPTLSMode = "none" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewService(&blockedUsers{user: users.User{ID: "user", PasswordHash: "savedhash"}}, &blockingPasswords{}, auditTokens{})
			if err != nil {
				t.Fatal(err)
			}
			reader := configuredRecoveryPolicy()
			if test.change != nil {
				test.change(reader)
			}
			repo := &recoveryRepositoryProbe{}
			service.WithPasswordRecovery(reader, repo, test.cipher)
			err = service.PasswordRecovery(context.Background(), "test@example.invalid")
			if !errors.Is(err, apperrors.ErrUnavailable) || err.Error() != "password recovery unavailable" || repo.requests != 0 {
				t.Fatal("recovery unavailable contract changed", err)
			}
		})
	}
}
func TestPasswordResetRejectsMalformedTokenBeforePasswordHash(t *testing.T) {
	passwords := &blockingPasswords{}
	service, err := NewService(&blockedUsers{}, passwords, auditTokens{})
	if err != nil {
		t.Fatal(err)
	}
	service.WithPasswordRecovery(configuredRecoveryPolicy(), &recoveryRepositoryProbe{}, &recoveryCipherProbe{})
	for _, token := range []string{"", strings.Repeat("a", 50000), strings.Repeat("a", 42), strings.Repeat("a", 44)} {
		if err = service.ResetPassword(context.Background(), token, "ValidPassword123"); !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Fatal("malformed reset accepted", err)
		}
	}
	if passwords.calls.Load() != 0 {
		t.Fatal("malformed token performed expensive password work")
	}
}
