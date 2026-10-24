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

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/personal"
	"gorm.io/gorm"
)

// decorateConversationPins добавляет общий маркер к уже авторизованным проекциям сообщений,
// включая историю, поиск, контекст и обновления сообщений.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - items: Ограниченная коллекция записей для обработки.
//   - ids: Идентификаторы ресурсов для обработки в рамках ограниченного пакета.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func decorateConversationPins(tx *gorm.DB, items []chat.Message, ids []string) error {
	var pinned []string
	if err := tx.Table("conversation_message_pins").
		Where("conversation_id=? AND message_id IN ?", items[0].ConversationID, ids).
		Pluck("message_id", &pinned).Error; err != nil {
		return err
	}
	set := make(map[string]bool, len(pinned))
	for _, id := range pinned {
		set[id] = true
	}
	for i := range items {
		items[i].Pinned = set[items[i].ID] && items[i].DeletedAt == nil
	}
	return nil
}

// ConversationPins применяет те же правила в отношении членства в группах, скрытых чатов и
// очищенной истории, что и при обычном чтении сообщений.  Закреплённые сообщения не
// восстанавливаются после очистки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - cursor: Непрозрачная граница продолжения с предыдущей страницы.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//
// @returns:
//   - result1: Результирующая страница с сообщениями.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *PersonalRepository) ConversationPins(ctx context.Context, user, id, cursor string, limit int) (chat.MessagePage, error) {
	page := chat.MessagePage{Items: []chat.Message{}}
	if limit < 1 || limit > 100 {
		return page, apperrors.ErrInvalidInput
	}
	before, err := parseChatCursor(id, cursor)
	if err != nil {
		return page, err
	}
	repo := NewDirectChatRepository(r.db)
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := repo.authorize(tx, user, id, false, false); err != nil {
			return err
		}
		cutoff, err := repo.historyCutoff(tx, user, id)
		if err != nil {
			return err
		}
		query := repo.messageQuery(tx).
			Joins("JOIN conversation_message_pins pins ON pins.conversation_id=conversation_messages.conversation_id AND pins.message_id=conversation_messages.id").
			Where("conversation_messages.conversation_id=? AND conversation_messages.sequence>? AND conversation_messages.deleted_at IS NULL", id, cutoff)
		if before > 0 {
			query = query.Where("conversation_messages.sequence<?", before)
		}
		if err := query.Order("conversation_messages.sequence DESC").Limit(limit + 1).Find(&page.Items).Error; err != nil {
			return err
		}
		if len(page.Items) > limit {
			page.Items = page.Items[:limit]
			page.NextCursor = chatCursor(id, page.Items[len(page.Items)-1].Sequence)
		}
		return repo.decorateMessages(tx, page.Items, cutoff)
	})
	return page, err
}

// SetConversationPin сериализует изменения закреплённых сообщений с записями сообщений,
// изменениями членства в группах и очисткой истории.  Все текущие участники могут управлять
// закреплёнными сообщениями.  Возвращает достоверный тип диалога для минимального события
// аннулирования.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - messageID: Идентификатор сообщения в рамках авторизованного диалога.
//   - pinned: Нужно ли закрепить выбранное сообщение.
//
// @returns:
//   - result1: Идентификатор диалога, в котором изменилось состояние закреплённого
//     сообщения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *PersonalRepository) SetConversationPin(ctx context.Context, user, id, messageID string, pinned bool) (string, error) {
	var kind string
	repo := NewDirectChatRepository(r.db)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := repo.authorize(tx, user, id, true, false); err != nil {
			return err
		}
		cutoff, err := repo.historyCutoff(tx, user, id)
		if err != nil {
			return err
		}
		var message chat.Message
		if err := repo.messages(tx).Where("conversation_id=? AND id=? AND sequence>? AND deleted_at IS NULL AND kind='text'", id, messageID, cutoff).
			Take(&message).Error; err != nil {
			return mapNotFound(err)
		}
		if err := tx.Table("conversations").Select("type").Where("id=?", id).Scan(&kind).Error; err != nil {
			return err
		}
		if !pinned {
			return tx.Exec("DELETE FROM conversation_message_pins WHERE conversation_id=? AND message_id=?", id, messageID).Error
		}
		var existing int64
		if err := tx.Table("conversation_message_pins").Where("conversation_id=? AND message_id=?", id, messageID).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return nil
		}
		// Закрелённое сообщение, очищенное всеми текущими участниками, не должно занимать место
		// вечно.  Все закреплённые сообщения должны быть видны хотя бы одному активному
		// аккаунту.
		if err := tx.Exec(`DELETE FROM conversation_message_pins pins
			USING conversation_messages message
			WHERE pins.conversation_id=? AND message.conversation_id=pins.conversation_id AND message.id=pins.message_id
			AND (message.deleted_at IS NOT NULL OR NOT EXISTS (
				SELECT 1 FROM conversation_members member JOIN users account ON account.id=member.user_id
				WHERE member.conversation_id=pins.conversation_id AND member.left_at IS NULL AND member.hidden_at IS NULL
				AND member.history_cleared_through<message.sequence AND account.guest_conference_id IS NULL AND account.password_hash<>''
			))`, id).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Table("conversation_message_pins pins").
			Joins("JOIN conversation_messages m ON m.conversation_id=pins.conversation_id AND m.id=pins.message_id").
			Where("pins.conversation_id=? AND m.deleted_at IS NULL", id).Count(&count).Error; err != nil {
			return err
		}
		if count >= personal.MaxPinnedMessages {
			return apperrors.New(apperrors.ErrConflict, "Можно закрепить до 20 сообщений. Сначала открепите одно из них.")
		}
		return tx.Exec(`INSERT INTO conversation_message_pins(conversation_id,message_id,pinned_by_user_id)
			VALUES(?::uuid,?::uuid,?::uuid) ON CONFLICT DO NOTHING`, id, messageID, user).Error
	})
	return kind, err
}
