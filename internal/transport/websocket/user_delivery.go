// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/personal"
	domain "github.com/janickiy/meet-space/internal/domain/realtime"
)

type conversationAccessChecker interface {
	// HasActiveMembership проверяет текущий статус участия в диалоге учетной записи
	// непосредственно перед предоставлением защищенного доступа или отправкой данных.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//
	// @returns:
	//   - result1: Указывает, выполняется ли задокументированное условие операции и
	//   произошло ли успешное изменение ее условного состояния.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	HasActiveMembership(context.Context, string, string) (bool, error)
}

// Модуль доставки сериализует запись в сокет с ограничением на отзыв членства.  Для
// одновременной доставки используется разделяемая блокировка, а для изменения членства —
// монопольная блокировка.  Таким образом, после завершения удаления сообщение из очереди не
// может быть доставлено.
type conversationDeliveryGuard interface {
	// WithConversationDelivery подключает доставку сообщений к компоненту до того, как будут
	// обработаны его операции.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg4: Обратный вызов доставки вызывается только до тех пор, пока действителен
	//   получатель или авторизация ресурса.
	//
	// @returns:
	//   - result1: Указывает, выполняется ли задокументированное условие операции и
	//   произошло ли успешное изменение ее условного состояния.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	WithConversationDelivery(context.Context, string, string, func() error) (bool, error)
}

// conversationMessageDeliveryGuard предотвращает обход очереди сообщений с задержкой, что
// позволяет избежать очистки истории, и предоставляет текущие персональные настройки
// получателя.
type conversationMessageDeliveryGuard interface {
	// WithConversationMessageDelivery подключает доставку сообщений к компоненту до того, как
	// будут обработаны его операции.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg4: Идентификатор сообщения в рамках авторизованного диалога.
	//   - arg5: Обратный вызов доставки вызывается только до тех пор, пока действителен
	//   получатель или авторизация ресурса.
	//
	// @returns:
	//   - result1: Указывает, выполняется ли задокументированное условие операции и
	//   произошло ли успешное изменение ее условного состояния.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	WithConversationMessageDelivery(context.Context, string, string, string, func(bool, int64) error) (bool, error)
}

// conversationActionDeliveryGuard заменяет отложенные персональные данные текущим
// представлением владельца и не позволяет скрыть диалог, который уже был восстановлен.
type conversationActionDeliveryGuard interface {
	// WithConversationActionDelivery привязывает доставку действий в диалоге к компоненту до
	// того, как будут выполнены его операции.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg4: Подтвержденное событие, задание, категория медиаданных или ресурсов.
	//   - arg5: Обратный вызов доставки вызывается только до тех пор, пока действителен
	//   получатель или авторизация ресурса.
	//
	// @returns:
	//   - result1: Указывает, выполняется ли задокументированное условие операции и
	//   произошло ли успешное изменение ее условного состояния.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	WithConversationActionDelivery(context.Context, string, string, string, func(*personal.Conversation) error) (bool, error)
}

