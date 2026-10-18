package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/realtime"
	goredis "github.com/redis/go-redis/v9"
)

// Граница аренды начинается с момента подтверждения приема от API и ограничивается временем
// приема от Redis TIME плюс TTL; часы API и Redis должны быть синхронизированы.  Ключ маршрута
// с истекающим сроком действия является достоверным.  Индекс сохраняет метаданные, необходимые
// для сообщения об ошибке после истечения срока действия ключа.  Все изменения являются
// атомарными.
var presenceScript = goredis.NewScript(`
local prefix = ARGV[1]
local op = ARGV[2]
local clock = redis.call('TIME')
local now = tonumber(clock[1])*1000 + math.floor(tonumber(clock[2])/1000)
local meta = prefix .. ':metadata'
local expiry = prefix .. ':expiry'
local channel = prefix .. ':bus'
local function remove(id, kind)
    local raw = redis.call('HGET', meta, id)
    if raw then
        local s = cjson.decode(raw)
        redis.call('ZREM', prefix .. ':conference:' .. s.conferenceId, id)
        redis.call('PUBLISH', channel, cjson.encode({kind=kind, conferenceId=s.conferenceId, session=s}))
    end
    redis.call('DEL', prefix .. ':route:' .. id)
    redis.call('HDEL', meta, id)
    redis.call('ZREM', expiry, id)
end
for _, id in ipairs(redis.call('ZRANGEBYSCORE', expiry, '-inf', now, 'LIMIT', 0, 100)) do
    remove(id, 'expired')
end
local id = ARGV[3]
if op == 'join' then
    local raw = ARGV[4]
    local ttl = tonumber(ARGV[5])
    local deadline = math.min(tonumber(ARGV[6]), now+ttl)
    if deadline <= now then return 0 end
    local s = cjson.decode(raw)
    if redis.call('HEXISTS', meta, id) == 1 then return 0 end
    redis.call('HSET', meta, id, raw)
    redis.call('ZADD', expiry, deadline, id)
    redis.call('ZADD', prefix .. ':conference:' .. s.conferenceId, deadline, id)
    redis.call('SET', prefix .. ':route:' .. id, raw, 'PXAT', deadline)
    redis.call('PUBLISH', channel, cjson.encode({kind='connected', conferenceId=s.conferenceId, session=s}))
    return 1
elseif op == 'leave' then
    remove(id, 'disconnected')
    return 1
elseif op == 'touch' then
    local raw = redis.call('GET', prefix .. ':route:' .. id)
    if not raw then return 0 end
    local ttl = tonumber(ARGV[5])
    local deadline = math.min(tonumber(ARGV[6]), now+ttl)
    if deadline <= now then return 0 end
    local s = cjson.decode(raw)
    s.lastSeenAt = ARGV[4]
    raw = cjson.encode(s)
    redis.call('SET', prefix .. ':route:' .. id, raw, 'PXAT', deadline)
    redis.call('HSET', meta, id, raw)
    redis.call('ZADD', expiry, deadline, id)
    redis.call('ZADD', prefix .. ':conference:' .. s.conferenceId, deadline, id)
    return 1
elseif op == 'active' then
    local result = {}
    for _, cid in ipairs(redis.call('ZRANGEBYSCORE', prefix .. ':conference:' .. id, now+1, '+inf')) do
        local raw = redis.call('GET', prefix .. ':route:' .. cid)
        if raw then table.insert(result, raw) end
    end
    return result
end
return 1
`)

// RealtimeStore сохраняет данные о распределенном присутствии и одноразовые тикеты, а также
// доставляет события Redis с ограниченной областью видимости.
type RealtimeStore struct {
	client *goredis.Client
	prefix string
}

// NewRealtimeStore создает хранилище в реальном времени с зависимостями и ограничениями,
// необходимыми для распределенного контроля присутствия и владения.
//
// @Parameters:
//   - client: Идентификация медиа-клиента браузера или зависимость от транспорта, определяемая
//     его типом.
//   - prefix: Объект или префикс ключа, управляемый сервером.
//
// @returns:
//   - result1: Созданное или загруженное хранилище в реальном времени.
func NewRealtimeStore(client *goredis.Client, prefix string) *RealtimeStore {
	return &RealtimeStore{client: client, prefix: prefix}
}

