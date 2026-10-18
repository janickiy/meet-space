package postgres

import (
	"context"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/records"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Delete checks the same ownership policy as Stop and serializes with recording
// workers, retention, and conference moderation before touching storage.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - recordID: External recording UUID.
//   - remove: Callback removing the authorized recording's owned storage objects.
//
// Returns:
//   - result 1: The resulting record.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRecordingRepository) Delete(ctx context.Context, userID, conferenceID, recordID string, remove func(context.Context, records.Record) error) (records.Record, error) {
	var record records.Record
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		conference, err := findConference(tx, conferenceID, true)
		if err != nil {
			return err
		}
		actor, err := findMembership(tx, conferenceID, userID)
		if err != nil {
			return membershipError(err)
		}
		if !actor.CanReadHistory() {
			return apperrors.ErrForbidden
		}
		if err := requireRecordingAccount(tx, userID); err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("uuid = ? AND platform_conference_id = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus')", recordID, conferenceID).
			Take(&record).Error; err != nil {
			return mapNotFound(err)
		}
		owner := conference.OwnerID == userID && actor.Role == conferences.Owner
		if !owner && (record.RequestedBy == nil || *record.RequestedBy != userID) {
			return apperrors.ErrForbidden
		}
		var state struct {
			Deleted bool
			Leased  bool
		}
		if err := tx.Model(&records.Record{}).Select("deleted_at IS NOT NULL AS deleted, COALESCE(recorder_lease_until > clock_timestamp(), false) AS leased").
			Where("id = ?", record.ID).Scan(&state).Error; err != nil {
			return err
		}
		if state.Deleted {
			return nil // Repeat requests after a lost response are safe.
		}
		if !records.IsTerminalStatus(record.Status) || state.Leased {
			return apperrors.New(apperrors.ErrConflict, "recording is still being processed")
		}
		return removeRecordingArtifacts(ctx, tx, record, remove)
	})
	return record, err
}