// deliver проверяет текущие права доступа получателя непосредственно перед доставкой
// сохраненного события или команды.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - data: Входные байты или структурированная полезная информация о событии.
//   - write: Обратный вызов, который записывает авторизованное сериализованное событие в
//     принимающий сокет.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *UserHandler) deliver(ctx context.Context, user string, data []byte, write func([]byte) error) error {
	var event domain.Envelope
	if json.Unmarshal(data, &event) != nil {
		return nil
	}
	if !strings.HasPrefix(event.Type, "message.") && !strings.HasPrefix(event.Type, "conversation.") {
		return write(data)
	}
	var payload struct {
		ConversationID string        `json:"conversationId"`
		Type           string        `json:"type"`
		UserID         string        `json:"userId"`
		Message        *chat.Message `json:"message"`
	}
	if json.Unmarshal(event.Data, &payload) != nil || (payload.Type != "direct" && payload.Type != "group") {
		return nil
	}
	conversation, err := uuid.Parse(payload.ConversationID)
	if err != nil || conversation == uuid.Nil || conversation.String() != payload.ConversationID {
		return nil
	}
	// Для отмены подписки пользователю достаточно этого события жизненного цикла, чтобы
	// очистить локальные кэши.  Отклоняйте запросы на отмену подписки, содержащие
	// метаданные/сообщения, прежде чем обходить аутентификацию.
	if event.Type == "conversation.member.removed" && payload.UserID == user {
		var fields map[string]json.RawMessage
		if json.Unmarshal(event.Data, &fields) != nil {
			return nil
		}
		for key := range fields {
			if key != "conversationId" && key != "type" && key != "userId" && key != "role" {
				return nil
			}
		}
		return write(data)
	}
	if event.Type == "conversation.preferences.updated" || event.Type == "conversation.history.cleared" || event.Type == "conversation.hidden" {
		// Никогда не публикуйте персональные настройки или данные об очистке истории в
		// диалоговом узле.
		if payload.UserID != user || (payload.Type == "group" && event.Type != "conversation.preferences.updated") {
			return nil
		}
		if guard, ok := h.accounts.(conversationActionDeliveryGuard); ok {
			_, err := guard.WithConversationActionDelivery(ctx, payload.ConversationID, user, event.Type, func(item *personal.Conversation) error {
				if item == nil {
					return errors.New("missing authoritative conversation state")
				}
				fields := map[string]any{"conversationId": payload.ConversationID, "type": item.Type, "userId": user}
				fields["historyClearedThrough"] = item.HistoryClearedThrough
				if event.Type == "conversation.hidden" {
					fields["hidden"] = true
				} else {
					fields["item"] = *item
				}
				var err error
				event.Data, err = json.Marshal(fields)
				if err != nil {
					return err
				}
				outbound, err := json.Marshal(event)
				if err != nil {
					return err
				}
				return write(outbound)
			})
			return err
		}
	}
	if strings.HasPrefix(event.Type, "message.") {
		if guard, ok := h.accounts.(conversationMessageDeliveryGuard); ok {
			if payload.Message == nil || payload.Message.ConversationID != payload.ConversationID {
				return nil
			}
			messageID, err := uuid.Parse(payload.Message.ID)
			if err != nil || messageID == uuid.Nil || messageID.String() != payload.Message.ID {
				return nil
			}
			_, err = guard.WithConversationMessageDelivery(ctx, payload.ConversationID, user, payload.Message.ID, func(notificationsEnabled bool, cutoff int64) error {
				message := *payload.Message
				if cutoff > 0 && message.ReplyPreview != nil && message.ReplyPreview.Sequence <= cutoff {
					message.ReplyPreview = nil
					message.ReplyTo = nil
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(event.Data, &fields); err != nil {
					return err
				}
				fields["message"], err = json.Marshal(message)
				if err != nil {
					return err
				}
				fields["notificationsEnabled"], err = json.Marshal(notificationsEnabled)
				if err != nil {
					return err
				}
				fields["historyClearedThrough"], err = json.Marshal(cutoff)
				if err != nil {
					return err
				}
				event.Data, err = json.Marshal(fields)
				if err != nil {
					return err
				}
				outbound, err := json.Marshal(event)
				if err != nil {
					return err
				}
				return write(outbound)
			})
			return err
		}
	}
	if guard, ok := h.accounts.(conversationDeliveryGuard); ok {
		_, err := guard.WithConversationDelivery(ctx, payload.ConversationID, user, func() error { return write(data) })
		return err
	}
	if checker, ok := h.accounts.(conversationAccessChecker); ok {
		allowed, err := checker.HasActiveMembership(ctx, payload.ConversationID, user)
		if err != nil || !allowed {
			return err
		}
		return write(data)
	}
	// Совместимость с тестовыми адаптерами, поддерживающими только прямую связь; групповая
	// доставка не работает при закрытии.
	if payload.Type == "direct" {
		return write(data)
	}
	return nil
}
