package postgres

import (
	"context"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/personal"
	"gorm.io/gorm"
)

// lockDirectConversation locks the caller's direct-conversation membership before changing
// personal preferences or history.
//
// Parameters:
//   - tx: Current database transaction.
//   - user: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func lockDirectConversation(tx *gorm.DB, user, id string) error {
	if err := authorizeDirect(tx, user, id, true); err != nil {
		return err
	}
	var kind string
	if err := tx.Table("conversations").Select("type").Where("id=?", id).Scan(&kind).Error; err != nil {
		return err
	}
	if kind != "direct" {
		return apperrors.ErrForbidden
	}
	return nil
}

// SetConversationPreferences updates only the acting member's conversation notification
// preference.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//   - enabled: Whether the requested feature or preference should be enabled.
//
// Returns:
//   - result 1: The resulting conversation.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *PersonalRepository) SetConversationPreferences(ctx context.Context, user, id string, enabled bool) (personal.Conversation, error) {
	var item personal.Conversation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := authorizeDirect(tx, user, id, true); err != nil {
			return err
		}
		if err := tx.Table("conversation_members").Where("conversation_id=? AND user_id=?", id, user).
			Updates(map[string]any{"notifications_enabled": enabled, "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
			return err
		}
		var err error
		item, err = personalView(tx, user, id)
		return err
	})
	return item, err
}

// clearDirectHistory advances the member-specific history boundary without deleting the other
// participant's messages.
//
// Parameters:
//   - tx: Current database transaction.
//   - user: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//   - hide: Whether the member-specific conversation view should also be hidden.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func clearDirectHistory(tx *gorm.DB, user, id string, hide bool) error {
	var latest struct {
		ID       string
		Sequence int64
	}
	result := tx.Table("conversation_messages").Select("id,sequence").Where("conversation_id=?", id).
		Order("sequence DESC").Limit(1).Scan(&latest)
	if result.Error != nil {
		return result.Error
	}
	updates := map[string]any{"history_cleared_through": gorm.Expr("GREATEST(history_cleared_through,?)", latest.Sequence), "updated_at": gorm.Expr("clock_timestamp()")}
	if latest.ID != "" {
		updates["last_read_message_id"] = latest.ID
		updates["last_read_sequence"] = latest.Sequence
	}
	if hide {
		updates["hidden_at"] = gorm.Expr("COALESCE(hidden_at,clock_timestamp())")
	}
	return tx.Table("conversation_members").Where("conversation_id=? AND user_id=?", id, user).Updates(updates).Error
}

// ClearDirectHistory moves only the acting member's direct-chat history boundary without
// deleting the peer's history.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting conversation.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *PersonalRepository) ClearDirectHistory(ctx context.Context, user, id string) (personal.Conversation, error) {
	var item personal.Conversation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockDirectConversation(tx, user, id); err != nil {
			return err
		}
		if err := clearDirectHistory(tx, user, id, false); err != nil {
			return err
		}
		var err error
		item, err = personalView(tx, user, id)
		return err
	})
	return item, err
}

// HideDirectConversation hides the direct conversation for the acting account and retains the
// peer's saved history.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *PersonalRepository) HideDirectConversation(ctx context.Context, user, id string) (int64, error) {
	var cutoff int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockDirectConversation(tx, user, id); err != nil {
			return err
		}
		if err := clearDirectHistory(tx, user, id, true); err != nil {
			return err
		}
		return tx.Table("conversation_members").Select("history_cleared_through").Where("conversation_id=? AND user_id=?", id, user).Scan(&cutoff).Error
	})
	return cutoff, err
}
