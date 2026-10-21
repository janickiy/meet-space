// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package personal

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/realtime"
)

type Members interface {
	// Members возвращает текущие идентификаторы участников или их представления для
	// авторизованного диалога.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Собранные строковые значения.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Members(context.Context, string) ([]string, error)
}
type conversationTypes interface {
	// ConversationType возвращает информацию о том, является ли сохраненный диалог прямым или
	// групповым.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Тип сохраненного диалога: прямой или групповой.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ConversationType(context.Context, string) (string, error)
}
type Bus interface {
	// Publish публикует сохраненные изменения в переписке через настроенный транспорт событий
	// или команд.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Доверенное событие для публикации или обработки.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Publish(context.Context, string, realtime.Envelope) error
}

// Events адаптирует приложение для группового чата к доставке данных для каждого аккаунта после
// сохранения.
type Events struct {
	Members Members
	Bus     Bus
}

// Функция Broadcast публикует доверенное событие только для тех участников конференции или
// беседы, которые в данный момент имеют к ней доступ.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - event: Доверенное событие для публикации или обработки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (e *Events) Broadcast(ctx context.Context, event realtime.Envelope) error {
	var m chat.Message
	if err := json.Unmarshal(event.Data, &m); err != nil {
		return err
	}
	kind, err := e.conversationType(ctx, m.ConversationID)
	if err != nil {
		return err
	}
	return e.PublishConversation(ctx, m.ConversationID, strings.TrimPrefix(event.Type, "chat."), map[string]any{"conversationId": m.ConversationID, "type": kind, "message": m})
}

// PublishConversation публикует зафиксированное изменение в диалоге для авторизованных текущих
// участников.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - data: Входные байты или структурированная полезная информация о событии.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (e *Events) PublishConversation(ctx context.Context, id, kind string, data any) error {
	return e.PublishConversationTo(ctx, id, kind, data)
}

// PublishUser отправляет изменения личных настроек или истории только на вкладки владельца
// аккаунта; другие аккаунты не получают уведомления о личных настройках или истории.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - data: Входные байты или структурированная полезная информация о событии.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (e *Events) PublishUser(ctx context.Context, user, kind string, data any) error {
	if kind != "conversation.preferences.updated" && kind != "conversation.history.cleared" && kind != "conversation.hidden" {
		return errors.New("unsupported personal conversation event")
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var payload struct {
		ConversationID string `json:"conversationId"`
		Type           string `json:"type"`
		UserID         string `json:"userId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	id, err := uuid.Parse(payload.ConversationID)
	validType := payload.Type == "direct" || (payload.Type == "group" && kind == "conversation.preferences.updated")
	if user == "" || payload.UserID != user || !validType || err != nil || id == uuid.Nil || id.String() != payload.ConversationID {
		return errors.New("invalid personal conversation event payload")
	}
	return e.Bus.Publish(ctx, user, realtime.Event(kind, "", data))
}

// PublishConversationTo также отправляет минимальное событие жизненного цикла удаленному
// пользователю.  Сначала сохраняются данные отправителя; дополнительные получатели никогда не
// должны получать личные сообщения.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - data: Входные байты или структурированная полезная информация о событии.
//   - extraUsers: Дополнительные авторизованные получатели, включенные в зафиксированное
//     событие изменения группы.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (e *Events) PublishConversationTo(ctx context.Context, id, kind string, data any, extraUsers ...string) error {
	ids, err := e.Members.Members(ctx, id)
	if err != nil {
		return err
	}
	if len(extraUsers) > 0 && kind != "conversation.member.removed" {
		return errors.New("extra recipients require a membership removal event")
	}
	ids = append(ids, extraUsers...)
	seen := make(map[string]struct{}, len(ids))
	event := realtime.Event(kind, "", data)
	for _, user := range ids {
		if _, exists := seen[user]; exists {
			continue
		}
		seen[user] = struct{}{}
		err = errors.Join(err, e.Bus.Publish(ctx, user, event))
	}
	return err
}

// SendToParticipant публикует доверенное событие для физических сеансов выбранного
// авторизованного участника.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - event: Доверенное событие для публикации или обработки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (e *Events) SendToParticipant(ctx context.Context, id, user string, event realtime.Envelope) error {
	var state chat.ReadState
	if err := json.Unmarshal(event.Data, &state); err != nil {
		return err
	}
	kind, err := e.conversationType(ctx, id)
	if err != nil {
		return err
	}
	return e.Bus.Publish(ctx, user, realtime.Event("conversation.read.updated", "", map[string]any{"conversationId": id, "type": kind, "state": state}))
}

// conversationType определяет тип сохраненной переписки, прежде чем выбрать способ доставки
// событий.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Тип сохраненного диалога: прямой или групповой.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (e *Events) conversationType(ctx context.Context, id string) (string, error) {
	if types, ok := e.Members.(conversationTypes); ok {
		return types.ConversationType(ctx, id)
	}
	// Совместимость с существующими адаптерами, поддерживающими только прямую отправку.
	return "direct", nil
}
