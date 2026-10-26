package auth

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
)

type auditUsers struct{}

func (auditUsers) Create(_ context.Context, user users.User) (users.User, error) { return user, nil }
func (auditUsers) GetByID(context.Context, string) (users.User, error) {
	return users.User{ID: "00000000-0000-4000-8000-000000000001", PasswordHash: "synthetic-hash"}, nil
}
func (auditUsers) GetByEmail(ctx context.Context, email string) (users.User, error) {
	if email == "existing@example.invalid" {
		return (auditUsers{}).GetByID(ctx, "")
	}
	return users.User{}, apperrors.ErrNotFound
}
func (auditUsers) UpdateDisplayName(context.Context, string, string) (users.User, error) {
	return users.User{}, nil
}
func (auditUsers) DeleteAccount(context.Context, string, string) error { return nil }

type auditTokens struct{}

func (auditTokens) Issue(string) (string, error) { return "synthetic-token", nil }

type blockingPasswords struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int64
	active  atomic.Int64
	peak    atomic.Int64
	workErr error
	valid   bool
}

func (p *blockingPasswords) work() error {
	p.calls.Add(1)
	active := p.active.Add(1)
	defer p.active.Add(-1)
	for previous := p.peak.Load(); active > previous; previous = p.peak.Load() {
		if p.peak.CompareAndSwap(previous, active) {
			break
		}
	}
	if p.entered != nil {
		p.entered <- struct{}{}
	}
	if p.release != nil {
		<-p.release
	}
	return p.workErr
}

func (p *blockingPasswords) Hash(password string) (string, error) {
	if password == "dummy-password-for-login-timing" {
		return "synthetic-dummy-hash", nil
	}
	return "synthetic-hash", p.work()
}
func (p *blockingPasswords) Verify(string, string) (bool, error) { return p.valid, p.work() }

func newAuditAuth(t *testing.T, passwords *blockingPasswords) *Service {
	t.Helper()
	s, err := NewService(auditUsers{}, passwords, auditTokens{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func auditPasswordOperations(s *Service) map[string]func(context.Context) error {
	return map[string]func(context.Context) error{
		"register": func(ctx context.Context) error {
			_, err := s.Register(ctx, users.RegisterRequest{Email: "audit@example.invalid", Password: "synthetic-password"})
			return err
		},
		"login": func(ctx context.Context) error {
			_, err := s.Login(ctx, users.LoginRequest{Email: "missing@example.invalid", Password: "synthetic-password"})
			return err
		},
		"delete": func(ctx context.Context) error {
			return s.DeleteAccount(ctx, "00000000-0000-4000-8000-000000000001", "synthetic-password")
		},
	}
}

// TestPasswordWorkRejectsOverload proves that all password entry points share a
// bounded budget without allocating Argon2's actual memory during the test.
func TestPasswordWorkRejectsOverload(t *testing.T) {
	for _, candidate := range []string{"register", "login", "delete"} {
		t.Run(candidate, func(t *testing.T) {
			passwords := &blockingPasswords{entered: make(chan struct{}, 3), release: make(chan struct{})}
			s := newAuditAuth(t, passwords)
			operations := auditPasswordOperations(s)
			finished := make(chan error, 3)
			defer func() {
				close(passwords.release)
				for range 3 {
					select {
					case <-finished:
					case <-time.After(5 * time.Second):
						t.Error("password operation did not finish")
					}
				}
			}()
			for _, name := range []string{"register", "login"} {
				go func() { finished <- operations[name](context.Background()) }()
				select {
				case <-passwords.entered:
				case <-time.After(5 * time.Second):
					t.Fatal("password operation did not start")
				}
			}
			go func() { finished <- operations[candidate](context.Background()) }()
			select {
			case err := <-finished:
				finished <- err
				if !errors.Is(err, apperrors.ErrRateLimited) {
					t.Fatalf("overload error = %v, want rate limited", err)
				}
			case <-passwords.entered:
				t.Fatalf("overloaded request entered password work: peak=%d, want <=2", passwords.peak.Load())
			case <-time.After(5 * time.Second):
				t.Fatal("overloaded request queued instead of returning promptly")
			}
			if peak := passwords.peak.Load(); peak != 2 {
				t.Fatalf("peak password work = %d, want 2", peak)
			}
		})
	}
}

// TestCancelledPasswordWorkDoesNotHash prevents cancelled HTTP requests from
// starting an expensive, non-cancellable password derivation.
func TestCancelledPasswordWorkDoesNotHash(t *testing.T) {
	for _, name := range []string{"register", "login", "delete"} {
		t.Run(name, func(t *testing.T) {
			passwords := &blockingPasswords{}
			s := newAuditAuth(t, passwords)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := auditPasswordOperations(s)[name](ctx)
			if !errors.Is(err, context.Canceled) || passwords.calls.Load() != 0 {
				t.Fatalf("error=%v, password calls=%d; want cancellation before hashing", err, passwords.calls.Load())
			}
		})
	}
}

// TestPasswordWorkReleasesCapacity verifies failed derivations do not exhaust
// the per-service budget and disable subsequent login attempts.
func TestPasswordWorkReleasesCapacity(t *testing.T) {
	want := errors.New("synthetic hash failure")
	passwords := &blockingPasswords{workErr: want}
	s := newAuditAuth(t, passwords)
	for range 8 {
		for name, operation := range auditPasswordOperations(s) {
			if err := operation(context.Background()); !errors.Is(err, want) {
				t.Fatalf("%s error = %v, want original hash failure", name, err)
			}
		}
	}
}

// TestPasswordWorkSequentialSuccess preserves normal registration, login and
// password-confirmed deletion while proving successful work releases slots.
func TestPasswordWorkSequentialSuccess(t *testing.T) {
	passwords := &blockingPasswords{valid: true}
	s := newAuditAuth(t, passwords)
	for range 8 {
		if _, err := s.Register(context.Background(), users.RegisterRequest{Email: "audit@example.invalid", Password: "synthetic-password"}); err != nil {
			t.Fatal(err)
		}
		login, err := s.Login(context.Background(), users.LoginRequest{Email: "existing@example.invalid", Password: "synthetic-password"})
		if err != nil || login.AccessToken != "synthetic-token" {
			t.Fatalf("login returned error=%v token=%q", err, login.AccessToken)
		}
		if err := s.DeleteAccount(context.Background(), login.User.ID, "synthetic-password"); err != nil {
			t.Fatal(err)
		}
	}
	if got := passwords.calls.Load(); got != 24 {
		t.Fatalf("password calls=%d, want 24", got)
	}
}
