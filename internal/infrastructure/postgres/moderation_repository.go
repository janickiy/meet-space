package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/media"
	"gorm.io/gorm"
)

// Moderate применяет действие по модерации участников после проверки роли субъекта и целевых
// ограничений.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - participantID: Идентификатор участника конференции.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Полученный участник.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ConferenceRepository) Moderate(ctx context.Context, conferenceID, userID, participantID string, request conferences.ModerationRequest) (conferences.Participant, error) {
	var target conferences.Participant
	if err := request.Validate(); err != nil {
		return target, err
	}
	err := r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			conference, err := findConference(tx, conferenceID, true)
			if err != nil {
				return err
			}
			if conference.Status != conferences.Active {
				return apperrors.New(apperrors.ErrConflict, "conference is not active")
			}
			actor, err := findMembership(tx, conferenceID, userID)
			if err != nil {
				return membershipError(err)
			}
			if err = tx.Where("id = ? AND conference_id = ?", participantID, conferenceID).Take(&target).Error; err != nil {
				return mapNotFound(err)
			}
			if !conferences.CanModerate(actor, target, request.Action) {
				return apperrors.ErrForbidden
			}
			updates := map[string]any{}
			switch request.Action {
			case "mute":
				if target.MicrophoneBlocked != *request.Blocked {
					updates["microphone_blocked"] = *request.Blocked
				}
				if *request.Blocked && target.MicrophoneEnabled {
					updates["microphone_enabled"] = false
				}
			case "camera":
				if target.CameraBlocked != *request.Blocked {
					updates["camera_blocked"] = *request.Blocked
				}
				if *request.Blocked && target.CameraEnabled {
					updates["camera_enabled"] = false
				}
				if *request.Blocked && target.ScreenSharing {
					updates["screen_sharing"] = false
				}
			case "screen":
				if target.ScreenBlocked != *request.Blocked {
					updates["screen_blocked"] = *request.Blocked
				}
				if *request.Blocked && target.ScreenSharing {
					updates["screen_sharing"] = false
				}
			case "kick":
				if target.Status != conferences.Kicked {
					updates["status"] = conferences.Kicked
					updates["admission_state"] = conferences.AdmissionKicked
					updates["admission_decided_at"] = time.Now().UTC()
					updates["admission_version"] = gorm.Expr("admission_version + 1")
					if target.JoinedAt != nil {
						updates["left_at"] = time.Now().UTC()
					}
					updates["microphone_enabled"], updates["camera_enabled"], updates["screen_sharing"] = false, false, false
				}
			case "role":
				if target.Role != request.Role {
					updates["role"] = request.Role
				}
			}
			// Снятие ограничения не приводит к восстановлению медиа-состояния другой вкладки,
			// которое было очищено модераторами.
			if request.Blocked != nil && *request.Blocked {
				cleared := map[string]any{}
				switch request.Action {
				case "mute":
					cleared["microphone_enabled"] = false
				case "camera":
					cleared["camera_enabled"] = false
					cleared["screen_sharing"] = false
				case "screen":
					cleared["screen_sharing"] = false
				}
				if len(cleared) > 0 {
					if err = tx.Table("participant_sessions").Where("participant_id = ? AND status = 'connected'", target.ID).Updates(cleared).Error; err != nil {
						return err
					}
				}
			}
			if len(updates) == 0 {
				return nil
			}
			updates["media_policy_version"] = gorm.Expr("media_policy_version + 1")
			if err = tx.Model(&target).Updates(updates).Error; err != nil {
				return err
			}
			if err = tx.Exec("INSERT INTO conference_moderation_audit(conference_id, actor_id, participant_id, action, blocked, role) VALUES(?,?,?,?,?,?)", conferenceID, actor.ID, target.ID, request.Action, request.Blocked, nullRole(request.Role)).Error; err != nil {
				return err
			}
			return tx.Where("id = ?", participantID).Take(&target).Error
		})
	return target, err
}

// nullRole преобразует необязательную сохранённую роль в представление роли домена.
//
// @Parameters:
//   - role: Запрошенная роль участника или члена группы.
//
// @returns:
//   - result1: Результирующий тип — any.
func nullRole(role conferences.Role) any {
	if role == "" {
		return nil
	}
	return role
}

// membershipError сопоставляет ошибку членства или допуска с ошибкой приложения.
//
// @Parameters:
//   - err: Ошибка классификации, инкапсуляции или безопасного оповещения.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func membershipError(err error) error {
	if errors.Is(err, apperrors.ErrNotFound) {
		return apperrors.ErrForbidden
	}
	return err
}

