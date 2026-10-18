// Package jobs runs the durable queue with a bounded set of independent workers.
package jobs

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"regexp"
	"sync"
	"time"

	domain "github.com/janickiy/meet-space/internal/domain/jobs"
)

// Handler performs the operation; Fail persists a safe terminal failure on the related entity.
type Handler struct {
	Handle func(context.Context, domain.Job) error
	Fail   func(context.Context, domain.Job, string) error
}

// Pool reserves independent concurrency and a timeout for one fixed work category.
type Pool struct {
	Kind        string
	Concurrency int
	MaxAttempts int // Zero retains the row budget; values from one through ten further bound older or
	// trigger-generated jobs.
	Timeout time.Duration
	Handler Handler
}

// Runner polls PostgreSQL using a fixed goroutine count, without creating one timer per
// conference.
type Runner struct {
	drainMu  sync.Mutex
	draining bool
	active   int
	repo     domain.Repository
	pools    []Pool
	interval time.Duration
	observe  func(string, string, time.Duration)
}

// New constructs the component with the dependencies and processing limits required for leased
// background jobs.
//
// Parameters:
//   - repo: Persistence dependency used by the application operation.
//   - pools: Independent bounded background-job execution pools.
//   - interval: Interval between bounded maintenance iterations.
//   - observe: Callback recording a bounded job or provider outcome.
//
// Returns:
//   - result 1: The constructed or loaded runner.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func New(repo domain.Repository, pools []Pool, interval time.Duration, observe func(string, string, time.Duration)) (*Runner, error) {
	if repo == nil || interval < 100*time.Millisecond || interval > time.Minute || len(pools) > 10 {
		return nil, errors.New("invalid job runner configuration")
	}
	seen := map[string]bool{}
	for _, pool := range pools {
		if pool.Kind == "" || seen[pool.Kind] || pool.Concurrency < 1 || pool.Concurrency > 16 || pool.MaxAttempts < 0 || pool.MaxAttempts > 10 || pool.Timeout < time.Second || pool.Timeout > 30*time.Minute || pool.Handler.Handle == nil {
			return nil, errors.New("invalid job pool configuration")
		}
		seen[pool.Kind] = true
	}
	return &Runner{repo: repo, pools: pools, interval: interval, observe: observe}, nil
}

// BeginDrain stops accepting new work while existing operations finish.
func (r *Runner) BeginDrain() { r.drainMu.Lock(); r.draining = true; r.drainMu.Unlock() }

// Active returns the count of component-owned operations still requiring completion before
// shutdown.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
func (r *Runner) Active() int { r.drainMu.Lock(); defer r.drainMu.Unlock(); return r.active }

// Run runs leased background jobs until the context is canceled or processing fails.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
func (r *Runner) Run(ctx context.Context) {
	var workers sync.WaitGroup
	for _, pool := range r.pools {
		for range pool.Concurrency {
			workers.Go(func() { r.work(ctx, pool) })
		}
	}
	workers.Wait()
}

