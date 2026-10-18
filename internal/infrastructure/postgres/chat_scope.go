package postgres

import (
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

// NewDirectChatRepository changes storage and authorization, sharing all message/file
// operations.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//
// Returns:
//   - result 1: The constructed or loaded chat repository.
func NewDirectChatRepository(db *gorm.DB) *ChatRepository {
	r := NewChatRepository(db)
	r.direct = true
	return r
}

// sql selects the SQL table or scope used by this repository adapter.
//
// Parameters:
//   - s: Validated server-controlled SQL relation or scalar input.
//
// Returns:
//   - result 1: Server-selected scoped SQL relation or statement.
func (r *ChatRepository) sql(s string) string {
	if !r.direct {
		return s
	}
	s = strings.ReplaceAll(s, "conference_id", "conversation_id")
	return strings.ReplaceAll(s, "chat_messages", "conversation_messages")
}

// messages returns the message relation for the repository's conversation scope.
//
// Parameters:
//   - tx: Current database transaction.
//
// Returns:
//   - result 1: The constructed or loaded db.
func (r *ChatRepository) messages(tx *gorm.DB) *gorm.DB {
	if r.direct {
		return tx.Model(&chat.Message{}).Table("conversation_messages").Omit("ConferenceID", "SenderName")
	}
	// CallID belongs only to conversation_messages; model-based SELECTs must not
	// project it from conference chat_messages, which has no such column.
	return tx.Model(&chat.Message{}).Omit("ConversationID", "SenderName", "CallID")
}

// attachments returns the attachment relation for the repository's conversation scope.
//
// Parameters:
//   - tx: Current database transaction.
//
// Returns:
//   - result 1: The constructed or loaded db.
func (r *ChatRepository) attachments(tx *gorm.DB) *gorm.DB {
	if r.direct {
		return tx.Model(&chat.Attachment{}).Table("conversation_attachments").Omit("ConferenceID", "SenderName")
	}
	return tx.Model(&chat.Attachment{}).Omit("ConversationID", "SenderName")
}

// readTable returns the read-state table associated with the chat scope.
//
// Returns:
//   - result 1: Read-state table for the selected conversation scope.
func (r *ChatRepository) readTable() string {
	if r.direct {
		return "conversation_members"
	}
	return "chat_read_states"
}

// messageScope reads the conference or conversation scope from a stored message.
//
// Parameters:
//   - m: Validated backup manifest or media state, as defined by its type.
//
// Returns:
//   - result 1: Conference or conversation identifier stored on the message.
func (r *ChatRepository) messageScope(m chat.Message) string {
	if r.direct {
		return m.ConversationID
	}
	return m.ConferenceID
}

// setMessageScope sets the message scope fields for the selected repository mode.
//
// Parameters:
//   - m: Validated backup manifest or media state, as defined by its type.
//   - id: Identifier of the resource addressed by this operation.
func (r *ChatRepository) setMessageScope(m *chat.Message, id string) {
	if r.direct {
		m.ConferenceID = ""
		m.ConversationID = id
	}
}

// setAttachmentScope sets the attachment scope fields for the selected repository mode.
//
// Parameters:
//   - a: The attachment value being compared.
//   - id: Identifier of the resource addressed by this operation.
func (r *ChatRepository) setAttachmentScope(a *chat.Attachment, id string) {
	if r.direct {
		a.ConferenceID = ""
		a.ConversationID = id
	}
}

// authorize checks the current identity and saved access state before chat scope.
//
// Parameters:
//   - tx: Current database transaction.
//   - user: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//   - locked: Whether the query must lock the selected row before mutation.
//   - write: Whether this access performs a mutation and requires write authorization.
//
// Returns:
//   - result 1: The resulting participant.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ChatRepository) authorize(tx *gorm.DB, user, id string, locked, write bool) (conferences.Participant, error) {
	if !r.direct {
		return authorizeChat(tx, user, id, locked, write)
	}
	if err := authorizeDirect(tx, user, id, locked); err != nil {
		return conferences.Participant{}, err
	}
	var visible int64
	if err := tx.Table("conversation_members").Where("conversation_id=? AND user_id=? AND hidden_at IS NULL", id, user).Count(&visible).Error; err != nil {
		return conferences.Participant{}, err
	}
	if visible != 1 {
		return conferences.Participant{}, apperrors.ErrForbidden
	}
	// Direct conversations have no moderator. The ID is used only to address the reader's account stream.
	return conferences.Participant{ID: user, Role: conferences.ParticipantRole}, nil
}

// historyCutoff loads the member-specific history boundary used after a direct chat is cleared.
//
// Parameters:
//   - tx: Current database transaction.
//   - user: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *ChatRepository) historyCutoff(tx *gorm.DB, user, id string) (int64, error) {
	if !r.direct {
		return 0, nil
	}
	var cutoff int64
	err := tx.Table("conversation_members").Select("history_cleared_through").Where("conversation_id=? AND user_id=?", id, user).Scan(&cutoff).Error
	return cutoff, err
}

// authorizeDirect checks that an account may access the direct conversation.
//
// Parameters:
//   - tx: Current database transaction.
//   - user: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//   - locked: Whether the query must lock the selected row before mutation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func authorizeDirect(tx *gorm.DB, user, id string, locked bool) error {
	var row struct{ ID string }
	q := tx.Table("conversations").Select("conversations.id").Where("conversations.id=? AND type IN('direct','group') AND deleted_at IS NULL", id)
	strength := "SHARE"
	if locked {
		strength = "UPDATE"
	}
	q = q.Clauses(clause.Locking{Strength: strength, Table: clause.Table{Name: "conversations"}})
	if err := q.Take(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrForbidden
		}
		return err
	}
	// Check membership in a new statement after acquiring the conversation lock.
	// All group mutations lock this same row before changing member activity.
	var count int64
	if err := tx.Table("conversation_members m").Joins("JOIN users u ON u.id=m.user_id").Where("m.conversation_id=? AND m.user_id=? AND m.left_at IS NULL AND u.guest_conference_id IS NULL AND u.password_hash<>''", id, user).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return apperrors.ErrForbidden
	}
	return nil
}

// attachmentScope resolves the authorized chat scope for an attachment operation.
//
// Parameters:
//   - a: The attachment value being compared.
//
// Returns:
//   - result 1: Conference or conversation identifier stored on the attachment.
func (r *ChatRepository) attachmentScope(a chat.Attachment) string {
	if r.direct {
		return a.ConversationID
	}
	return a.ConferenceID
}
