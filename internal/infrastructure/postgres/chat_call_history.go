package postgres

import (
	"github.com/janickiy/meet-space/internal/domain/chat"
	"gorm.io/gorm"
)

// decorateCallHistory hydrates the page in one bounded query after chat authorization
// and the reader's history cutoff have been applied. Device claims are never returned.
func decorateCallHistory(tx *gorm.DB, items []chat.Message, ids []string) error {
	hasCalls := false
	for _, item := range items {
		if item.CallID != nil {
			hasCalls = true
			break
		}
	}
	if !hasCalls {
		return nil
	}
	var rows []struct {
		MessageID        string
		chat.CallHistory `gorm:"embedded"`
	}
	if err := tx.Raw(`SELECT m.id AS message_id,c.id,c.caller_user_id,c.callee_user_id,
 c.status,c.created_at,c.accepted_at,c.ended_at,
 CASE WHEN c.accepted_at IS NOT NULL AND c.ended_at IS NOT NULL
 THEN GREATEST(0,FLOOR(EXTRACT(EPOCH FROM (c.ended_at-c.accepted_at))))::bigint ELSE 0 END AS duration_sec
 FROM conversation_messages m JOIN direct_calls c ON c.id=m.call_id AND c.conversation_id=m.conversation_id
 WHERE m.id IN ? AND m.deleted_at IS NULL`, ids).Scan(&rows).Error; err != nil {
		return err
	}
	byMessage := make(map[string]chat.CallHistory, len(rows))
	for _, row := range rows {
		byMessage[row.MessageID] = row.CallHistory
	}
	for i := range items {
		if call, ok := byMessage[items[i].ID]; ok {
			items[i].Call = &call
		}
	}
	return nil
}