// action атомарно выполняет выбранную операцию Redis Lua в пространстве имен компонента.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - op: Атомарная операция или команда, выбранная компонентом.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - raw: Недоверенное сериализованное значение для декодирования или проверки.
//   - ttl: Срок действия, назначенный для временного значения или записи о владельце.
//   - deadline: Абсолютный срок, ограничивающий время существования ресурса.
//
// @returns:
//   - result1: Созданная или загруженная команда cmd.
func (s *RealtimeStore) action(ctx context.Context, op, id, raw string, ttl time.Duration, deadline time.Time) *goredis.Cmd {
	return presenceScript.Run(ctx, s.client, []string{s.prefix + ":metadata", s.prefix + ":expiry"}, s.prefix, op, id, raw, ttl.Milliseconds(), deadline.UnixMilli())
}

// Register регистрирует физическое подключение и время его ограниченного присутствия.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - session: Постоянное или физическое состояние сессии, определяемое ее типом.
//   - ttl: Срок действия, назначенный для временного значения или записи о владельце.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *RealtimeStore) Register(ctx context.Context, session realtime.Session, ttl time.Duration) error {
	if session.LastSeenAt.IsZero() || ttl < time.Millisecond {
		return apperrors.ErrConflict
	}
	raw, err := json.Marshal(session)
	if err != nil {
		return err
	}
	n, err := s.action(ctx, "join", session.ConnectionID, string(raw), ttl, session.LastSeenAt.Add(ttl)).Int()
	if err == nil && n != 1 {
		return apperrors.ErrConflict
	}
	return err
}

// Unregister закрывает физическую сессию и обновляет данные о распределенном присутствии
// участника.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *RealtimeStore) Unregister(ctx context.Context, id string) error {
	return s.action(ctx, "leave", id, "", 0, time.Time{}).Err()
}

// Функция Touch обновляет данные о присутствии, используя предоставленное время подтвержденной
// активности на транспортном уровне.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - confirmedAt: Временная метка подтвержденного на транспортном уровне ответа Pong или
//     другого подтверждения активности.
//   - ttl: Срок действия, назначенный для временного значения или записи о владельце.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *RealtimeStore) Touch(ctx context.Context, id string, confirmedAt time.Time, ttl time.Duration) error {
	if confirmedAt.IsZero() || ttl < time.Millisecond {
		return apperrors.ErrNotFound
	}
	n, err := s.action(ctx, "touch", id, confirmedAt.UTC().Format(time.RFC3339Nano), ttl, confirmedAt.Add(ttl)).Int()
	if err == nil && n != 1 {
		return apperrors.ErrNotFound
	}
	return err
}

// Get загружает запрошенный редис в авторизованную область видимости вызывающей стороны.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Результирующий сеанс.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *RealtimeStore) Get(ctx context.Context, id string) (realtime.Session, error) {
	raw, err := s.client.Get(ctx, s.prefix+":route:"+id).Bytes()
	if errors.Is(err, goredis.Nil) {
		return realtime.Session{}, apperrors.ErrNotFound
	}
	var session realtime.Session
	if err == nil {
		err = json.Unmarshal(raw, &session)
	}
	return session, err
}

// Missing проверяет наличие маршрута без чтения сессии JSON.  Каждый конвейер ограничен
// размером страницы для сверки данных.  Частичные ответы при ошибках на транспортном уровне
// игнорируются, поэтому сбой в работе Redis не приводит к закрытию активных сессий SQL.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - ids: Идентификаторы ресурсов для обработки в рамках ограниченного пакета.
//
// @returns:
//   - result1: Собранные строковые значения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *RealtimeStore) Missing(ctx context.Context, ids []string) ([]string, error) {
	var missing []string
	for start := 0; start < len(ids); start += 500 {
		batch := ids[start:min(start+500, len(ids))]
		commands := make([]*goredis.IntCmd, len(batch))
		_, err := s.client.Pipelined(ctx, func(pipe goredis.Pipeliner) error {
			for i, id := range batch {
				commands[i] = pipe.Exists(ctx, s.prefix+":route:"+id)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		for i, command := range commands {
			if command.Val() == 0 {
				missing = append(missing, batch[i])
			}
		}
	}
	return missing, nil
}

// Active возвращает физические сессии, время присутствия которых еще не истекло.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Собранные значения сеанса.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *RealtimeStore) Active(ctx context.Context, conferenceID string) ([]realtime.Session, error) {
	raw, err := s.action(ctx, "active", conferenceID, "", 0, time.Time{}).StringSlice()
	if err != nil {
		return nil, err
	}
	result := make([]realtime.Session, 0, len(raw))
	for _, entry := range raw {
		var session realtime.Session
		if err := json.Unmarshal([]byte(entry), &session); err != nil {
			return nil, err
		}
		result = append(result, session)
	}
	return result, nil
}

// Prune удаляет физические сессии с истекшим временем присутствия и возвращает состояние,
// необходимое для сверки данных о присутствии.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *RealtimeStore) Prune(ctx context.Context) error {
	return s.action(ctx, "prune", "", "", 0, time.Time{}).Err()
}

