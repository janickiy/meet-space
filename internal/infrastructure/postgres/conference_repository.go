package postgres

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/users"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ConferenceRepository persists conference lifecycle and participant memberships through GORM.
type ConferenceRepository struct{ db *gorm.DB }

// NewConferenceRepository constructs the conference repository with the dependencies and limits
// required for conference.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//
// Returns:
//   - result 1: The constructed or loaded conference repository.
func NewConferenceRepository(db *gorm.DB) *ConferenceRepository { return &ConferenceRepository{db: db} }

// Create creates and persists a new conference for the acting account.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conference: Conference record and its validated lifecycle metadata.
//   - owner: Account, participant or worker that owns the resource.
//
// Returns:
//   - result 1: The resulting conference.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRepository) Create(ctx context.Context, conference conferences.Conference, owner conferences.Participant) (conferences.Conference, error) {
	err := r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			var account users.User
			if err := tx.Clauses(clause.Locking{Strength: "KEY SHARE"}).Where("id=? AND password_hash<>''", conference.OwnerID).Take(&account).Error; err != nil {
				return mapNotFound(err)
			}
			if err := tx.Create(&conference).Error; err != nil {
				return err
			}
			return tx.Create(&owner).Error
		})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "conferences_invite_code_key" {
		return conferences.Conference{}, conferences.ErrInviteCollision
	}
	return conference, err
}

// Get loads the requested conference within the caller's authorized scope.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting conference.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRepository) Get(ctx context.Context, id string) (conferences.Conference, error) {
	return findConference(r.db.WithContext(ctx), id, false)
}

// GetByInvite loads the conference associated with a valid invitation code.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - code: Bounded public status or failure code.
//
// Returns:
//   - result 1: The resulting conference.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRepository) GetByInvite(ctx context.Context, code string) (conferences.Conference, error) {
	var conference conferences.Conference
	err := r.db.WithContext(ctx).Where("invite_code = ? AND kind = 'meeting'", code).Take(&conference).Error
	return conference, mapNotFound(err)
}

// ListForUser returns only conferences visible through the specified account's membership.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - limit: Maximum number of results or operations in this batch.
//   - offset: Number of results skipped before the requested page.
//
// Returns:
//   - result 1: The collected conference values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRepository) ListForUser(ctx context.Context, userID string, limit, offset int) ([]conferences.Conference, error) {
	items := []conferences.Conference{}
	err := r.db.WithContext(ctx).Model(&conferences.Conference{}).
		Where("conferences.kind = 'meeting'").
		Joins("JOIN conference_participants p ON p.conference_id = conferences.id").
		Where("p.user_id = ? AND p.admission_state IN ('admitted','waiting')", userID).
		Where("NOT EXISTS (SELECT 1 FROM conference_chat_preferences cp WHERE cp.conference_id=conferences.id AND cp.user_id=? AND cp.left_at IS NOT NULL)", userID).
		Order("conferences.created_at DESC, conferences.id").
		Limit(limit).Offset(offset).Find(&items).Error
	return items, err
}

// Membership loads the account's saved membership in the requested conference.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: The resulting participant.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRepository) Membership(ctx context.Context, id, userID string) (conferences.Participant, error) {
	return findMembership(r.db.WithContext(ctx), id, userID)
}

// Participants returns the conference participants visible to the acting account.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//   - limit: Maximum number of results or operations in this batch.
//   - offset: Number of results skipped before the requested page.
//
// Returns:
//   - result 1: The collected participant values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRepository) Participants(ctx context.Context, id string, limit, offset int) ([]conferences.Participant, error) {
	items := []conferences.Participant{}
	err := r.db.WithContext(ctx).Where("conference_id = ?", id).
		Order("created_at ASC, id").Limit(limit).Offset(offset).Find(&items).Error
	return items, err
}

