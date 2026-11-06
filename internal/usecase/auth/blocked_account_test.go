package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	platformsettings "github.com/janickiy/meet-space/internal/domain/settings"
	"github.com/janickiy/meet-space/internal/domain/users"
)

type blockedUsers struct {
	auditUsers
	user     users.User
	err      error
	created  int
	lookedUp string
}

func (r *blockedUsers) GetByEmail(_ context.Context, email string) (users.User, error) {
	r.lookedUp = email
	return r.user, r.err
}
func (r *blockedUsers) GetByID(context.Context, string) (users.User, error) { return r.user, r.err }
func (r *blockedUsers) Create(_ context.Context, user users.User) (users.User, error) {
	r.created++
	return user, nil
}

type unavailableVerification struct{ calls int }

func (p *unavailableVerification) Snapshot(context.Context) (platformsettings.Values, error) {
	p.calls++
	return platformsettings.Values{}, apperrors.ErrUnavailable
}

func TestBlockedLoginHasExplicitStatusWithoutIssuingCredentials(t *testing.T) {
	repo := &blockedUsers{user: users.User{ID: "blocked-user", Email: "blocked@example.invalid", PasswordHash: "saved-hash", IsBlocked: true}}
	passwords := &blockingPasswords{}
	service, err := NewService(repo, passwords, auditTokens{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Login(context.Background(), users.LoginRequest{Email: " BLOCKED@EXAMPLE.INVALID ", Password: "supplied-password"})
	if !errors.Is(err, apperrors.ErrAccountBlocked) || !errors.Is(err, apperrors.ErrForbidden) || result.AccessToken != "" || result.SessionToken != "" {
		t.Fatalf("blocked login returned credentials or wrong error: %v", err)
	}
	if passwords.calls.Load() != 1 || repo.lookedUp != "blocked@example.invalid" {
		t.Fatal("blocked login skipped normalized lookup/password work")
	}
}

func TestDuplicateBlockedEmailPrecedesUnavailableVerification(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		repo := &blockedUsers{user: users.User{ID: "existing-user", IsBlocked: blocked}}
		passwords := &blockingPasswords{}
		service, err := NewService(repo, passwords, auditTokens{})
		if err != nil {
			t.Fatal(err)
		}
		policy := &unavailableVerification{}
		service.WithEmailVerification(policy, nil, nil)
		_, err = service.Register(context.Background(), users.RegisterRequest{Email: " Existing@Example.Invalid ", Password: "new-password"})
		if !errors.Is(err, apperrors.ErrConflict) || err.Error() != "email is already registered" {
			t.Fatalf("duplicate error: %v", err)
		}
		if repo.created != 0 || passwords.calls.Load() != 0 || policy.calls != 0 || repo.lookedUp != "existing@example.invalid" {
			t.Fatal("duplicate registration performed password, creation or verification work")
		}
	}
}

func TestPasswordRecoveryRejectsBlockedAndNeverClaimsDelivery(t *testing.T) {
	for _, test := range []struct {
		name            string
		user            users.User
		lookupErr, want error
	}{
		{"blocked", users.User{IsBlocked: true}, nil, apperrors.ErrAccountBlocked},
		{"active", users.User{}, nil, apperrors.ErrUnavailable},
		{"missing", users.User{}, apperrors.ErrNotFound, apperrors.ErrUnavailable},
		{"database failure", users.User{}, apperrors.ErrTimeout, apperrors.ErrTimeout},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &blockedUsers{user: test.user, err: test.lookupErr}
			passwords := &blockingPasswords{}
			service, err := NewService(repo, passwords, auditTokens{})
			if err != nil {
				t.Fatal(err)
			}
			err = service.PasswordRecovery(context.Background(), " Test@Example.Invalid ")
			if !errors.Is(err, test.want) || repo.lookedUp != "test@example.invalid" {
				t.Fatalf("recovery error: %v", err)
			}
			if repo.created != 0 || passwords.calls.Load() != 0 {
				t.Fatal("recovery mutated credentials")
			}
			if err := service.PasswordRecovery(context.Background(), "invalid"); !errors.Is(err, apperrors.ErrInvalidInput) {
				t.Fatalf("invalid recovery email: %v", err)
			}
		})
	}
}