// work processes one background-job category with its separate bounded lease and execution
// policy.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - pool: Execution policy and handlers for the selected job category.
func (r *Runner) work(ctx context.Context, pool Pool) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		r.drainMu.Lock()
		if r.draining {
			r.drainMu.Unlock()
			return
		}
		r.active++
		r.drainMu.Unlock()
		claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		job, found, err := r.repo.Claim(claimCtx, pool.Kind, pool.Timeout+30*time.Second)
		cancel()
		if err == nil && found {
			r.execute(ctx, pool, job)
			r.drainMu.Lock()
			r.active--
			r.drainMu.Unlock()
			continue
		}
		r.drainMu.Lock()
		r.active--
		r.drainMu.Unlock()
		if err != nil && ctx.Err() == nil {
			slog.Warn("product job queue unavailable", "event_type", "product.queue.failed", "kind", pool.Kind)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// execute runs a leased job outside the result transaction and applies its bounded retry policy.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - pool: Execution policy and handlers for the selected job category.
//   - job: Leased background job and its generation or ownership token.
func (r *Runner) execute(ctx context.Context, pool Pool, job domain.Job) {
	start := time.Now()
	if pool.MaxAttempts > 0 && pool.MaxAttempts < job.MaxAttempts {
		job.MaxAttempts = pool.MaxAttempts
	}
	var err error
	if job.Attempts > job.MaxAttempts {
		err = &domain.Error{Code: "attempts_exhausted"}
	} else {
		callCtx, cancel := context.WithTimeout(ctx, pool.Timeout)
		err = callHandler(callCtx, pool.Handler.Handle, job)
		cancel()
	}
	state, code, retryable, wait := classify(err)
	var retryAt *time.Time
	if retryable && job.Attempts < job.MaxAttempts {
		delay := Backoff(job.Attempts, wait)
		next := time.Now().Add(delay)
		retryAt, state = &next, "queued"
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if state == "failed" && pool.Handler.Fail != nil {
		if failure := pool.Handler.Fail(finishCtx, job, code); failure != nil {
			slog.Warn("product failure state not saved", "event_type", "product.failure.pending", "kind", pool.Kind, "job_id", job.ID)
			return // The lease will expire; a subsequent claim preserves terminal state without invoking the
			// provider.
		}
	}
	if finishErr := r.repo.Finish(finishCtx, job, state, code, retryAt); finishErr != nil {
		slog.Warn("product job completion deferred", "event_type", "product.completion.pending", "kind", pool.Kind, "job_id", job.ID)
		return
	}
	if r.observe != nil {
		r.observe(pool.Kind, state, time.Since(start))
	}
	slog.Info("product job completed", "event_type", "product.job.completed", "kind", pool.Kind, "state", state, "job_id", job.ID, "entity_id", job.EntityID, "conference_id", job.ConferenceID, "attempt", job.Attempts, "code", code)
}

var safeCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// callHandler invokes the leased job handler and converts a panic into a controlled worker
// failure.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - handle: Callback executing the leased background job.
//   - job: Leased background job and its generation or ownership token.
//
// Returns:
//   - err: nil on success; otherwise the validation, access, transport or persistence failure.
func callHandler(ctx context.Context, handle func(context.Context, domain.Job) error, job domain.Job) (err error) {
	defer func() {
		if recover() != nil {
			err = domain.Error{Code: "handler_panic"}
		}
	}()
	return handle(ctx, job)
}

// classify converts a handler failure into safe retry metadata without preserving a provider
// response body.
//
// Parameters:
//   - err: Error to classify, wrap or report safely.
//
// Returns:
//   - result 1: Bounded terminal or retry job state.
//   - result 2: Safe failure code without provider or user content.
//   - result 3: Whether this job failure is retryable.
//   - result 4: Provider-requested retry delay, before the runner applies its bounds.
func classify(err error) (string, string, bool, time.Duration) {
	if err == nil {
		return "done", "", false, 0
	}
	if errors.Is(err, domain.ErrSkip) {
		return "skipped", "", false, 0
	}
	if errors.Is(err, domain.ErrLeaseLost) {
		return "skipped", "lease_lost", false, 0
	}
	var failure *domain.Error
	var valueFailure domain.Error
	if errors.As(err, &valueFailure) {
		failure = &valueFailure
	} else {
		_ = errors.As(err, &failure)
	}
	if failure != nil {
		code := failure.Code
		if !safeCode.MatchString(code) {
			code = "provider_failed"
		}
		return "failed", code, failure.Retryable, failure.RetryAfter
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "failed", "operation_timeout", true, 0
	}
	return "failed", "internal_unavailable", true, 0
}

// Backoff computes the bounded retry delay for the background-job attempt.
//
// Parameters:
//   - attempt: Current retry attempt.
//   - requested: Requested retry delay before bounding it to the supported policy.
//
// Returns:
//   - result 1: The duration or bounded waiting interval computed by the operation.
func Backoff(attempt int, requested time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 10 {
		attempt = 10
	}
	base := time.Second * time.Duration(1<<uint(attempt))
	delay := base/2 + time.Duration(rand.Int64N(int64(base/2)+1))
	if requested > delay {
		delay = requested
	}
	if delay > time.Hour {
		delay = time.Hour
	}
	return delay
}