// UpdateMediaState сохраняет заявленное участником состояние источника мультимедиа.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - state: Текущее состояние или обновлённое валидированное состояние.
//
// @returns:
//   - result1: Полученный участник.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ConferenceRepository) UpdateMediaState(ctx context.Context, conferenceID, userID string, state conferences.MediaState) (conferences.Participant, error) {
	var participant conferences.Participant
	connectionID, parseErr := uuid.Parse(state.ConnectionID)
	if parseErr != nil || connectionID == uuid.Nil || state.Sequence < 1 || state.Sequence > 9007199254740991 {
		return participant, apperrors.New(apperrors.ErrInvalidInput, "connectionId and a positive safe-integer sequence are required")
	}
	err := r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			conference, err := findMediaRoom(tx, conferenceID, true)
			if err != nil {
				return err
			}
			if conference.Status != conferences.Active && conference.Status != conferences.Created {
				return apperrors.ErrConflict
			}
			participant, err = findMembership(tx, conferenceID, userID)
			if err != nil {
				return membershipError(err)
			}
			if !participant.CanParticipate() {
				return apperrors.ErrForbidden
			}
			var session struct{ MediaSequence int64 }
			query := tx.Table("participant_sessions").Where("connection_id = ? AND participant_id = ? AND conference_id = ? AND user_id = ? AND status = 'connected'", connectionID.String(), participant.ID, conferenceID, userID)
			if err = query.Select("media_sequence").Take(&session).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return apperrors.ErrForbidden
				}
				return err
			}
			if state.Sequence <= session.MediaSequence {
				return nil
			}
			if (state.MicrophoneEnabled && participant.MicrophoneBlocked) || (state.CameraEnabled && participant.CameraBlocked) || (state.ScreenSharing && (participant.ScreenBlocked || participant.CameraBlocked)) {
				return apperrors.New(apperrors.ErrForbidden, "media source is disabled by moderator")
			}
			if err = tx.Table("participant_sessions").Where("connection_id = ?", connectionID.String()).Updates(map[string]any{"media_sequence": state.Sequence, "microphone_enabled": state.MicrophoneEnabled, "camera_enabled": state.CameraEnabled, "screen_sharing": state.ScreenSharing}).Error; err != nil {
				return err
			}
			_, err = aggregateParticipantMedia(tx, &participant)
			return err
		})
	return participant, err
}

// MediaPolicy загружает действующие сохранённые медиа-ограничения для участника.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - participantID: Идентификатор участника конференции.
//
// @returns:
//   - result1: Результирующая политика в отношении участника.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ConferenceRepository) MediaPolicy(ctx context.Context, conferenceID, participantID string) (media.ParticipantPolicy, error) {
	var participant conferences.Participant
	err := r.db.WithContext(ctx).Where("id = ? AND conference_id = ?", participantID, conferenceID).Take(&participant).Error
	if err != nil {
		return media.ParticipantPolicy{}, mapNotFound(err)
	}
	conference, err := findMediaRoom(r.db.WithContext(ctx), conferenceID, false)
	if err != nil {
		return media.ParticipantPolicy{}, err
	}
	policy := media.PolicyForParticipant(participant)
	if conference.Status != conferences.Active && conference.Status != conferences.Created {
		policy.Kicked = true
	}
	return policy, nil
}

// ReconcileParticipants возвращает участников, чьи сохранённые медиа-ограничения требуют
// согласования.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - after: Граница, после которой выбираются результаты или состояние.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//
// @returns:
//   - result1: Собранные данные об участниках.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ConferenceRepository) ReconcileParticipants(ctx context.Context, after string, limit int) ([]conferences.Participant, error) {
	items := []conferences.Participant{}
	query := r.db.WithContext(ctx).Model(&conferences.Participant{}).Joins("JOIN conferences c ON c.id = conference_participants.conference_id").Where("c.status IN ('created','active') AND conference_participants.id > ?", after).Order("conference_participants.id").Limit(limit)
	err := query.Select("conference_participants.*").Find(&items).Error
	return items, err
}

// ClearDisconnectedMedia очищает объявленное медиа-состояние, если у участника больше нет
// активного авторизованного сеанса.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - participantID: Идентификатор участника конференции.
//
// @returns:
//   - result1: Полученный участник.
//   - result2: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ConferenceRepository) ClearDisconnectedMedia(ctx context.Context, conferenceID, participantID string) (conferences.Participant, bool, error) {
	var p conferences.Participant
	changed := false
	err := r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			if _, err := findMediaRoom(tx, conferenceID, true); err != nil {
				return err
			}
			if err := tx.Where("id = ? AND conference_id = ?", participantID, conferenceID).Take(&p).Error; err != nil {
				return mapNotFound(err)
			}
			var err error
			changed, err = aggregateParticipantMedia(tx, &p)
			return err
		})
	return p, changed, err
}

// aggregateParticipantMedia объединяет активные физические сеансы в действующее медиа-состояние
// участника.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - p: Состояние участника, обработанное в ходе этой операции.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func aggregateParticipantMedia(tx *gorm.DB, p *conferences.Participant) (bool, error) {
	var aggregate struct {
		MicrophoneEnabled bool
		CameraEnabled     bool
		ScreenSharing     bool
	}
	if err := tx.Table("participant_sessions").Select("COALESCE(bool_or(microphone_enabled), false) AS microphone_enabled, COALESCE(bool_or(camera_enabled), false) AS camera_enabled, COALESCE(bool_or(screen_sharing), false) AS screen_sharing").Where("participant_id = ? AND status = 'connected'", p.ID).Scan(&aggregate).Error; err != nil {
		return false, err
	}
	aggregate.MicrophoneEnabled = aggregate.MicrophoneEnabled && !p.MicrophoneBlocked && p.CanParticipate()
	aggregate.CameraEnabled = aggregate.CameraEnabled && !p.CameraBlocked && p.CanParticipate()
	aggregate.ScreenSharing = aggregate.ScreenSharing && !p.ScreenBlocked && !p.CameraBlocked && p.CanParticipate()
	if p.MicrophoneEnabled == aggregate.MicrophoneEnabled && p.CameraEnabled == aggregate.CameraEnabled && p.ScreenSharing == aggregate.ScreenSharing {
		return false, nil
	}
	if err := tx.Model(p).Updates(map[string]any{"microphone_enabled": aggregate.MicrophoneEnabled, "camera_enabled": aggregate.CameraEnabled, "screen_sharing": aggregate.ScreenSharing}).Error; err != nil {
		return false, err
	}
	return true, nil
}
