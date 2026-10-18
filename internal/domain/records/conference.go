package records

import "time"

// ConferenceStartRequest contains recording settings for an authorized conference.
type ConferenceStartRequest struct {
	SegmentDurationSec int    `json:"segmentDurationSec,omitempty"`
	Mode               string `json:"mode,omitempty"`
}

// ValidConferenceMode checks that the requested conference recording mode is supported.
//
// Parameters:
//   - mode: Validated operation or recording mode.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func ValidConferenceMode(mode string) bool {
	return mode == ModeComposite || mode == ModeAudioOnly || mode == ModeIndividualTracks || mode == ModeScreenFocus
}

// OutboxCommand persists reliable PostgreSQL-to-RabbitMQ recording delivery with versioned retry
// leases.
type OutboxCommand struct {
	ID           int64
	RecordID     string
	CommandType  string
	Reason       string
	ClaimToken   *string
	ClaimedUntil *time.Time
	PublishedAt  *time.Time
	CreatedAt    time.Time
}

// TableName returns the explicit database table name so GORM does not infer a different plural
// form.
//
// Returns:
//   - result 1: Exact database table name used by GORM.
func (OutboxCommand) TableName() string { return "recording_outbox" }

// IsComposite reports whether the recording belongs to the composite conference-recording
// pipeline.
//
// Parameters:
//   - record: Recording and its persisted lifecycle metadata.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func IsComposite(record Record) bool {
	return ValidConferenceMode(record.Mode) || record.SourceType == "conference" || record.PlatformConferenceID != nil
}

// PublicStatus converts the stored recording status to its client-facing lifecycle state.
//
// Parameters:
//   - status: Lifecycle state or response status to record.
//
// Returns:
//   - result 1: Client-facing recording lifecycle status.
func PublicStatus(status string) string {
	if status == StatusFinalizing || status == StatusUploading {
		return "processing"
	}
	return status
}
