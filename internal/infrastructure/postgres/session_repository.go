package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/realtime"
	"gorm.io/gorm"
)

// SessionRepository persists physical session history and room authorization state through GORM.
type SessionRepository struct{ db *gorm.DB }

// NewSessionRepository constructs the session repository with the dependencies and limits
// required for session.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//
// Returns:
//   - result 1: The constructed or loaded session repository.
func NewSessionRepository(db *gorm.DB) *SessionRepository { return &SessionRepository{db: db} }

// authorizeSession checks that the participant may open a live physical session for the
// conference.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - userID: Acting account identifier used to scope access.
//   - locked: Whether the query must lock the selected row before mutation.
//
// Returns:
//   - result 1: The resulting participant.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func authorizeSession(db *gorm.DB, conferenceID, userID string, locked bool) (conferences.Participant, error) {
	return authorizeRoomSession(db, conferenceID, userID, locked, "", false)
}

// authorizeRoomSession checks the persisted room membership and conference state for a physical
// session.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - userID: Acting account identifier used to scope access.
//   - locked: Whether the query must lock the selected row before mutation.
//   - clientID: Physical browser or socket client identifier.
//   - requireClient: Whether a physical client binding is mandatory for authorization.
//
// Returns:
//   - result 1: The resulting participant.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func authorizeRoomSession(db *gorm.DB, conferenceID, userID string, locked bool, clientID string, requireClient bool) (conferences.Participant, error) {
	c, err := findMediaRoom(db, conferenceID, locked)
	if err != nil {
		return conferences.Participant{}, err
	}
	if c.Kind == "direct_call" {
		// A membership alone cannot grant access to another device or revive an
		// ended call. Only the two clients that started/accepted may connect.
		q := db.Table("direct_calls").Where("conference_id=? AND status='accepted' AND caller_lease_until>clock_timestamp() AND callee_lease_until>clock_timestamp()", conferenceID)
		if requireClient {
			if clientID == "" {
				return conferences.Participant{}, apperrors.ErrForbidden
			}
			q = q.Where("(caller_user_id=? AND caller_client_id=?) OR (callee_user_id=? AND accepted_by_client_id=?)", userID, clientID, userID, clientID)
		} else {
			q = q.Where("caller_user_id=? OR callee_user_id=?", userID, userID)
		}
		var count int64
		if err = q.Count(&count).Error; err != nil {
			return conferences.Participant{}, err
		}
		if count != 1 {
			return conferences.Participant{}, apperrors.ErrForbidden
		}
	}
	p, err := findMembership(db, conferenceID, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return p, apperrors.ErrForbidden
	}
	if err != nil {
		return p, err
	}
	if c.Status != conferences.Created && c.Status != conferences.Active {
		return p, apperrors.New(apperrors.ErrConflict, "conference is closed")
	}
	if !p.CanParticipate() {
		return p, apperrors.New(apperrors.ErrForbidden, "join the conference before connecting")
	}
	var left int64
	if err := db.Table("conference_chat_preferences").Where("conference_id=? AND user_id=? AND left_at IS NOT NULL", conferenceID, userID).Count(&left).Error; err != nil {
		return p, err
	}
	if left > 0 {
		return p, apperrors.ErrForbidden
	}
	return p, nil
}

// Authorize checks the current identity and saved access state before session.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: The resulting participant.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *SessionRepository) Authorize(ctx context.Context, conferenceID, userID string) (conferences.Participant, error) {
	return authorizeSession(r.db.WithContext(ctx), conferenceID, userID, false)
}

// AuthorizeClient validates conference access for the specified account and physical client
// identity.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - userID: Acting account identifier used to scope access.
//   - clientID: Physical browser or socket client identifier.
//
// Returns:
//   - result 1: The resulting participant.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *SessionRepository) AuthorizeClient(ctx context.Context, conferenceID, userID, clientID string) (conferences.Participant, error) {
	return authorizeRoomSession(r.db.WithContext(ctx), conferenceID, userID, false, clientID, true)
}

// Open opens the scoped media, recording or session resource required by this component.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - session: Persistent or physical session state, as defined by its type.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *SessionRepository) Open(ctx context.Context, session realtime.Session) error {
	return r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			p, err := authorizeRoomSession(tx, session.ConferenceID, session.UserID, true, session.ClientID, true)
			if err != nil {
				return err
			}
			if p.ID != session.ParticipantID {
				return apperrors.ErrForbidden
			}
			return tx.Create(&session).Error
		})
}

// Close closes the component-owned resources and ends their lifecycle.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - connectionID: Physical media-connection identifier.
//   - seen: Reference timestamp of confirmed transport activity.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *SessionRepository) Close(ctx context.Context, connectionID string, seen time.Time) error {
	// Idempotent: PostgreSQL changes only on open and close, not on every pong.
	return r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			var session realtime.Session
			if err := tx.Where("connection_id = ?", connectionID).Take(&session).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
			// Use the same lock order as media updates, Open, participant removal and conference completion.
			if _, err := findMediaRoom(tx, session.ConferenceID, true); err != nil {
				return err
			}
			return tx.Model(&realtime.Session{}).Where("connection_id = ? AND status = 'connected'", connectionID).
				Updates(map[string]any{"status": "disconnected", "microphone_enabled": false, "camera_enabled": false, "screen_sharing": false, "last_seen_at": gorm.Expr("GREATEST(connected_at, ?)", seen), "disconnected_at": gorm.Expr("GREATEST(connected_at, ?, NOW())", seen)}).Error
		})
}

// Stale returns historical physical sessions that remain open beyond the liveness boundary.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - before: Boundary before which results or state are selected.
//   - cursor: Opaque continuation boundary from the previous page.
//
// Returns:
//   - result 1: The collected session values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *SessionRepository) Stale(ctx context.Context, before time.Time, cursor string) ([]realtime.Session, error) {
	var rows []realtime.Session
	query := r.db.WithContext(ctx).Where("status = 'connected' AND connected_at < ?", before)
	if cursor != "" {
		query = query.Where("id > ?", cursor)
	}
	err := query.Order("id").Limit(500).Find(&rows).Error
	return rows, err
}

// Roster loads the conference and memberships needed to authorize the current room snapshot.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//
// Returns:
//   - result 1: The resulting status.
//   - result 2: The collected participant values.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *SessionRepository) Roster(ctx context.Context, conferenceID string) (conferences.Status, []conferences.Participant, error) {
	c, err := findMediaRoom(r.db.WithContext(ctx), conferenceID, false)
	if err != nil {
		return "", nil, err
	}
	rows := []conferences.Participant{}
	err = r.db.WithContext(ctx).Table("conference_participants p").Select(`p.*, EXISTS (
		SELECT 1 FROM conference_chat_preferences cp
		WHERE cp.conference_id=p.conference_id AND cp.user_id=p.user_id AND cp.left_at IS NOT NULL
	) AS chat_left`).Where("p.conference_id = ?", conferenceID).Order("p.created_at, p.id").Scan(&rows).Error
	return c.Status, rows, err
}
