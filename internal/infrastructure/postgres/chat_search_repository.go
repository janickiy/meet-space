package postgres

import (
	"context"
	"strings"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"gorm.io/gorm"
)

// SearchMessages preserves the conference action API while using the shared visibility rules.
func (r *ChatRepository) SearchMessages(ctx context.Context, userID, scopeID, query, cursor string, limit int) (chat.MessagePage, error) {
	page, err := r.SearchMessagePage(ctx, userID, scopeID, query, cursor, limit)
	return chat.MessagePage{Items: page.Items, NextCursor: page.NextCursor}, err
}

// SearchMessagePage searches literal text in the history currently visible to this reader.
// The conversation SHARE lock in authorize serializes group removal and history clearing.
func (r *ChatRepository) SearchMessagePage(ctx context.Context, userID, scopeID, query, cursor string, limit int) (chat.Page, error) {
	page := chat.Page{Items: []chat.Message{}}
	query, err := chat.NormalizeSearchQuery(query)
	if err != nil {
		return page, err
	}
	if limit < 1 || limit > 50 {
		return page, apperrors.ErrInvalidInput
	}
	before, err := parseChatCursor(scopeID, cursor)
	if err != nil {
		return page, err
	}
	escaped := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(query)
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := r.authorize(tx, userID, scopeID, false, false); err != nil {
			return err
		}
		cutoff, err := r.historyCutoff(tx, userID, scopeID)
		if err != nil {
			return err
		}
		q := r.messageQuery(tx).Where(r.sql("chat_messages.conference_id=? AND chat_messages.sequence>? AND chat_messages.deleted_at IS NULL AND chat_messages.text ILIKE ? ESCAPE '!'"), scopeID, cutoff, "%"+escaped+"%")
		if before > 0 {
			q = q.Where(r.sql("chat_messages.sequence<?"), before)
		}
		if err := q.Order(r.sql("chat_messages.sequence DESC")).Limit(limit + 1).Find(&page.Items).Error; err != nil {
			return err
		}
		if len(page.Items) > limit {
			page.Items = page.Items[:limit]
			page.NextCursor = chatCursor(scopeID, page.Items[len(page.Items)-1].Sequence)
		}
		if err := r.decorateMessages(tx, page.Items, cutoff); err != nil {
			return err
		}
		if err := r.markImportant(tx, userID, scopeID, page.Items); err != nil {
			return err
		}
		state, err := r.readState(tx, userID, scopeID)
		page.UnreadCount, page.LastReadMessageID = state.UnreadCount, state.LastReadMessageID
		return err
	})
	return page, err
}

// ChatMessageContext returns up to 25 messages through the target and 25 newer ones,
// chronologically, with an ordinary older-message cursor anchored to the visible window.
func (r *ChatRepository) ChatMessageContext(ctx context.Context, userID, scopeID, messageID string) (chat.Page, error) {
	page := chat.Page{Items: []chat.Message{}}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := r.authorize(tx, userID, scopeID, false, false); err != nil {
			return err
		}
		cutoff, err := r.historyCutoff(tx, userID, scopeID)
		if err != nil {
			return err
		}
		var target chat.Message
		if err := r.messages(tx).Where(r.sql("conference_id=? AND id=? AND deleted_at IS NULL AND sequence>?"), scopeID, messageID, cutoff).Take(&target).Error; err != nil {
			return mapNotFound(err)
		}
		older := []chat.Message{}
		if err := r.messageQuery(tx).Where(r.sql("chat_messages.conference_id=? AND chat_messages.sequence<=? AND chat_messages.sequence>?"), scopeID, target.Sequence, cutoff).
			Order(r.sql("chat_messages.sequence DESC")).Limit(26).Find(&older).Error; err != nil {
			return err
		}
		if len(older) > 25 {
			older = older[:25]
			page.NextCursor = chatCursor(scopeID, older[len(older)-1].Sequence)
		}
		newer := []chat.Message{}
		if err := r.messageQuery(tx).Where(r.sql("chat_messages.conference_id=? AND chat_messages.sequence>?"), scopeID, target.Sequence).
			Order(r.sql("chat_messages.sequence ASC")).Limit(25).Find(&newer).Error; err != nil {
			return err
		}
		for i := len(older) - 1; i >= 0; i-- {
			page.Items = append(page.Items, older[i])
		}
		page.Items = append(page.Items, newer...)
		if err := r.decorateMessages(tx, page.Items, cutoff); err != nil {
			return err
		}
		if err := r.markImportant(tx, userID, scopeID, page.Items); err != nil {
			return err
		}
		state, err := r.readState(tx, userID, scopeID)
		page.UnreadCount, page.LastReadMessageID = state.UnreadCount, state.LastReadMessageID
		return err
	})
	return page, err
}
