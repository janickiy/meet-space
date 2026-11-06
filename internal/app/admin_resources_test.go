package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/operations"
)

func blockedAdminProbe(t *testing.T) (*adminResources, func()) {
	t.Helper()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	r := &adminResources{cache: map[string]adminCached{
		"files": {value: admin.Files{Count: 42}, expires: time.Now().Add(time.Minute)},
	}, checks: map[string]operations.Check{"API": func(ctx context.Context) error {
		once.Do(func() { close(entered) })
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}}
	go func() { defer close(done); _, _ = r.Query(context.Background(), "system", admin.Query{}) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("probe did not start")
	}
	return r, func() { close(release); <-done }
}

func TestAdminCacheInvalidationDoesNotResurrectStaleResult(t *testing.T) {
	r := &adminResources{}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		_, _ = r.cachedQuery(context.Background(), "files", func() (any, error) {
			close(entered)
			<-release
			return admin.Files{Count: 42}, nil
		})
	}()
	<-entered
	r.invalidateCache() // A recording was deleted while the aggregate was loading.
	close(release)
	<-done
	v, err := r.cachedQuery(context.Background(), "files", func() (any, error) {
		return admin.Files{Count: 41}, nil
	})
	if err != nil || v.(admin.Files).Count != 41 {
		t.Fatalf("stale aggregate survived deletion: %v %v", v, err)
	}
}

func TestAdminCacheConcurrentRequestsShareProbe(t *testing.T) {
	var probes atomic.Int64
	r := &adminResources{checks: map[string]operations.Check{"API": func(context.Context) error { probes.Add(1); return nil }}}
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			if _, err := r.Query(context.Background(), "system", admin.Query{}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if probes.Load() != 1 {
		t.Fatalf("concurrent cache miss started %d probes", probes.Load())
	}
}

func TestAdminCacheWaitHonorsCancellation(t *testing.T) {
	r, release := blockedAdminProbe(t)
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { _, err := r.Query(ctx, "system", admin.Query{}); done <- err }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled cache wait: %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("cancelled request remained blocked behind an unrelated probe")
	}
}

func TestAdminCachedFilesDoNotWaitForSystemProbe(t *testing.T) {
	r, release := blockedAdminProbe(t)
	defer release()
	done := make(chan error, 1)
	go func() {
		value, err := r.Query(context.Background(), "files", admin.Query{})
		if err == nil && value.(admin.Files).Count != 42 {
			err = errors.New("cached files changed")
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("cached files blocked behind a system probe")
	}
}