// Transition applies an allowed conference lifecycle transition without overwriting a newer
// state.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//   - userID: Acting account identifier used to scope access.
//   - target: Target state, account or decoded value, as defined by its type.
//
// Returns:
//   - result 1: The resulting conference.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRepository) Transition(ctx context.Context, id, userID string, target conferences.Status) (conferences.Conference, error) {
	var conference conferences.Conference
	err := r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			var err error
			conference, err = findConference(tx, id, true)
			if err != nil {
				return err
			}
			if conference.OwnerID != userID {
				return apperrors.ErrForbidden
			}
			if !conferences.CanTransition(conference.Status, target) {
				return apperrors.New(apperrors.ErrConflict, "conference status does not allow this transition")
			}
			now := time.Now().UTC()
			updates := map[string]any{"status": target}
			if target == conferences.Active {
				updates["started_at"] = now
			} else {
				updates["finished_at"] = now
			}
			if err := tx.Model(&conference).Updates(updates).Error; err != nil {
				return err
			}
			if target == conferences.Finished || target == conferences.Cancelled {
				if err := stopConferenceRecordings(tx, id); err != nil {
					return err
				}
				if err := tx.Model(&conferences.Participant{}).
					Where("conference_id = ? AND status = ?", id, conferences.Joined).
					Updates(map[string]any{"status": conferences.Left, "left_at": now, "microphone_enabled": false, "camera_enabled": false, "screen_sharing": false, "media_policy_version": gorm.Expr("media_policy_version + 1")}).Error; err != nil {
					return err
				}
			}
			conference, err = findConference(tx, id, false)
			return err
		})
	return conference, err
}

// Join creates or resumes the authorized membership or media-room connection, preserving its
// current identity.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//   - user: Validated account record whose public identity or server session is being processed.
//   - inviteCode: Validated conference invitation code.
//
// Returns:
//   - result 1: The resulting participant.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRepository) Join(ctx context.Context, id string, user users.User, inviteCode string) (conferences.Participant, error) {
	var participant conferences.Participant
	err := r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			conference, err := findConference(tx, id, true)
			if err != nil {
				return err
			}
			if conference.Status != conferences.Created && conference.Status != conferences.Active && conference.Status != conferences.Scheduled {
				return apperrors.New(apperrors.ErrConflict, "conference is closed")
			}
			participant, err = findMembership(tx, id, user.ID)
			missing := errors.Is(err, apperrors.ErrNotFound)
			if err != nil && !missing {
				return err
			}
			hasInvite := inviteCode != "" && subtle.ConstantTimeCompare([]byte(inviteCode), []byte(conference.InviteCode)) == 1
			if missing && !hasInvite {
				return apperrors.New(apperrors.ErrForbidden, "inviteCode is required for a new membership")
			}
			if !missing && participant.IsRejectedOrKicked() {
				return apperrors.New(apperrors.ErrForbidden, "this membership cannot rejoin the conference")
			}
			if !missing {
				if err := tx.Exec("UPDATE conference_chat_preferences SET left_at=NULL,updated_at=clock_timestamp() WHERE conference_id=? AND user_id=? AND left_at IS NOT NULL", id, user.ID).Error; err != nil {
					return err
				}
			}
			if !missing && participant.CanParticipate() {
				return nil
			}
			now := time.Now().UTC()
			if missing {
				userID := user.ID
				participant = conferences.Participant{ID: uuid.NewString(), ConferenceID: id, UserID: &userID,
					DisplayName: user.ParticipantName(), Role: conferences.ParticipantRole, Status: conferences.Joined, JoinedAt: &now, AdmissionState: conferences.AdmissionAdmitted}
				if conference.WaitingRoomEnabled && !hasInvite {
					participant.Status, participant.AdmissionState, participant.JoinedAt = conferences.Waiting, conferences.AdmissionWaiting, nil
				} else if conference.Status == conferences.Scheduled {
					participant.Status, participant.JoinedAt = conferences.Left, nil
				}
				return tx.Create(&participant).Error
			}
			// The invitation grants immediate admission, including a participant previously waiting.
			// Explicit rejection and removal were checked above.
			if hasInvite && participant.AdmissionState == conferences.AdmissionWaiting {
				status := conferences.Joined
				var joinedAt *time.Time = &now
				if conference.Status == conferences.Scheduled {
					status, joinedAt = conferences.Left, nil
				}
				if err := tx.Model(&participant).Updates(map[string]any{"status": status, "admission_state": conferences.AdmissionAdmitted, "admission_decided_at": now, "admission_version": gorm.Expr("admission_version + 1"), "joined_at": joinedAt, "left_at": nil, "media_policy_version": gorm.Expr("media_policy_version + 1")}).Error; err != nil {
					return err
				}
				participant, err = findMembership(tx, id, user.ID)
				return err
			}
			// Without an invitation, retain the previous admission state.
			if participant.AdmissionState == conferences.AdmissionWaiting {
				if participant.Status == conferences.Waiting {
					return nil
				}
				return tx.Model(&participant).Update("status", conferences.Waiting).Error
			}
			if conference.Status == conferences.Scheduled {
				return nil
			}
			if err := tx.Model(&participant).Updates(map[string]any{"status": conferences.Joined, "joined_at": now, "left_at": nil, "media_policy_version": gorm.Expr("media_policy_version + 1")}).Error; err != nil {
				return err
			}
			participant, err = findMembership(tx, id, user.ID)
			return err
		})
	return participant, err
}

