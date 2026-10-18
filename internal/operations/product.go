package operations

import (
	"context"
	"errors"
	"time"

	"github.com/janickiy/meet-space/internal/domain/jobs"
)

// productKind maps a product job kind to the bounded label used by operational metrics.
//
// Parameters:
//   - kind: Validated event, job, media or resource category.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func productKind(kind string) bool {
	switch kind {
	case "integrations.conference", "integrations.event", "integrations.delivery", "integrations.calendar", "content.transcribe", "content.summarize", "content.embed", "analytics.aggregate":
		return true
	}
	return false
}

// Product records the bounded outcome of a product background-job category.
//
// Parameters:
//   - kind: Validated event, job, media or resource category.
//   - outcome: Fixed operational outcome label.
//   - duration: Duration or validity window used by the operation.
func Product(kind, outcome string, duration time.Duration) {
	if !productKind(kind) {
		return
	}
	switch outcome {
	case "done", "failed", "skipped", "queued":
	default:
		return
	}
	if r := current.Load(); r != nil {
		r.productJobs.WithLabelValues(kind, outcome).Inc()
		r.productDuration.WithLabelValues(kind).Observe(duration.Seconds())
	}
}

// ProductQueue updates the bounded queue-depth metrics for a fixed product-job category.
//
// Parameters:
//   - kind: Validated event, job, media or resource category.
//   - state: Current state or validated state update.
//   - count: Expected or maximum number of items.
func ProductQueue(kind, state string, count int64) {
	if !productKind(kind) || (state != "queued" && state != "processing" && state != "failed") {
		return
	}
	if r := current.Load(); r != nil {
		r.productQueue.WithLabelValues(kind, state).Set(float64(count))
	}
}

// ProviderCall records a provider-call outcome using fixed technical labels rather than private
// request data.
//
// Parameters:
//   - provider: Operator-selected provider identifier or fixed technical provider label.
//   - duration: Duration or validity window used by the operation.
//   - err: Error to classify, wrap or report safely.
func ProviderCall(provider string, duration time.Duration, err error) {
	switch provider {
	case "email", "push", "calendar", "stt", "ai", "embedding", "live_stt":
	default:
		return
	}
	outcome := "success"
	if err != nil {
		outcome = "retryable"
		var value jobs.Error
		var pointer *jobs.Error
		if errors.As(err, &pointer) && pointer != nil {
			value = *pointer
		} else {
			_ = errors.As(err, &value)
		}
		switch {
		case errors.Is(err, jobs.ErrSkip):
			outcome = "skipped"
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			outcome = "timeout"
		case value.Code == "provider_rate_limited":
			outcome = "rate_limited"
		case value.Code != "" && !value.Retryable:
			outcome = "permanent"
		}
	}
	if r := current.Load(); r != nil {
		r.providerCalls.WithLabelValues(provider, outcome).Inc()
		r.providerTime.WithLabelValues(provider).Observe(duration.Seconds())
	}
}

// Search records search latency and a bounded success or failure label without retaining the
// query text.
//
// Parameters:
//   - duration: Duration or validity window used by the operation.
//   - failed: Whether the bounded operation failed.
func Search(duration time.Duration, failed bool) {
	outcome := "success"
	if failed {
		outcome = "failed"
	}
	if r := current.Load(); r != nil {
		r.searchRequests.WithLabelValues(outcome).Inc()
		r.searchTime.Observe(duration.Seconds())
	}
}
