// Package content defines transcripts, validated AI results and conference search. It is
// independent of provider SDKs and does not participate in media transport.
package content

import (
	"context"
	"encoding/json"
	"io"
	"time"
)

const (
	Queued     = "queued"
	Processing = "processing"
	Ready      = "ready"
	Failed     = "failed"
)

// Transcript stores one generation of transcription for a completed recording. Public responses
// exclude MinIO keys, job leases and provider secrets.
type Transcript struct {
	ID           string     `json:"id"`
	ConferenceID string     `json:"conferenceId"`
	RecordingID  string     `json:"recordingId"`
	Status       string     `json:"status"`
	Language     string     `json:"language"`
	Provider     string     `json:"provider"`
	Generation   int64      `json:"generation"`
	ErrorCode    *string    `json:"errorCode,omitempty"`
	ErrorMessage *string    `json:"errorMessage,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	ProcessedAt  *time.Time `json:"processedAt,omitempty"`
}

// Segment binds text to a completed-recording time interval. speakerLabel is a diarization
// label, not a verified conference participant identity.
type Segment struct {
	ID           string   `json:"id"`
	TranscriptID string   `json:"transcriptId"`
	Ordinal      int      `json:"-"`
	StartMS      int64    `json:"startMs" gorm:"column:start_ms"`
	EndMS        int64    `json:"endMs" gorm:"column:end_ms"`
	SpeakerID    *string  `json:"speakerId"`
	SpeakerLabel *string  `json:"speakerLabel"`
	Text         string   `json:"text"`
	Confidence   *float64 `json:"confidence"`
}

// ActionItem contains only actions supported by source text. Assignee and deadline remain null
// when the source segments do not explicitly support them.
type ActionItem struct {
	Text             string   `json:"text"`
	Assignee         *string  `json:"assignee"`
	DueDate          *string  `json:"dueDate"`
	SourceSegmentIDs []string `json:"sourceSegmentIds"`
}

// SummaryOutput defines the only accepted JSON schema for an AI summary response.
type SummaryOutput struct {
	Summary     string       `json:"summary"`
	KeyPoints   []string     `json:"keyPoints"`
	ActionItems []ActionItem `json:"actionItems"`
	Topics      []string     `json:"topics"`
}

// Summary stores a validated result with the source transcript and instruction versions.
type Summary struct {
	ID                   string `json:"id"`
	ConferenceID         string `json:"conferenceId"`
	TranscriptID         string `json:"transcriptId"`
	TranscriptGeneration int64  `json:"transcriptGeneration"`
	Status               string `json:"status"`
	SummaryOutput        `gorm:"-"`
	StructuredOutput     json.RawMessage `json:"-" gorm:"column:structured_output"`
	Provider             string          `json:"provider"`
	Model                string          `json:"model"`
	PromptVersion        string          `json:"promptVersion"`
	SchemaVersion        string          `json:"schemaVersion"`
	Generation           int64           `json:"generation"`
	ErrorCode            *string         `json:"errorCode,omitempty"`
	ErrorMessage         *string         `json:"errorMessage,omitempty"`
	CreatedAt            time.Time       `json:"createdAt"`
	UpdatedAt            time.Time       `json:"updatedAt"`
	ProcessedAt          *time.Time      `json:"processedAt,omitempty"`
}

// TranscriptState distinguishes an absent transcript from a disabled capability and exposes the
// server-verified retry permission.
type TranscriptState struct {
	Item         *Transcript `json:"item"`
	Enabled      bool        `json:"enabled"`
	CanRetry     bool        `json:"canRetry"`
	ProviderMode string      `json:"providerMode"`
}

// SummaryState returns an optional saved result and permissions without inventing processing
// states.
type SummaryState struct {
	Item          *Summary `json:"item"`
	Enabled       bool     `json:"enabled"`
	CanRegenerate bool     `json:"canRegenerate"`
	ProviderMode  string   `json:"providerMode"`
}

// SegmentPage contains a bounded page of chronologically ordered transcript text.
type SegmentPage struct {
	Items  []Segment `json:"items"`
	Total  int64     `json:"total"`
	Limit  int       `json:"limit"`
	Offset int       `json:"offset"`
}

// SearchQuery defines server-side filters; times are unambiguous UTC timestamps.
type SearchQuery struct {
	Mode, ParticipantID, Membership string
	Query, Source, ConferenceID     string
	From, To                        *time.Time
	Limit, Offset                   int
}

// SearchResult contains plain-text excerpts and recording navigation positions, never
// object-storage URLs.
type SearchResult struct {
	SpeakerID       *string `json:"speakerId,omitempty"`
	Speaker         string  `json:"speaker,omitempty"`
	Type            string  `json:"type"`
	ConferenceID    string  `json:"conferenceId"`
	ConferenceTitle string  `json:"conferenceTitle"`
	RecordingID     *string `json:"recordingId"`
	TranscriptID    *string `json:"transcriptId"`
	SegmentID       *string `json:"segmentId"`
	StartMS         *int64  `json:"startMs" gorm:"column:start_ms"`
	Snippet         string  `json:"snippet"`
	Rank            float64 `json:"rank"`
}

// SearchPage contains a bounded page of authorized results only.
type SearchPage struct {
	EffectiveMode  string         `json:"effectiveMode"`
	FallbackReason string         `json:"fallbackReason,omitempty"`
	Items          []SearchResult `json:"items"`
	Total          int64          `json:"total"`
	Limit          int            `json:"limit"`
	Offset         int            `json:"offset"`
}

// RecordingSource identifies a private object loaded from trusted database state, not a
// user-supplied URL.
type RecordingSource struct {
	RecordingID, ConferenceID, ObjectKey string
	DurationSec                          int
	SizeBytes                            int64
}

// Audio contains a bounded mono 16 kHz WAV source and its mandatory cleanup operation.
type Audio struct {
	Reader      io.ReadCloser
	Size        int64
	ContentType string
	Cleanup     func()
}

// TranscriptionRequest supplies audio, automatic language selection and a stable idempotency key
// to the provider adapter.
type TranscriptionRequest struct {
	Audio                                 io.Reader
	Size                                  int64
	ContentType, Language, IdempotencyKey string
	DurationSec                           int
}

// TranscriptionResult contains the detected language and the provider's still-unvalidated
// segments.
type TranscriptionResult struct {
	Language string    `json:"language"`
	Segments []Segment `json:"segments"`
}

// TranscriptionProvider isolates external speech recognition from domain and application code.
type TranscriptionProvider interface {
	// Name returns the configured provider identifier used to label and validate generated content.
	//
	// Returns:
	//   - result 1: Configured provider identifier.
	Name() string
	// Transcribe submits bounded recording audio to the configured transcription provider.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting transcription result.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Transcribe(context.Context, TranscriptionRequest) (TranscriptionResult, error)
}

// AIRequest separates immutable instructions from untrusted conference-content JSON. Tool use
// and network actions are outside this contract.
type AIRequest struct {
	PromptVersion, SchemaVersion, Instructions, InputJSON, IdempotencyKey string
	Merge                                                                 bool
}

// AIProvider returns JSON that the application must validate before persistence.
type AIProvider interface {
	// Name returns the configured provider identifier used to label and validate generated content.
	//
	// Returns:
	//   - result 1: Configured provider identifier.
	Name() string
	// Model returns the configured model identifier used to label and validate generated content.
	//
	// Returns:
	//   - result 1: Configured provider-model identifier.
	Model() string
	// Summarize generates a schema-constrained summary from the supplied transcript input.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting raw message.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Summarize(context.Context, AIRequest) (json.RawMessage, error)
}
