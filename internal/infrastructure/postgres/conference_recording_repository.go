package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/records"
	"github.com/janickiy/meet-space/internal/domain/users"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ConferenceRecordingRepository persists authorized conference recording lifecycle and
// command-outbox state through GORM.
type ConferenceRecordingRepository struct{ db *gorm.DB }

// NewConferenceRecordingRepository constructs the conference recording repository with the
// dependencies and limits required for conference recording.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//
// Returns:
//   - result 1: The constructed or loaded conference recording repository.
func NewConferenceRecordingRepository(db *gorm.DB) *ConferenceRecordingRepository {
	return &ConferenceRecordingRepository{db: db}
}

type RecordingOutbox = records.OutboxCommand

// Start starts conference recording and prepares the resources owned by that operation.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - segmentDuration: Requested recording-segment duration.
//
// Returns:
//   - result 1: The resulting record.
//   - result 2: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRecordingRepository) Start(ctx context.Context, userID, conferenceID string, segmentDuration int) (records.Record, bool, error) {
	return r.StartMode(ctx, userID, conferenceID, segmentDuration, records.ModeComposite)
}

// StartMode starts a recording in the selected validated conference recording mode.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - segmentDuration: Requested recording-segment duration.
//   - mode: Validated operation or recording mode.
//
// Returns:
//   - result 1: The resulting record.
//   - result 2: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRecordingRepository) StartMode(ctx context.Context, userID, conferenceID string, segmentDuration int, mode string) (records.Record, bool, error) {
	if !records.ValidConferenceMode(mode) {
		return records.Record{}, false, apperrors.ErrInvalidInput
	}
	var record records.Record
	created := false
	err := r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			conference, err := findConference(tx, conferenceID, true)
			if err != nil {
				return err
			}
			actor, err := findMembership(tx, conferenceID, userID)
			if err != nil {
				return membershipError(err)
			}
			if !actor.CanParticipate() {
				return apperrors.ErrForbidden
			}
			if err = requireRecordingAccount(tx, userID); err != nil {
				return err
			}
			if conference.Status != conferences.Active {
				return apperrors.New(apperrors.ErrConflict, "conference is not active")
			}
			err = tx.Where("platform_conference_id = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus') AND status IN ?", conferenceID, activeRecordingStatuses()).Take(&record).Error
			if err == nil {
				return apperrors.New(apperrors.ErrConflict, "recording is already active")
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			record = records.Record{UUID: uuid.NewString(), ConferenceID: conferenceID, PlatformConferenceID: &conferenceID, RequestedBy: &userID, Mode: mode, SourceType: "conference", TransportType: "sfu", Status: records.StatusStarting, QualityMode: "auto", SegmentDurationSec: segmentDuration, NeedPreview: mode != records.ModeAudioOnly && mode != records.ModeIndividualTracks}
			if err = tx.Create(&record).Error; err != nil {
				return err
			}
			if err = enqueueRecording(tx, record.UUID, "record.start", ""); err != nil {
				return err
			}
			created = true
			return nil
		})
	return record, created, err
}

// Stop stops the active conference recording operation and releases its owned resources.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - recordID: External recording UUID.
//
// Returns:
//   - result 1: The resulting record.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRecordingRepository) Stop(ctx context.Context, userID, conferenceID, recordID string) (records.Record, error) {
	var record records.Record
	err := r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
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
			if err = requireRecordingAccount(tx, userID); err != nil {
				return err
			}
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ? AND platform_conference_id = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus')", recordID, conferenceID).Take(&record).Error; err != nil {
				return mapNotFound(err)
			}
			owner := conference.OwnerID == userID && actor.Role == conferences.Owner
			if !owner && (record.RequestedBy == nil || *record.RequestedBy != userID) {
				return apperrors.ErrForbidden
			}
			reason := "participant_requested"
			if owner {
				reason = "owner_requested"
			}
			if records.IsTerminalStatus(record.Status) || record.Status == records.StatusFinalizing || record.Status == records.StatusUploading {
				return nil
			}
			if record.Status != records.StatusStopping {
				if err = tx.Model(&record).Updates(map[string]any{"status": records.StatusStopping, "stopped_at": time.Now().UTC(), "ended_reason": reason}).Error; err != nil {
					return err
				}
			}
			return enqueueRecording(tx, record.UUID, "record.stop", reason)
		})
	return record, err
}

// requireRecordingAccount rejects guest identities before account-only recording operations.
//
// Parameters:
//   - tx: Current database transaction.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func requireRecordingAccount(tx *gorm.DB, userID string) error {
	var user users.User
	err := tx.Select("id", "guest_conference_id").Where("id = ?", userID).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.ErrForbidden
	}
	if err != nil {
		return err
	}
	if user.GuestConferenceID != nil {
		return apperrors.ErrForbidden
	}
	return nil
}

// Accessible loads a recording only within the acting account's authorized conference scope.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - recordID: External recording UUID.
//
// Returns:
//   - result 1: The resulting record.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRecordingRepository) Accessible(ctx context.Context, userID, conferenceID, recordID string) (records.Record, error) {
	if err := r.authorizeRead(ctx, userID, conferenceID); err != nil {
		return records.Record{}, err
	}
	var record records.Record
	err := r.db.WithContext(ctx).Scopes(availableRecordings).Where("uuid = ? AND platform_conference_id = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus')", recordID, conferenceID).Take(&record).Error
	return record, mapNotFound(err)
}

