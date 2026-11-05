package admin

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	domain "github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

type auditPasswords struct{}

func (auditPasswords) Hash(v string) (string, error)    { return "hash:" + v, nil }
func (auditPasswords) Verify(v, h string) (bool, error) { return h == "hash:"+v, nil }

type passwordAuditRepo struct {
	domain.Repository
	account  domain.Administrator
	changed  bool
	expected string
}

func TestPasswordWorkConcurrencyBudget(t *testing.T) {
	svc, err := NewService(nil, auditPasswords{})
	if err != nil {
		t.Fatal(err)
	}
	const requests = 64
	for round := 0; round < 3; round++ {
		var finished sync.WaitGroup
		var active, peak atomic.Int32
		entered := make(chan struct{}, 2)
		release := make(chan struct{})
		results := make(chan error, requests)
		finished.Add(requests)
		for i := 0; i < requests; i++ {
			go func() {
				defer finished.Done()
				results <- svc.passwordWork(context.Background(), func() error {
					n := active.Add(1)
					for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
					}
					entered <- struct{}{}
					<-release
					active.Add(-1)
					return nil
				})
			}()
		}
		<-entered
		<-entered
		for i := 0; i < requests-2; i++ {
			if err := <-results; !errors.Is(err, apperrors.ErrRateLimited) {
				t.Fatalf("overflow queued or entered password worker: %v", err)
			}
		}
		close(release)
		finished.Wait()
		for i := 0; i < 2; i++ {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		if peak.Load() != 2 || active.Load() != 0 || len(svc.work) != 0 {
			t.Fatalf("password work leaked capacity: peak=%d active=%d slots=%d", peak.Load(), active.Load(), len(svc.work))
		}
	}
	t.Log("3 waves × 64 requests: exactly 2 concurrent workers and 62 immediate rejections per wave; no retained worker slots")
}

func TestPasswordWorkCancellationAndFailureReleaseSlots(t *testing.T) {
	svc, err := NewService(nil, auditPasswords{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = svc.passwordWork(ctx, func() error { t.Fatal("canceled work started"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	failed := errors.New("synthetic password operation failure")
	if err = svc.passwordWork(context.Background(), func() error { return failed }); !errors.Is(err, failed) {
		t.Fatalf("failure lost: %v", err)
	}
	if len(svc.work) != 0 {
		t.Fatal("password worker retained after error")
	}
}

func (r *passwordAuditRepo) ByID(_ context.Context, id string) (domain.Administrator, error) {
	return r.account, nil
}
func (r *passwordAuditRepo) ChangePassword(_ context.Context, id, hash, expected string, _ domain.AuditInput) error {
	r.changed = true
	r.expected = expected
	return nil
}
func TestSelfPasswordUUIDAliasesRequireCurrentPassword(t *testing.T) {
	const id = "aabbccdd-1122-4334-8556-aabbccddeeff"
	for _, alias := range []string{strings.ToUpper(id), "{" + id + "}"} {
		t.Run(alias, func(t *testing.T) {
			repo := &passwordAuditRepo{account: domain.Administrator{ID: id, Username: "admin", PasswordHash: "hash:PreviousPassword123", Status: "active"}}
			svc, err := NewService(repo, auditPasswords{})
			if err != nil {
				t.Fatal(err)
			}
			err = svc.Password(context.Background(), alias, "", "ChangedPassword123", domain.AuditInput{AdministratorID: id, Username: "admin"}, false)
			if !errors.Is(err, apperrors.ErrUnauthorized) || repo.changed {
				t.Fatalf("self password changed without current password via UUID alias; changed=%v error=%v", repo.changed, err)
			}
		})
	}
}
