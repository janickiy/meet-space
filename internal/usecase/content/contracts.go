// Package content coordinates isolated transcription and AI background jobs and authorized
// access to their results.
package content

import (
	"context"
	domain "github.com/janickiy/meet-space/internal/domain/content"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"time"
)

// Config bounds external-processing cost, result size and execution time. TranscriptionEnabled
// and AIEnabled independently disable sending content to providers.
type Config struct {
	// Independent enable flags permit sending audio and text to external adapters.
	TranscriptionEnabled, AIEnabled bool
	// Duration and segment limits bound both the accepted recording and the provider response.
	MaxDurationSec, MaxSegments int
	// The text budget counts UTF-8 bytes, not character count.
	MaxTranscriptBytes int64
	// Chunk and worker budgets bound model input and concurrent AI calls.
	ChunkRunes, MaxChunks, AIConcurrency int
	// Deadline bounds each job; cooldown prevents expensive manual replay.
	STTTimeout, AITimeout, ReprocessCooldown time.Duration
	// Reprocess counts new generations; Attempts counts transient retries within one generation.
	MaxReprocess, MaxAttempts int
}

// DefaultConfig returns the bounded default settings for content processing.
//
// Returns:
//   - result 1: The resulting config.
func DefaultConfig() Config {
	return Config{MaxDurationSec: 7200, MaxSegments: 10000, MaxTranscriptBytes: 8 << 20, ChunkRunes: 12000, MaxChunks: 256, AIConcurrency: 2, STTTimeout: 10 * time.Minute, AITimeout: 2 * time.Minute, ReprocessCooldown: 5 * time.Minute, MaxReprocess: 3, MaxAttempts: 5}
}

// AudioSource extracts bounded audio from a private object after the database source is
// verified.
type AudioSource interface {
	// Open opens the scoped media, recording or session resource required by this component.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Media or content source belonging to the current operation.
	//
	// Returns:
	//   - result 1: The resulting audio.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Open(context.Context, domain.RecordingSource) (domain.Audio, error)
}

// Repository combines history-based authorization with atomic generation transitions and an
// outbox. Every worker commit checks lease and generation; every read checks current access
// policy.
type Repository interface {
	// Transcript returns the current transcript metadata and provider capabilities without starting
	// transcription.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Recording identifier used by the current operation.
	//
	// Returns:
	//   - result 1: The constructed or loaded transcript.
	//   - result 2: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Transcript(context.Context, string, string, string) (*domain.Transcript, bool, error)
	// Segments returns a bounded page of saved transcript segments for timestamp navigation.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Recording identifier used by the current operation.
	//   - argument 5: Maximum number of results or operations in this batch.
	//   - argument 6: Number of results skipped before the requested page.
	//
	// Returns:
	//   - result 1: The resulting segment page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Segments(context.Context, string, string, string, int, int) (domain.SegmentPage, error)
	// Summary returns the current persisted summary without invoking an external provider.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Recording identifier used by the current operation.
	//
	// Returns:
	//   - result 1: The constructed or loaded summary.
	//   - result 2: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Summary(context.Context, string, string, string) (*domain.Summary, bool, error)
	// QueueTranscript queues an authorized transcription generation with deduplication and retry
	// limits.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Recording identifier used by the current operation.
	//   - argument 5: Maximum permitted size or number of characters.
	//   - argument 6: Minimum interval between expensive retriable processing requests.
	//   - argument 7: Maximum permitted job attempts or the current retry count.
	//
	// Returns:
	//   - result 1: The constructed or loaded transcript.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	QueueTranscript(context.Context, string, string, string, int, time.Duration, int) (*domain.Transcript, error)
	// QueueSummary queues an authorized summary generation with deduplication and retry limits.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Recording identifier used by the current operation.
	//   - argument 5: Maximum permitted size or number of characters.
	//   - argument 6: Minimum interval between expensive retriable processing requests.
	//   - argument 7: Maximum permitted job attempts or the current retry count.
	//
	// Returns:
	//   - result 1: The constructed or loaded summary.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	QueueSummary(context.Context, string, string, string, int, time.Duration, int) (*domain.Summary, error)
	// StartTranscript claims the current transcript generation before expensive provider processing
	// starts.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Leased background job and its generation or ownership token.
	//
	// Returns:
	//   - result 1: The resulting transcript.
	//   - result 2: The resulting recording source.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	StartTranscript(context.Context, jobs.Job) (domain.Transcript, domain.RecordingSource, error)
	// SaveTranscript commits transcript output only for the current recording generation and job
	// lease.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Leased background job and its generation or ownership token.
	//   - argument 3: Test handle used for assertions, cleanup and failure reporting.
	//   - argument 4: Validated processing or operation result.
	//   - argument 5: Configured external-provider adapter.
	//   - argument 6: Whether summary generation should be scheduled for the completed transcript.
	//   - argument 7: Maximum permitted job attempts or the current retry count.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SaveTranscript(context.Context, jobs.Job, domain.Transcript, domain.TranscriptionResult, string, bool, int) error
	// StartSummary claims the current summary generation before invoking the configured provider.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Leased background job and its generation or ownership token.
	//
	// Returns:
	//   - result 1: The resulting summary.
	//   - result 2: The collected segment values.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	StartSummary(context.Context, jobs.Job) (domain.Summary, []domain.Segment, error)
	// SaveSummary commits a validated summary only for its current transcript generation and job
	// lease.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Leased background job and its generation or ownership token.
	//   - argument 3: Validated server-controlled SQL relation or scalar input.
	//   - argument 4: Destination or generated result required by this operation.
	//   - argument 5: Configured external-provider adapter.
	//   - argument 6: Configured provider model identifier.
	//   - argument 7: Versioned model-instruction identity used by the summary.
	//   - argument 8: Expected output-schema version.
	//   - argument 9: Maximum permitted job attempts or the current retry count.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SaveSummary(context.Context, jobs.Job, domain.Summary, domain.SummaryOutput, string, string, string, string, int) error
	// FailJob records a bounded job failure without exposing provider responses or replacing a newer
	// generation.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Leased background job and its generation or ownership token.
	//   - argument 3: Bounded public status or failure code.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	FailJob(context.Context, jobs.Job, string) error
	// Search searches recording transcripts and summaries using bounded input and the caller's
	// visibility filters.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Search text or validated query filters.
	//
	// Returns:
	//   - result 1: The resulting search page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Search(context.Context, string, domain.SearchQuery) (domain.SearchPage, error)
}
