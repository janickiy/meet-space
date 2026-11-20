// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package postgres

import (
	"context"
	"strings"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/personal"
	"gorm.io/gorm"
)

// authorizeConversationInfo keeps profile/material reads behind the same
// membership, hidden-chat and account boundaries as the conversation itself.
func authorizeConversationInfo(tx *gorm.DB, user, id string) error {
	if _, err := NewDirectChatRepository(tx).authorize(tx, user, id, false, false); err != nil {
		return err
	}
	var count int64
	if err := tx.Table("users").Where("id=? AND deleted_at IS NULL AND guest_conference_id IS NULL AND NOT is_blocked AND NOT ldap_disabled", user).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return apperrors.ErrForbidden
	}
	return nil
}

// ConversationProfile exposes contact details only inside an active shared
// conversation. It does not extend the global user search projection.
func (r *PersonalRepository) ConversationProfile(ctx context.Context, user, id, target string) (personal.Profile, error) {
	var item personal.Profile
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := authorizeConversationInfo(tx, user, id); err != nil {
			return err
		}
		return mapNotFound(tx.Table("conversation_members m").
			Select("u.id,COALESCE(NULLIF(u.display_name,''),'Пользователь') AS display_name,u.email,u.bio,u.created_at").
			Joins("JOIN users u ON u.id=m.user_id").
			Where("m.conversation_id=? AND m.user_id=? AND m.left_at IS NULL AND u.deleted_at IS NULL AND u.guest_conference_id IS NULL AND NOT u.is_blocked AND NOT u.ldap_disabled", id, target).
			Take(&item).Error)
	})
	return item, err
}

// ConversationMaterials pages individual attachments/links, including multiple
// items in one message, and never crosses the caller's cleared-history boundary.
func (r *PersonalRepository) ConversationMaterials(ctx context.Context, user, id, kind, cursor string, limit int) (chat.MaterialPage, error) {
	page := chat.MaterialPage{Items: []chat.Material{}}
	if kind == "image" {
		kind = "media"
	}
	if limit < 1 || limit > 100 || (kind != "media" && kind != "file" && kind != "link") {
		return page, apperrors.ErrInvalidInput
	}
	// Binding the cursor to both the conversation and the tab prevents skipped
	// items when an old pagination response races with a tab change.
	scope := id + "." + kind
	before, offset, err := parseMaterialCursor(scope, cursor)
	if err != nil {
		return page, err
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := authorizeConversationInfo(tx, user, id); err != nil {
			return err
		}
		repo := NewDirectChatRepository(tx)
		cutoff, err := repo.historyCutoff(tx, user, id)
		if err != nil {
			return err
		}
		itemSQL := `SELECT row_number() OVER (ORDER BY a.created_at,a.id) AS item_index,
			a.id::text AS attachment_id, ''::text AS url
			FROM conversation_attachments a
			WHERE a.conversation_id=m.conversation_id AND a.message_id=m.id AND a.status='attached' AND `
		args := []any{}
		if kind == "media" {
			itemSQL += `(a.mime_type LIKE 'image/%' OR a.mime_type LIKE 'video/%')`
		} else if kind == "file" {
			itemSQL += `(a.mime_type NOT LIKE 'image/%' AND a.mime_type NOT LIKE 'video/%')`
		} else {
			itemSQL = `SELECT urls.item_index, ''::text AS attachment_id, urls.match[1] AS url
				FROM regexp_matches(m.text, ?, 'g') WITH ORDINALITY AS urls(match,item_index)`
			args = append(args, chatURL.String())
		}
		query := `SELECT m.id AS message_id,m.sequence,items.item_index,items.attachment_id,items.url
			FROM conversation_messages m CROSS JOIN LATERAL (` + itemSQL + `) items
			WHERE m.conversation_id=? AND m.sequence>? AND m.deleted_at IS NULL AND m.kind='text'`
		args = append(args, id, cutoff)
		if before > 0 {
			query += ` AND (m.sequence<? OR (m.sequence=? AND items.item_index>?))`
			args = append(args, before, before, offset)
		}
		query += ` ORDER BY m.sequence DESC,items.item_index ASC LIMIT ?`
		args = append(args, limit+1)
		var rows []struct {
			MessageID, AttachmentID, URL string
			Sequence                     int64
			ItemIndex                    int
		}
		if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		if len(rows) > limit {
			rows = rows[:limit]
			last := rows[len(rows)-1]
			page.NextCursor = materialCursor(scope, last.Sequence, last.ItemIndex)
		}
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.MessageID)
		}
		var messages []chat.Message
		if err := repo.messageQuery(tx).Where("conversation_messages.conversation_id=? AND conversation_messages.id IN ? AND conversation_messages.sequence>? AND conversation_messages.deleted_at IS NULL", id, ids, cutoff).Find(&messages).Error; err != nil {
			return err
		}
		if err := repo.decorateMessages(tx, messages, cutoff); err != nil {
			return err
		}
		byID := make(map[string]chat.Message, len(messages))
		for _, message := range messages {
			byID[message.ID] = message
		}
		for _, row := range rows {
			message, exists := byID[row.MessageID]
			if !exists {
				continue
			}
			if kind == "link" {
				page.Items = append(page.Items, chat.Material{Message: message, URL: strings.TrimRight(row.URL, ".,;:!?)")})
				continue
			}
			for _, attachment := range message.Attachments {
				if attachment.ID == row.AttachmentID {
					file := attachment
					page.Items = append(page.Items, chat.Material{Message: message, Attachment: &file})
					break
				}
			}
		}
		return nil
	})
	return page, err
}
