// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package postgres

import (
	"github.com/janickiy/meet-space/internal/domain/chat"
	"gorm.io/gorm"
)

// decorateCallHistory загружает страницу за один ограниченный запрос после авторизации в чате и
// применения фильтра истории чтения.  Данные об устройстве никогда не возвращаются.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - items: Ограниченная коллекция записей для обработки.
//   - ids: Идентификаторы ресурсов для обработки в рамках ограниченного пакета.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
