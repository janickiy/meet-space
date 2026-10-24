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
	"github.com/janickiy/meet-space/internal/domain/personal"
	"gorm.io/gorm"
)

// lockDirectConversation блокирует членство вызывающего абонента в прямых беседах, прежде чем
// изменить его личные настройки или историю.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// SetConversationPreferences обновляет только настройки уведомлений о беседах действующего
// участника.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - enabled: Включает или отключает запрошенную функцию или настройку.
//
// @returns:
//   - result1: Полученная беседа.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// clearDirectHistory перемещает границу истории конкретного участника, не удаляя сообщения
// другого участника.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - hide: Нужно ли также скрыть представление беседы конкретного участника.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// ClearDirectHistory перемещает только границу истории прямого чата действующего участника, не
// удаляя историю собеседника.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Полученная беседа.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// HideDirectConversation скрывает прямые беседы для действующего аккаунта и сохраняет
// сохранённую историю переписки с другим участником.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
