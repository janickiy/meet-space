// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package redis

import (
	"context"
	"encoding/json"

	domain "github.com/janickiy/meet-space/internal/domain/realtime"
	goredis "github.com/redis/go-redis/v9"
)

// NotificationBus изолирует персональные уведомления в каналах Redis, привязанных к конкретному
// аккаунту.
type NotificationBus struct {
	client *goredis.Client
	prefix string
}

// NewNotificationBus создает шину уведомлений с зависимостями и ограничениями, необходимыми для
// распределенного присутствия и владения.
//
// @Parameters:
//   - client: Идентификация медиа-клиента браузера или зависимость от транспорта, определяемая
//     его типом.
//   - namespace: Пространство имен, изолирующее распределенные ключи и каналы этого компонента.
//
// @returns:
//   - result1: Созданная или загруженная шина уведомлений.
func NewNotificationBus(client *goredis.Client, namespace string) *NotificationBus {
	return &NotificationBus{client: client, prefix: namespace + ":notification:"}
}

// Publish — публикует сохраненные изменения в Redis через настроенный транспорт событий или команд.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - event: Доверенное событие для публикации или обработки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (b *NotificationBus) Publish(ctx context.Context, userID string, event domain.Envelope) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return b.client.Publish(ctx, b.prefix+userID, data).Err()
}

// Subscribe открывает подписку на события с областью видимости, время жизни которой ограничено
// предоставленным контекстом.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: Созданная или загруженная шина pub sub.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (b *NotificationBus) Subscribe(ctx context.Context, userID string) (*goredis.PubSub, error) {
	sub := b.client.Subscribe(ctx, b.prefix+userID)
	if _, err := sub.Receive(ctx); err != nil {
		_ = sub.Close()
		return nil, err
	}
	return sub, nil
}
