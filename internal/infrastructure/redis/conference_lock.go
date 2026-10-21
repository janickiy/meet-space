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
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const conferenceLockPrefix = "record:conference:"

var releaseLockScript = goredis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

var acquireLockScript = goredis.NewScript(`
local owner = redis.call("GET", KEYS[1])
if owner == ARGV[1] then
    redis.call("PEXPIRE", KEYS[1], ARGV[2])
    return 1
end
if not owner then
    redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
    return 1
end
return 0
`)

// ConferenceLock поддерживает блокировку Redis для одной активной записи на конференцию.
type ConferenceLock struct {
	client *goredis.Client
	ttl    time.Duration
}

// NewConferenceLock создает блокировку конференции с зависимостями и ограничениями,
// необходимыми для распределенного присутствия и владения.
//
// @Parameters:
//   - client: Идентификация медиа-клиента браузера или зависимость от транспорта, определяемая
//     его типом.
//   - ttl: Срок действия, назначенный для временного значения или записи о владельце.
//
// @returns:
//   - result1: Созданная или загруженная блокировка конференции.
func NewConferenceLock(client *goredis.Client, ttl time.Duration) *ConferenceLock {
	return &ConferenceLock{client: client, ttl: ttl}
}

// Атомарная операция Acquire устанавливает блокировку записи конференции, владельцем которой
// является указанная запись.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - recordID: Внешняя запись UUID.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (l *ConferenceLock) Acquire(ctx context.Context, conferenceID string, recordID string) (bool, error) {
	if l == nil || l.client == nil {
		return true, nil
	}
	ttl := l.ttl
	if ttl <= 0 {
		ttl = 6 * time.Hour
	}
	result, err := acquireLockScript.Run(ctx, l.client, []string{lockKey(conferenceID)}, recordID, ttl.Milliseconds()).Int()
	if err != nil {
		return false, fmt.Errorf("acquire conference record lock: %w", err)
	}

	return result == 1, nil
}

// Release — при освобождении освобождается только ресурс Redis, принадлежащий соответствующей операции
// или токену.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - recordID: Внешняя запись UUID.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (l *ConferenceLock) Release(ctx context.Context, conferenceID string, recordID string) error {
	if l == nil || l.client == nil || strings.TrimSpace(conferenceID) == "" || strings.TrimSpace(recordID) == "" {
		return nil
	}
	if err := releaseLockScript.Run(ctx, l.client, []string{lockKey(conferenceID)}, recordID).Err(); err != nil {
		return fmt.Errorf("release conference record lock: %w", err)
	}

	return nil
}

// lockKey создает ключ Redis с пространством имен для блокировки записи конференции.
//
// @Parameters:
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Ключ блокировки записи конференции с пространством имен.
func lockKey(conferenceID string) string {
	return conferenceLockPrefix + strings.TrimSpace(conferenceID) + ":lock"
}
