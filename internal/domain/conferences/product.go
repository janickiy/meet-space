package conferences

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// IsRejectedOrKicked identifies a revoked membership before rejoin or invitation.
// Waiting, absent and unknown states are not revocation; admission checks remain separate.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (p Participant) IsRejectedOrKicked() bool {
	return p.Status == Kicked || p.Status == Rejected || p.AdmissionState == AdmissionKicked || p.AdmissionState == AdmissionRejected
}

// IsAdmitted reports whether the saved membership permits admission to the conference.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (p Participant) IsAdmitted() bool {
	return (p.AdmissionState == AdmissionAdmitted || p.AdmissionState == "") && p.Status != Waiting && p.Status != Rejected && p.Status != Kicked
}

// CanParticipate checks whether a membership and conference state permit active participation.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (p Participant) CanParticipate() bool { return p.IsAdmitted() && p.Status == Joined }

// CanReadHistory checks whether the membership permits access to the conference history.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (p Participant) CanReadHistory() bool {
	return p.IsAdmitted() && (p.Status == Joined || p.Status == Left)
}

// CanAdmit checks whether the membership has the authority to decide waiting-room admission.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (p Participant) CanAdmit() bool {
	return p.CanParticipate() && (p.Role == Owner || p.Role == CoHost)
}

// AdmissionRequest contains the organizer's acceptance or rejection decision for a waiting
// participant.
type AdmissionRequest struct {
	Decision string `json:"decision"`
}

// Validate rejects inconsistent or out-of-range conference fields before the value is used.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r AdmissionRequest) Validate() error {
	if r.Decision != "admit" && r.Decision != "reject" {
		return apperrors.New(apperrors.ErrInvalidInput, "decision must be admit or reject")
	}
	return nil
}

// ScheduleRequest contains an unambiguous start time and an optional planned duration.
type ScheduleRequest struct {
	ScheduledAt        time.Time `json:"scheduledAt"`
	PlannedDurationMin *int      `json:"plannedDurationMin"`
}

// ValidateSchedule checks the requested start and end times before a conference is scheduled.
//
// Parameters:
//   - at: Timestamp of the event or state boundary.
//   - duration: Duration or validity window used by the operation.
//   - now: Reference time used for expiration or duration calculations.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func ValidateSchedule(at *time.Time, duration *int, now time.Time) error {
	if duration != nil && (*duration < 1 || *duration > 1440) {
		return apperrors.New(apperrors.ErrInvalidInput, "plannedDurationMin must be between 1 and 1440")
	}
	if at == nil {
		if duration != nil {
			return apperrors.New(apperrors.ErrInvalidInput, "planned duration requires scheduledAt")
		}
		return nil
	}
	if at.IsZero() || !at.After(now) || at.Year() > 9999 {
		return apperrors.New(apperrors.ErrInvalidInput, "scheduledAt must be a future RFC3339 instant with an explicit timezone")
	}
	return nil
}

// TimelineQuery contains the account's server-side conference filters and pagination cursor.
type TimelineQuery struct {
	View   string
	Scope  string
	Status Status
	From   *time.Time
	To     *time.Time
	Cursor string
	Limit  int
}

// TimelineCursor contains the date and UUID of a conference-list page boundary.
type TimelineCursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

// Validate rejects inconsistent or out-of-range conference fields before the value is used.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (q TimelineQuery) Validate() error {
	if q.View != "upcoming" && q.View != "active" && q.View != "past" {
		return apperrors.New(apperrors.ErrInvalidInput, "view must be upcoming, active or past")
	}
	if q.Scope != "all" && q.Scope != "owned" && q.Scope != "participating" {
		return apperrors.ErrInvalidInput
	}
	if q.Limit < 1 || q.Limit > 100 || (q.From != nil && q.To != nil && q.From.After(*q.To)) {
		return apperrors.ErrInvalidInput
	}
	if q.Status != "" && q.Status != Created && q.Status != Scheduled && q.Status != Active && q.Status != Finished && q.Status != Cancelled {
		return apperrors.ErrInvalidInput
	}
	_, err := DecodeTimelineCursor(q.Cursor)
	return err
}

// DecodeTimelineCursor decodes and validates the continuation boundary for the conference
// timeline.
//
// Parameters:
//   - value: Value to validate, normalize or convert.
//
// Returns:
//   - result 1: The constructed or loaded timeline cursor.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func DecodeTimelineCursor(value string) (*TimelineCursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > 256 {
		return nil, apperrors.ErrInvalidInput
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(value)
	var c TimelineCursor
	if err != nil || json.Unmarshal(data, &c) != nil || c.At.IsZero() {
		return nil, apperrors.ErrInvalidInput
	}
	id, err := uuid.Parse(c.ID)
	if err != nil || id == uuid.Nil {
		return nil, apperrors.ErrInvalidInput
	}
	c.ID = id.String()
	return &c, nil
}

// EncodeTimelineCursor encodes a stable continuation boundary for the conference timeline.
//
// Parameters:
//   - c: The conference value processed by this operation.
//
// Returns:
//   - result 1: Opaque conference-timeline continuation cursor.
func EncodeTimelineCursor(c Conference) string {
	at := c.CreatedAt
	if c.ScheduledAt != nil {
		at = *c.ScheduledAt
	}
	if c.FinishedAt != nil {
		at = *c.FinishedAt
	}
	data, _ := json.Marshal(TimelineCursor{At: at, ID: c.ID})
	return base64.RawURLEncoding.EncodeToString(data)
}

// TimelinePage contains filtered conferences and the continuation cursor.
type TimelinePage struct {
	Items      []View  `json:"items"`
	NextCursor *string `json:"nextCursor"`
}

// RecordingSummary aggregates recording states without duplicating file metadata in the
// conference.
type RecordingSummary struct {
	Total      int64 `json:"total"`
	Ready      int64 `json:"ready"`
	Processing int64 `json:"processing"`
	Failed     int64 `json:"failed"`
}

// HistoryOwner holds the history owner data used by conference lifecycle and participant
// membership.
type HistoryOwner struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// HistoryView combines completed-conference information, visible memberships and recording
// summaries.
type HistoryView struct {
	Conference            View              `json:"conference"`
	Owner                 HistoryOwner      `json:"owner"`
	DurationSec           *int64            `json:"durationSec"`
	ParticipantCount      int64             `json:"participantCount"`
	Participants          []ParticipantView `json:"participants"`
	ParticipantsTruncated bool              `json:"participantsTruncated"`
	Recordings            RecordingSummary  `json:"recordings"`
	ChatAvailable         bool              `json:"chatAvailable"`
	ChatReadOnly          bool              `json:"chatReadOnly"`
}

// CanParticipate applies the same membership policy to a realtime/public view.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (p ParticipantView) CanParticipate() bool {
	return Participant{Status: p.Status, AdmissionState: p.AdmissionState}.CanParticipate()
}

// CanReadHistory includes admitted former participants, unlike live participation.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (p ParticipantView) CanReadHistory() bool {
	return Participant{Status: p.Status, AdmissionState: p.AdmissionState}.CanReadHistory()
}
