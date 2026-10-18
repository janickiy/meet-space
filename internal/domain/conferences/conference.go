package conferences

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

type Status string
type Role string
type ParticipantStatus string
type AdmissionState string

const (
	Created           Status            = "created"
	Scheduled         Status            = "scheduled"
	Active            Status            = "active"
	Finished          Status            = "finished"
	Cancelled         Status            = "cancelled"
	Owner             Role              = "owner"
	CoHost            Role              = "co_host"
	ParticipantRole   Role              = "participant"
	Guest             Role              = "guest"
	Joined            ParticipantStatus = "joined"
	Left              ParticipantStatus = "left"
	Waiting           ParticipantStatus = "waiting"
	Rejected          ParticipantStatus = "rejected"
	Kicked            ParticipantStatus = "kicked"
	AdmissionWaiting  AdmissionState    = "waiting"
	AdmissionAdmitted AdmissionState    = "admitted"
	AdmissionRejected AdmissionState    = "rejected"
	AdmissionKicked   AdmissionState    = "kicked"
)

var ErrInviteCollision = errors.New("invite code collision")

// CanTransition checks whether the requested lifecycle transition is allowed from the current
// state.
//
// Parameters:
//   - from: Source state of the proposed lifecycle transition.
//   - to: Destination state or recipient of the operation.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func CanTransition(from, to Status) bool {
	return ((from == Created || from == Scheduled) && (to == Active || to == Cancelled)) || (from == Active && to == Finished)
}

// Conference persists its owner, lifecycle state and schedule.
type Conference struct {
	ID                 string `gorm:"type:uuid;primaryKey"`
	OwnerID            string `gorm:"column:owner_id;type:uuid"`
	Kind               string `gorm:"default:meeting"`
	Title              string
	InviteCode         string `gorm:"column:invite_code"`
	Status             Status
	CreatedAt          time.Time
	UpdatedAt          time.Time
	StartedAt          *time.Time
	FinishedAt         *time.Time
	WaitingRoomEnabled bool
	ScheduledAt        *time.Time
	PlannedDurationMin *int
}

// TableName returns the explicit database table name so GORM does not infer a different plural
// form.
//
// Returns:
//   - result 1: Exact database table name used by GORM.
func (Conference) TableName() string { return "conferences" }

// Participant persists account membership, admission and media restrictions independently of
// physical connections.
type Participant struct {
	ID                 string  `gorm:"type:uuid;primaryKey"`
	ConferenceID       string  `gorm:"column:conference_id;type:uuid"`
	UserID             *string `gorm:"column:user_id;type:uuid"`
	DisplayName        string
	Role               Role
	Status             ParticipantStatus
	JoinedAt           *time.Time
	LeftAt             *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	MicrophoneEnabled  bool
	CameraEnabled      bool
	ScreenSharing      bool
	MicrophoneBlocked  bool
	CameraBlocked      bool
	ScreenBlocked      bool
	MediaPolicyVersion int64          `gorm:"default:1"`
	AdmissionState     AdmissionState `gorm:"default:admitted"`
	AdmissionDecidedAt *time.Time
	AdmissionVersion   int64 `gorm:"default:1"`
	ChatLeft           bool  `json:"-" gorm:"column:chat_left;->;-:migration"`
}

// TableName returns the explicit database table name so GORM does not infer a different plural
// form.
//
// Returns:
//   - result 1: Exact database table name used by GORM.
func (Participant) TableName() string { return "conference_participants" }

// View is the safe public API projection of a conference.
type View struct {
	ID                 string     `json:"id"`
	OwnerID            string     `json:"ownerId"`
	Title              string     `json:"title"`
	InviteCode         string     `json:"inviteCode"`
	InviteURL          string     `json:"inviteUrl"`
	Status             Status     `json:"status"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	StartedAt          *time.Time `json:"startedAt"`
	FinishedAt         *time.Time `json:"finishedAt"`
	WaitingRoomEnabled bool       `json:"waitingRoomEnabled"`
	ScheduledAt        *time.Time `json:"scheduledAt"`
	PlannedDurationMin *int       `json:"plannedDurationMin"`
}

// View builds the public API representation without exposing private persistence fields.
//
// Returns:
//   - result 1: The resulting view.
func (c Conference) View() View {
	return View{ID: c.ID, OwnerID: c.OwnerID, Title: c.Title, InviteCode: c.InviteCode,
		InviteURL: "/api/v1/conference-invites/" + c.InviteCode, Status: c.Status,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, StartedAt: c.StartedAt, FinishedAt: c.FinishedAt,
		WaitingRoomEnabled: c.WaitingRoomEnabled, ScheduledAt: c.ScheduledAt, PlannedDurationMin: c.PlannedDurationMin}
}

// InviteView exposes limited invitation information without granting access to protected room
// data.
type InviteView struct {
	ID                 string     `json:"id"`
	Title              string     `json:"title"`
	Status             Status     `json:"status"`
	WaitingRoomEnabled bool       `json:"waitingRoomEnabled,omitempty"`
	ScheduledAt        *time.Time `json:"scheduledAt,omitempty"`
}

// ParticipantView contains the membership and participant state that an external client is
// allowed to see.
type ParticipantView struct {
	ID                 string            `json:"id"`
	ConferenceID       string            `json:"conferenceId"`
	UserID             *string           `json:"userId"`
	DisplayName        string            `json:"displayName"`
	Role               Role              `json:"role"`
	Status             ParticipantStatus `json:"status"`
	JoinedAt           *time.Time        `json:"joinedAt"`
	LeftAt             *time.Time        `json:"leftAt"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
	MicrophoneEnabled  bool              `json:"microphoneEnabled"`
	CameraEnabled      bool              `json:"cameraEnabled"`
	ScreenSharing      bool              `json:"screenSharing"`
	MicrophoneBlocked  bool              `json:"microphoneBlocked"`
	CameraBlocked      bool              `json:"cameraBlocked"`
	ScreenBlocked      bool              `json:"screenBlocked"`
	MediaPolicyVersion int64             `json:"mediaPolicyVersion"`
	AdmissionState     AdmissionState    `json:"admissionState"`
	AdmissionDecidedAt *time.Time        `json:"admissionDecidedAt"`
	AdmissionVersion   int64             `json:"admissionVersion"`
	ChatLeft           bool              `json:"-"`
}

// View builds the public API representation without exposing private persistence fields.
//
// Returns:
//   - result 1: The resulting participant view.
func (p Participant) View() ParticipantView {
	return ParticipantView(p)
}

// CreateRequest contains conference creation settings, including schedule and waiting-room
// configuration.
type CreateRequest struct {
	Title              string     `json:"title"`
	WaitingRoomEnabled bool       `json:"waitingRoomEnabled"`
	ScheduledAt        *time.Time `json:"scheduledAt"`
	PlannedDurationMin *int       `json:"plannedDurationMin"`
}

// JoinRequest contains the participant information supplied when joining a conference.
type JoinRequest struct {
	InviteCode string `json:"inviteCode"`
}

// NormalizeTitle trims and validates a conference title before it is stored.
//
// Parameters:
//   - value: Value to validate, normalize or convert.
//
// Returns:
//   - result 1: Validated normalized conference title.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NormalizeTitle(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !utf8.ValidString(value) || value == "" || utf8.RuneCountInString(value) > 200 {
		return "", apperrors.New(apperrors.ErrInvalidInput, "title must contain 1 to 200 characters")
	}
	return value, nil
}
