package postgres

import (
	"context"
	"database/sql"

	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/records"
	"gorm.io/gorm"
)

// Library uses the recording read ACL, without the meeting timeline's status or
// chat-left filters. An owner still needs readable membership, exactly as Read.
func (r *ConferenceRecordingRepository) Library(ctx context.Context, userID string, query records.LibraryQuery) ([]records.LibraryRecord, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	cursor, err := records.DecodeLibraryCursor(query.Cursor)
	if err != nil {
		return nil, err
	}
	result := []records.LibraryRecord{}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Apply the existing retention scope before joining other tables, whose
		// status/created_at columns would otherwise make that scope ambiguous.
		available := tx.Model(&records.Record{}).Scopes(availableRecordings).
			Where("mode IN ('composite','audio_only','individual_tracks','screen_focus')")
		q := tx.Table("(?) AS r", available).Select("r.*").
			Joins("JOIN conferences c ON c.id = r.platform_conference_id AND c.kind = 'meeting'").
			Where("r.conference_id = c.id").
			Where(`EXISTS (SELECT 1 FROM conference_participants p
				WHERE p.conference_id=c.id AND p.user_id=?
				AND p.admission_state IN ('admitted','') AND p.status IN ('joined','left'))`, userID)
		if cursor != nil {
			q = q.Where("(r.created_at, r.id) < (?, ?)", cursor.At, cursor.ID)
		}
		var rows []records.Record
		if err := q.Order("r.created_at DESC, r.id DESC").Limit(query.Limit + 1).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ConferenceID)
		}
		var meetings []conferences.Conference
		if err := tx.Where("id IN ?", ids).Find(&meetings).Error; err != nil {
			return err
		}
		byID := make(map[string]conferences.Conference, len(meetings))
		for _, meeting := range meetings {
			byID[meeting.ID] = meeting
		}
		for _, row := range rows {
			result = append(result, records.LibraryRecord{Recording: row, Conference: byID[row.ConferenceID]})
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return result, err
}