// Publish — публикует сохраненные изменения в Redis через настроенный транспорт событий или команд.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - bus: Публикация и подписка с ограничением области видимости.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *RealtimeStore) Publish(ctx context.Context, bus realtime.Bus) error {
	raw, err := json.Marshal(bus)
	if err != nil {
		return err
	}
	return s.client.Publish(ctx, s.prefix+":bus", raw).Err()
}

// Subscribe открывает подписку на события с областью видимости, время жизни которой ограничено
// предоставленным контекстом.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: Полученная подписка.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *RealtimeStore) Subscribe(ctx context.Context) (realtime.Subscription, error) {
	p := s.client.Subscribe(ctx, s.prefix+":bus")
	if _, err := p.Receive(ctx); err != nil {
		_ = p.Close()
		return nil, err
	}
	return &realtimeSubscription{p}, nil
}

// realtimeSubscription оборачивает одну подписку Redis Pub/Sub и ограничивает время ее
// существования.
type realtimeSubscription struct{ pub *goredis.PubSub }

// Close закрывает ресурсы, принадлежащие компоненту, и завершает их жизненный цикл.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *realtimeSubscription) Close() error { return s.pub.Close() }

// Receive ожидает и декодирует следующее событие подписки с ограничением области видимости в
// течение времени существования отмены подписки, заданного вызывающей стороной.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: Результирующая шина.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *realtimeSubscription) Receive(ctx context.Context) (realtime.Bus, error) {
	m, err := s.pub.ReceiveMessage(ctx)
	if err != nil {
		return realtime.Bus{}, err
	}
	var bus realtime.Bus
	err = json.Unmarshal([]byte(m.Payload), &bus)
	return bus, err
}

// ticketKey создает ключ Redis с пространством имен для одноразового билета подключения.
//
// @Parameters:
//   - ticket: Одноразовый билет подключения или его идентификатор на стороне сервера.
//
// @returns:
//   - result1: Ключ одноразового билета с пространством имен.
func (s *RealtimeStore) ticketKey(ticket string) string {
	hash := sha256.Sum256([]byte(ticket))
	return s.prefix + ":ticket:" + hex.EncodeToString(hash[:])
}

// SaveTicket сохраняет одноразовый билет подключения с ограниченным сроком действия.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - ticket: Одноразовый билет подключения или его идентификатор на стороне сервера.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - identity: Подтвержденный аккаунт и идентификатор физического сеанса.
//   - ttl: Срок действия, назначенный для временного значения или записи о владельце.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *RealtimeStore) SaveTicket(ctx context.Context, ticket, conferenceID string, identity realtime.Identity, ttl time.Duration) error {
	raw, err := json.Marshal(struct {
		ConferenceID string            `json:"conferenceId"`
		Identity     realtime.Identity `json:"identity"`
	}{conferenceID, identity})
	if err != nil {
		return err
	}
	return s.client.Set(ctx, s.ticketKey(ticket), raw, ttl).Err()
}

// ConsumeTicket атомарно использует билет подключения, чтобы его нельзя было воспроизвести.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - ticket: Одноразовый билет подключения или его идентификатор на стороне сервера.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Полученный идентификатор.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *RealtimeStore) ConsumeTicket(ctx context.Context, ticket, conferenceID string) (realtime.Identity, error) {
	if len(ticket) != 43 {
		return realtime.Identity{}, apperrors.ErrUnauthorized
	}
	raw, err := s.client.GetDel(ctx, s.ticketKey(ticket)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return realtime.Identity{}, apperrors.ErrUnauthorized
	}
	if err != nil {
		return realtime.Identity{}, err
	}
	var value struct {
		ConferenceID string            `json:"conferenceId"`
		Identity     realtime.Identity `json:"identity"`
	}
	if json.Unmarshal(raw, &value) != nil || value.ConferenceID != conferenceID || !value.Identity.ExpiresAt.After(time.Now()) {
		return realtime.Identity{}, apperrors.ErrUnauthorized
	}
	return value.Identity, nil
}
