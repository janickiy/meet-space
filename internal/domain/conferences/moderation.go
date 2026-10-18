package conferences

import "github.com/janickiy/meet-space/internal/domain/apperrors"

// MediaState describes the participant's declared microphone, camera and screen activity.
type MediaState struct {
	ConnectionID      string `json:"connectionId"`
	Sequence          int64  `json:"sequence"`
	MicrophoneEnabled bool   `json:"microphoneEnabled"`
	CameraEnabled     bool   `json:"cameraEnabled"`
	ScreenSharing     bool   `json:"screenSharing"`
}

// ModerationRequest contains a moderation action and its additional validated parameters.
type ModerationRequest struct {
	Action  string `json:"action"`
	Blocked *bool  `json:"blocked,omitempty"`
	Role    Role   `json:"role,omitempty"`
}

// CanModerate checks whether the actor may apply a moderation action to the target participant.
//
// Parameters:
//   - actor: Identifier of the account performing the operation.
//   - target: Target state, account or decoded value, as defined by its type.
//   - action: Requested lifecycle or moderation action.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func CanModerate(actor, target Participant, action string) bool {
	switch action {
	case "mute", "camera", "screen", "kick", "role":
	default:
		return false
	}
	if !actor.CanParticipate() || actor.ConferenceID != target.ConferenceID || target.Role == Owner || actor.ID == target.ID {
		return false
	}
	// Waiting participants are visible only in the waiting room and are handled through admit or
	// reject. General moderation events are visible to admitted room participants. Repeating an
	// already completed removal remains idempotent.
	if !target.CanReadHistory() && !(action == "kick" && target.Status == Kicked) {
		return false
	}
	if actor.Role == Owner {
		return true
	}
	return actor.Role == CoHost && target.Role == ParticipantRole && (action == "mute" || action == "screen" || action == "kick")
}

// Validate rejects inconsistent or out-of-range conference fields before the value is used.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r ModerationRequest) Validate() error {
	switch r.Action {
	case "mute", "camera", "screen":
		if r.Blocked == nil || r.Role != "" {
			return apperrors.New(apperrors.ErrInvalidInput, "blocked is required for media moderation")
		}
	case "kick":
		if r.Blocked != nil || r.Role != "" {
			return apperrors.ErrInvalidInput
		}
	case "role":
		if r.Blocked != nil || (r.Role != CoHost && r.Role != ParticipantRole) {
			return apperrors.New(apperrors.ErrInvalidInput, "role must be co_host or participant")
		}
	default:
		return apperrors.New(apperrors.ErrInvalidInput, "unsupported moderation action")
	}
	return nil
}
