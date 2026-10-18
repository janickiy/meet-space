package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"gorm.io/gorm"
)

// JobRepository stores the independent product queue without changing RabbitMQ media commands.
type JobRepository struct{ db *gorm.DB }

// NewJobRepository constructs the job repository with the dependencies and limits required for
// job.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//
// Returns:
//   - result 1: The constructed or loaded job repository.
func NewJobRepository(db *gorm.DB) *JobRepository { return &JobRepository{db: db} }

// Claim atomically acquires the current job lease or ownership record.
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
func (r *JobRepository) Claim(ctx context.Context, kind string, lease time.Duration) (jobs.Job, bool, error) {
	var job jobs.Job
	result := r.db.WithContext(ctx).Raw(`UPDATE background_jobs SET state='processing',attempts=attempts+1,
        lease_token=?::uuid,lease_until=clock_timestamp()+(? * interval '1 millisecond'),updated_at=clock_timestamp()
        WHERE id=(SELECT id FROM background_jobs WHERE kind=? AND available_at<=clock_timestamp()
          AND (state='queued' OR (state='processing' AND lease_until<clock_timestamp()))
          ORDER BY available_at,id LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING *`, uuid.NewString(), lease.Milliseconds(), kind).Scan(&job)
	return job, result.RowsAffected != 0, result.Error
}

// Finish records completion of the leased job operation.
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
func (r *JobRepository) Finish(ctx context.Context, job jobs.Job, state, code string, retryAt *time.Time) error {
	query := `UPDATE background_jobs SET state=?,error_code=NULLIF(?,''),updated_at=clock_timestamp(),
        lease_token=NULL,lease_until=NULL,finished_at=clock_timestamp()
        WHERE id=?::uuid AND state='processing' AND lease_token=?::uuid AND lease_until>clock_timestamp()`
	args := []any{state, code, job.ID, job.LeaseToken}
	if retryAt != nil {
		query = `UPDATE background_jobs SET state='queued',error_code=NULLIF(?,''),available_at=?,updated_at=clock_timestamp(),
            lease_token=NULL,lease_until=NULL,finished_at=NULL
            WHERE id=?::uuid AND state='processing' AND lease_token=?::uuid AND lease_until>clock_timestamp()`
		args = []any{code, retryAt.UTC(), job.ID, job.LeaseToken}
	}
	result := r.db.WithContext(ctx).Exec(query, args...)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return jobs.ErrLeaseLost
	}
	return nil
}

// Counts returns the bounded queue-state counts used by operational monitoring.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: The collected count values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *JobRepository) Counts(ctx context.Context) ([]jobs.Count, error) {
	items := []jobs.Count{}
	err := r.db.WithContext(ctx).Raw(`SELECT kind,state,count(*) AS count FROM background_jobs
        WHERE state IN ('queued','processing','failed') GROUP BY kind,state`).Scan(&items).Error
	return items, err
}