// Leave removes the active membership or media connection while preserving the saved
// conversation or conference history.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: The resulting participant.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRepository) Leave(ctx context.Context, id, userID string) (conferences.Participant, error) {
	var participant conferences.Participant
	err := r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			if _, err := findConference(tx, id, true); err != nil {
				return err
			}
			var err error
			participant, err = findMembership(tx, id, userID)
			if errors.Is(err, apperrors.ErrNotFound) {
				return apperrors.ErrForbidden
			}
			if err != nil {
				return err
			}
			if participant.Status == conferences.Waiting {
				return tx.Model(&participant).Update("status", conferences.Left).Error
			}
			if participant.Status != conferences.Joined {
				return nil
			}
			if err := tx.Model(&participant).Updates(map[string]any{"status": conferences.Left, "left_at": time.Now().UTC(), "microphone_enabled": false, "camera_enabled": false, "screen_sharing": false, "media_policy_version": gorm.Expr("media_policy_version + 1")}).Error; err != nil {
				return err
			}
			participant, err = findMembership(tx, id, userID)
			return err
		})
	return participant, err
}

// findConference loads the conference used to authorize the current transaction.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//   - id: Identifier of the resource addressed by this operation.
//   - locked: Whether the query must lock the selected row before mutation.
//
// Returns:
//   - result 1: The resulting conference.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func findConference(db *gorm.DB, id string, locked bool) (conferences.Conference, error) {
	return findMediaRoom(db.Where("kind = 'meeting'"), id, locked)
}

// findMediaRoom includes private call rooms. Only the media lifecycle may use
// it; ordinary meeting APIs must go through findConference instead.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//   - id: Identifier of the resource addressed by this operation.
//   - locked: Whether the query must lock the selected row before mutation.
//
// Returns:
//   - result 1: The resulting conference.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func findMediaRoom(db *gorm.DB, id string, locked bool) (conferences.Conference, error) {
	if locked {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var conference conferences.Conference
	err := db.Where("id = ?", id).Take(&conference).Error
	return conference, mapNotFound(err)
}

// findMembership loads the participant membership used by the current transaction.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//   - id: Identifier of the resource addressed by this operation.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: The resulting participant.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func findMembership(db *gorm.DB, id, userID string) (conferences.Participant, error) {
	var participant conferences.Participant
	err := db.Where("conference_id = ? AND user_id = ?", id, userID).Take(&participant).Error
	return participant, mapNotFound(err)
}
