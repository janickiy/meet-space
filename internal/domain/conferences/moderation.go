package conferences

import "github.com/janickiy/meet-space/internal/domain/apperrors"

// MediaState описывает заявленную участником активность микрофона, камеры и экрана.
type MediaState struct {
	ConnectionID      string `json:"connectionId"`
	Sequence          int64  `json:"sequence"`
	MicrophoneEnabled bool   `json:"microphoneEnabled"`
	CameraEnabled     bool   `json:"cameraEnabled"`
	ScreenSharing     bool   `json:"screenSharing"`
}

// ModerationRequest содержит действие по модерации и его дополнительные валидированные
// параметры.
type ModerationRequest struct {
	Action  string `json:"action"`
	Blocked *bool  `json:"blocked,omitempty"`
	Role    Role   `json:"role,omitempty"`
}

// CanModerate проверяет, может ли субъект применить действие по модерации к целевому участнику.
//
// @Parameters:
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - target: Целевое состояние, учетная запись или декодированное значение в соответствии с
//     типом.
//   - action: Запрошенное действие по управлению жизненным циклом или модерации.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func CanModerate(actor, target Participant, action string) bool {
	switch action {
	case "mute", "camera", "screen", "kick", "role":
	default:
		return false
	}
	if !actor.CanParticipate() || actor.ConferenceID != target.ConferenceID || target.Role == Owner || actor.ID == target.ID {
		return false
	}
	// Ожидание участников отображается только в комнате ожидания и обрабатывается с помощью
	// функции «принять» или «отклонить».  Общие события модерации отображаются для участников,
	// допущенных в комнату.  Повторное удаление участника, которое уже было выполнено, остается
	// идемпотентным.
	if !target.CanReadHistory() && !(action == "kick" && target.Status == Kicked) {
		return false
	}
	if actor.Role == Owner {
		return true
	}
	return actor.Role == CoHost && target.Role == ParticipantRole && (action == "mute" || action == "screen" || action == "kick")
}

// Validate отклоняет несогласованные или выходящие за пределы допустимых значений поля
// конференции до того, как значение будет использовано.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
