// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	platformsettings "github.com/janickiy/meet-space/internal/domain/settings"
	"github.com/janickiy/meet-space/internal/domain/users"
)

type modeLDAPProbe struct {
	user         users.User
	err          error
	email, login string
	afterAuth    func()
}

func (p *modeLDAPProbe) Authenticate(_ context.Context, login, password string) (users.User, error) {
	p.login = login
	if p.afterAuth != nil {
		p.afterAuth()
	}
	return p.user, p.err
}
func (p *modeLDAPProbe) AuthenticateEmail(_ context.Context, email, password string) (users.User, error) {
	p.email = email
	if p.afterAuth != nil {
		p.afterAuth()
	}
	return p.user, p.err
}

type modeSessionProbe struct {
	SessionRepository
	created []users.AuthSession
}

func (p *modeSessionProbe) Create(_ context.Context, session users.AuthSession) error {
	p.created = append(p.created, session)
	return nil
}

type modeTokens struct {
	auditTokens
	sessionTokens
}

func (modeTokens) IssueSession(string, string) (string, error) { return "session-token", nil }

func modeService(t *testing.T, ldapOnly bool) (*Service, *recoveryPolicy, *blockedUsers, *blockingPasswords, *modeLDAPProbe, *modeSessionProbe) {
	t.Helper()
	repo := &blockedUsers{user: users.User{ID: "local-user", Email: "person@example.invalid", PasswordHash: "local-hash"}}
	passwords := &blockingPasswords{valid: true}
	service, err := NewService(repo, passwords, modeTokens{})
	if err != nil {
		t.Fatal(err)
	}
	policy := &recoveryPolicy{values: platformsettings.Values{LDAPEnabled: ldapOnly}}
	ldap := &modeLDAPProbe{user: users.User{ID: "ldap-user", AuthSource: "ldap", Email: "person@example.invalid", LDAPRevision: 7}}
	sessions := &modeSessionProbe{}
	service.WithAuthenticationPolicy(policy).WithLDAP(ldap).WithSessions(sessions)
	return service, policy, repo, passwords, ldap, sessions
}

func TestUnifiedLoginSelectsExclusiveMode(t *testing.T) {
	for _, ldapOnly := range []bool{false, true} {
		service, _, repo, passwords, ldap, sessions := modeService(t, ldapOnly)
		response, err := service.Login(context.Background(), users.LoginRequest{Email: " Person@Example.Invalid ", Password: "valid-password"})
		if err != nil || response.AccessToken == "" || response.SessionToken == "" || len(sessions.created) != 1 {
			t.Fatalf("mode %v: login failed: %v", ldapOnly, err)
		}
		if ldapOnly {
			if ldap.email != "person@example.invalid" || ldap.login != "" || repo.lookedUp != "" || passwords.calls.Load() != 0 || response.User.ID != "ldap-user" {
				t.Fatal("LDAP email login used the local password or username path")
			}
		} else if ldap.email != "" || ldap.login != "" || repo.lookedUp != "person@example.invalid" || passwords.calls.Load() != 1 || response.User.ID != "local-user" {
			t.Fatal("local login reached LDAP or skipped password verification")
		}
	}
}

func TestLDAPFailureCannotFallBackToLocalPassword(t *testing.T) {
	for _, failure := range []error{apperrors.ErrUnauthorized, apperrors.ErrUnavailable} {
		service, _, repo, passwords, ldap, sessions := modeService(t, true)
		ldap.err = failure
		response, err := service.Login(context.Background(), users.LoginRequest{Email: "person@example.invalid", Password: "valid-local-password"})
		if !errors.Is(err, failure) || response.AccessToken != "" || response.SessionToken != "" || len(sessions.created) != 0 || repo.lookedUp != "" || passwords.calls.Load() != 0 {
			t.Fatalf("LDAP failure fell back to local account: %v", err)
		}
		if errors.Is(failure, apperrors.ErrUnauthorized) && err.Error() != invalidCredentials().Error() {
			t.Fatal("unified endpoint exposed provider-specific credential errors")
		}
	}
}

func TestUnifiedLDAPLoginRequiresEmail(t *testing.T) {
	for _, email := range []string{"employee", "Employee <person@example.invalid>", "person@example.invalid\x00"} {
		service, _, _, _, ldap, sessions := modeService(t, true)
		_, err := service.Login(context.Background(), users.LoginRequest{Email: email, Password: "password"})
		if !errors.Is(err, apperrors.ErrUnauthorized) || ldap.email != "" || len(sessions.created) != 0 {
			t.Fatalf("accepted non-email identifier %q: %v", email, err)
		}
	}
}