// List returns a bounded page of conference recording records using the supplied pagination and
// filters.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - limit: Maximum number of results or operations in this batch.
//   - offset: Number of results skipped before the requested page.
//
// Returns:
//   - result 1: The collected record values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRecordingRepository) List(ctx context.Context, userID, conferenceID string, limit, offset int) ([]records.Record, error) {
	if err := r.authorizeRead(ctx, userID, conferenceID); err != nil {
		return nil, err
	}
	items := []records.Record{}
	err := r.db.WithContext(ctx).Scopes(availableRecordings).Where("platform_conference_id = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus')", conferenceID).Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&items).Error
	return items, err
}

// authorizeRead checks whether the acting account can read the requested recording.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - conferenceID: Conference identifier limiting the operation scope.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRecordingRepository) authorizeRead(ctx context.Context, userID, conferenceID string) error {
	if _, err := findConference(r.db.WithContext(ctx), conferenceID, false); err != nil {
		return err
	}
	participant, err := findMembership(r.db.WithContext(ctx), conferenceID, userID)
	if err != nil {
		return membershipError(err)
	}
	if !participant.CanReadHistory() {
		return apperrors.ErrForbidden
	}
	return nil
}

// activeRecordingStatuses returns the persisted states that still represent an active recording.
//
// Returns:
//   - result 1: The collected string values.
func activeRecordingStatuses() []string {
	return []string{records.StatusStarting, records.StatusRecording, records.StatusDegraded, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}
}

// enqueueRecording adds the recording lifecycle command to the transactional outbox.
//
// Parameters:
//   - tx: Current database transaction.
//   - recordID: External recording UUID.
//   - command: Validated internal recording or media command.
//   - reason: Bounded lifecycle or failure reason.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func enqueueRecording(tx *gorm.DB, recordID, command, reason string) error {
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&RecordingOutbox{RecordID: recordID, CommandType: command, Reason: reason}).Error
}

// stopConferenceRecordings marks the conference's active recordings for stopping and queues
// their commands.
//
// Parameters:
//   - tx: Current database transaction.
//   - conferenceID: Conference identifier limiting the operation scope.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func stopConferenceRecordings(tx *gorm.DB, conferenceID string) error {
	var rows []records.Record
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("platform_conference_id = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus') AND status IN ?", conferenceID, []string{records.StatusStarting, records.StatusRecording, records.StatusDegraded, records.StatusStopping}).Find(&rows).Error; err != nil {
		return err
	}
	for _, record := range rows {
		if record.Status != records.StatusStopping {
			if err := tx.Model(&record).Updates(map[string]any{"status": records.StatusStopping, "stopped_at": time.Now().UTC(), "ended_reason": "conference_finished"}).Error; err != nil {
				return err
			}
		}
		if err := enqueueRecording(tx, record.UUID, "record.stop", "conference_finished"); err != nil {
			return err
		}
	}
	return nil
}

// ClaimCommand claims one transactional recording-outbox command together with its current
// recording state.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: The resulting recording outbox.
//   - result 2: The resulting record.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRecordingRepository) ClaimCommand(ctx context.Context) (RecordingOutbox, records.Record, error) {
	var out RecordingOutbox
	var record records.Record
	err := r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			err := tx.Raw(`SELECT o.* FROM recording_outbox o WHERE o.published_at IS NULL
		AND (o.claimed_until IS NULL OR o.claimed_until < now())
		AND NOT EXISTS (SELECT 1 FROM recording_outbox prev WHERE prev.record_id=o.record_id AND prev.id<o.id AND prev.published_at IS NULL)
		ORDER BY o.id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&out).Error
			if err != nil {
				return err
			}
			if out.ID == 0 {
				return apperrors.ErrNotFound
			}
			token := uuid.NewString()
			until := time.Now().UTC().Add(15 * time.Second)
			out.ClaimToken = &token
			out.ClaimedUntil = &until
			if err = tx.Model(&out).Updates(map[string]any{"claim_token": token, "claimed_until": until}).Error; err != nil {
				return err
			}
			return tx.Where("uuid = ?", out.RecordID).Take(&record).Error
		})
	return out, record, err
}

// CompleteCommand acknowledges a recording-outbox command only while its claim is current.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - command: Validated internal recording or media command.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRecordingRepository) CompleteCommand(ctx context.Context, command RecordingOutbox) error {
	return r.db.WithContext(ctx).Model(&RecordingOutbox{}).Where("id = ? AND claim_token = ?", command.ID, command.ClaimToken).Updates(map[string]any{"published_at": time.Now().UTC(), "claimed_until": nil}).Error
}

// RetryCommand reschedules the claimed recording command using its bounded retry policy.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - command: Validated internal recording or media command.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ConferenceRecordingRepository) RetryCommand(ctx context.Context, command RecordingOutbox) error {
	return r.db.WithContext(ctx).Model(&RecordingOutbox{}).Where("id = ? AND claim_token = ?", command.ID, command.ClaimToken).Updates(map[string]any{"claimed_until": time.Now().UTC().Add(time.Second)}).Error
}
