// Package jobs defines transport-independent durable background-job contracts.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	// ErrSkip identifies an intentionally skipped disabled or obsolete operation.
	ErrSkip = errors.New("job skipped")
	// ErrLeaseLost prevents job completion after ownership changes.
	ErrLeaseLost = errors.New("job lease lost")
)

// Job contains entity references and a bounded payload, never tokens or transcript text. Version
// binds work to an entity generation; LeaseToken fences stale workers. Attempts counts claims,
// including recovery after a crash.
type Job struct {
	ID           string          `json:"id"`
	Kind         string          `json:"kind"`
	EntityID     string          `json:"entityId"`
	ConferenceID string          `json:"conferenceId"`
	UserID       *string         `json:"userId,omitempty"`
	Version      int64           `json:"version"`
	Payload      json.RawMessage `json:"payload" gorm:"type:jsonb"`
	DedupKey     string          `json:"dedupKey"`
	Attempts     int             `json:"attempts"`
	MaxAttempts  int             `json:"maxAttempts"`
	LeaseToken   string          `json:"-"`
	LeaseUntil   time.Time       `json:"-"`
	CreatedAt    time.Time       `json:"createdAt"`
}

// Count records the number of jobs in a fixed technical category without user-specific labels.
type Count struct {
	Kind  string
	State string
	Count int64
}

// Repository atomically leases jobs and accepts completion only from the current lease owner.
type Repository interface {
	// Claim atomically acquires the current jobs lease or ownership record.
	//
	// Parameters:
	//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
	//   - kind: Validated event, job, media or resource category.
	//   - lease: Current lease identity used to fence stale workers.
	//
	// Returns:
	//   - result 1: The resulting job.
	//   - result 2: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Claim(ctx context.Context, kind string, lease time.Duration) (Job, bool, error)
	// Finish records completion of the leased jobs operation.
	//
	// Parameters:
	//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
	//   - job: Leased background job and its generation or ownership token.
	//   - state: Current state or validated state update.
	//   - code: Bounded public status or failure code.
	//   - retryAt: Next eligible retry time; nil means no scheduled retry.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Finish(ctx context.Context, job Job, state, code string, retryAt *time.Time) error
	// Counts returns the bounded queue-state counts used by operational monitoring.
	//
	// Parameters:
	//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: The collected count values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Counts(ctx context.Context) ([]Count, error)
}

// Error contains a safe failure code, retry eligibility and bounded provider delay. Code must
// not contain tokens, external response bodies or user text.
type Error struct {
	Code       string
	Retryable  bool
	RetryAfter time.Duration
}

// Error returns the error text required by the error interface.
//
// Returns:
//   - result 1: Error text exposed by this error implementation.
func (e Error) Error() string { return e.Code }