func TestLocalOnlyModeRejectsLegacyLDAPEndpoint(t *testing.T) {
	service, _, _, _, ldap, sessions := modeService(t, false)
	_, err := service.LDAPLogin(context.Background(), users.LDAPLoginRequest{Login: "employee", Password: "password"})
	if !errors.Is(err, apperrors.ErrForbidden) || ldap.login != "" || len(sessions.created) != 0 {
		t.Fatalf("legacy LDAP endpoint bypassed local mode: %v", err)
	}
}

func TestLocalOnlyModeRejectsLDAPAccountPassword(t *testing.T) {
	service, _, repo, _, ldap, sessions := modeService(t, false)
	repo.user = ldap.user
	_, err := service.Login(context.Background(), users.LoginRequest{Email: "person@example.invalid", Password: "password"})
	if !errors.Is(err, apperrors.ErrUnauthorized) || ldap.email != "" || len(sessions.created) != 0 {
		t.Fatalf("LDAP profile used local authentication: %v", err)
	}
}

func TestAuthenticationModeChangePreventsSessionCreation(t *testing.T) {
	service, policy, _, _, ldap, sessions := modeService(t, true)
	ldap.afterAuth = func() { policy.values.LDAPEnabled = false }
	response, err := service.Login(context.Background(), users.LoginRequest{Email: "person@example.invalid", Password: "password"})
	if !errors.Is(err, apperrors.ErrUnauthorized) || response.AccessToken != "" || len(sessions.created) != 0 {
		t.Fatalf("mode change during LDAP bind issued a session: %v", err)
	}
}

func TestLDAPOnlyRejectsLocalAccountFlowsAndLegacyToken(t *testing.T) {
	service, _, repo, passwords, _, sessions := modeService(t, true)
	operations := map[string]func() error{
		"register": func() error {
			_, err := service.Register(context.Background(), users.RegisterRequest{Email: "new@example.invalid", Password: "valid-password"})
			return err
		},
		"recovery": func() error { return service.PasswordRecovery(context.Background(), "person@example.invalid") },
		"reset": func() error {
			return service.ResetPassword(context.Background(), "prior-reset-token", "valid-password")
		},
		"verify": func() error { return service.VerifyEmail(context.Background(), "prior-verification-token") },
		"resend": func() error { return service.ResendVerification(context.Background(), "person@example.invalid") },
	}
	for name, operation := range operations {
		if err := operation(); !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatalf("%s allowed in LDAP mode: %v", name, err)
		}
	}
	if repo.lookedUp != "" || repo.created != 0 || passwords.calls.Load() != 0 || len(sessions.created) != 0 {
		t.Fatal("disabled local flow read or modified credentials")
	}
	if err := service.AuthorizeLegacyAccount(context.Background(), repo.user.ID); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatalf("local stateless token accepted in LDAP mode: %v", err)
	}
}

func TestUnknownAuthenticationPolicyFailsClosed(t *testing.T) {
	service, _, repo, passwords, ldap, sessions := modeService(t, false)
	service.WithAuthenticationPolicy(&unavailableVerification{})
	_, err := service.Login(context.Background(), users.LoginRequest{Email: "person@example.invalid", Password: "password"})
	if !errors.Is(err, apperrors.ErrUnavailable) || repo.lookedUp != "" || passwords.calls.Load() != 0 || ldap.email != "" || len(sessions.created) != 0 {
		t.Fatalf("unknown mode selected a provider: %v", err)
	}
}

func TestLDAPOnlySkipsQueuedLocalCredentialEmails(t *testing.T) {
	policy := configuredRecoveryPolicy()
	policy.values.LDAPEnabled = true
	policy.values.EmailVerificationRequired = true
	// Отсутствие репозиториев и провайдеров также подтверждает отказ до чтения задания и отправки письма.
	if err := (VerificationSender{Reader: policy}).Handle(context.Background(), jobs.Job{}); !errors.Is(err, jobs.ErrSkip) {
		t.Fatalf("verification job was not skipped: %v", err)
	}
	for _, kind := range []string{PasswordRecoveryJobKind, PasswordChangedJobKind} {
		if err := (PasswordRecoverySender{Reader: policy}).Handle(context.Background(), jobs.Job{Kind: kind}); !errors.Is(err, jobs.ErrSkip) {
			t.Fatalf("password job was not skipped: %v", err)
		}
	}
}
