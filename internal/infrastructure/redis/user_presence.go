package redis

import (
	"context"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	goredis "github.com/redis/go-redis/v9"
)

// Одна аренда на сокет физического аккаунта. Обрезка и мутация являются атомарными операциями в
// APIs.
type UserPresence struct {
	client *goredis.Client
	prefix string
	ttl    time.Duration
}

// NewUserPresence формирует данные о присутствии пользователя с учетом зависимостей и
// ограничений, необходимых для распределенного присутствия и владения.
//
// @Parameters:
//   - c: Клиент Redis, используемый для атомарных операций с данными о присутствии и владении.
//   - namespace: Пространство имен, изолирующее распределенные ключи и каналы этого компонента.
//   - ttl: Срок действия, назначенный для временного значения или записи о владельце.
//
// @returns:
//   - result1: Сформированные или загруженные данные о присутствии пользователя.
func NewUserPresence(c *goredis.Client, namespace string, ttl time.Duration) *UserPresence {
	return &UserPresence{c, namespace + ":user-presence:", ttl}
}

var userPresenceScript = goredis.NewScript(`
local clock=redis.call('TIME');local now=clock[1]*1000+math.floor(clock[2]/1000)
redis.call('ZREMRANGEBYSCORE',KEYS[1],'-inf',now)
if ARGV[2]=='remove' then redis.call('ZREM',KEYS[1],ARGV[1]) else redis.call('ZADD',KEYS[1],now+tonumber(ARGV[3]),ARGV[1]) end
local n=redis.call('ZCARD',KEYS[1]);if n==0 then redis.call('DEL',KEYS[1]) else redis.call('PEXPIRE',KEYS[1],ARGV[3]) end
return n`)

var userPresenceCountScript = goredis.NewScript(`
local clock=redis.call('TIME');local now=clock[1]*1000+math.floor(clock[2]/1000)
return redis.call('ZCOUNT',KEYS[1],'('..now,'+inf')`)

// Функция Touch обновляет данные о присутствии, используя предоставленное время подтвержденной
// активности на транспортном уровне.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - connection: Текущая интеграция учетной записи или физическое транспортное соединение.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *UserPresence) Touch(ctx context.Context, user, connection string) (int64, error) {
	return p.change(ctx, user, connection, "touch")
}

// Remove удаляет указанного физического клиента из набора данных о присутствии учетной записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - connection: Текущая интеграция учетной записи или физическое транспортное соединение.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *UserPresence) Remove(ctx context.Context, user, connection string) (int64, error) {
	return p.change(ctx, user, connection, "remove")
}

// change атомарно обновляет набор данных о присутствии учетной записи и возвращает его текущее
// состояние.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - action: Запрошенное действие по управлению жизненным циклом или модерации.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *UserPresence) change(ctx context.Context, user, id, action string) (int64, error) {
	return userPresenceScript.Run(ctx, p.client, []string{p.prefix + user}, id, action, p.ttl.Milliseconds()).Int64()
}

// Count возвращает количество активных физических сессий для учетной записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *UserPresence) Count(ctx context.Context, user string) (int64, error) {
	return userPresenceCountScript.Run(ctx, p.client, []string{p.prefix + user}).Int64()
}

// На странице используется одна команда Redis и общие часы Redis, не зависящие от часов API.
const userPresenceOnlineScript = `
local clock=redis.call('TIME');local now=clock[1]*1000+math.floor(clock[2]/1000)
local result={}
for i,key in ipairs(KEYS) do result[i]=redis.call('ZCOUNT',key,'('..now,'+inf') end
return result`

// Online — онлайн-проекты подтверждают глобальную аренду учетных записей без продления срока их
// действия.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - users: Идентификаторы учетных записей или репозиторий учетных записей в соответствии с их
//     типом.
//
// @returns:
//   - result1: Результирующая карта [строка]bool.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *UserPresence) Online(ctx context.Context, users []string) (map[string]bool, error) {
	if len(users) > 100 {
		return nil, apperrors.ErrInvalidInput
	}
	keys, ids := []string{}, []string{}
	seen := make(map[string]bool, len(users))
	for _, user := range users {
		if user == "" || seen[user] {
			continue
		}
		seen[user] = true
		keys, ids = append(keys, p.prefix+user), append(ids, user)
	}
	result := make(map[string]bool, len(ids))
	if len(keys) == 0 {
		return result, nil
	}
	// EVAL позволяет избежать дополнительной повторной попытки NOSCRIPT при первом поиске в
	// экземпляре API.
	counts, err := p.client.Eval(ctx, userPresenceOnlineScript, keys).Int64Slice()
	if err != nil {
		return nil, err
	}
	if len(counts) != len(ids) {
		return nil, apperrors.ErrUnavailable
	}
	for i, id := range ids {
		result[id] = counts[i] > 0
	}
	return result, nil
}
