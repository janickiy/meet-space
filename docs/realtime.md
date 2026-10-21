# Realtime, присутствие и сигнализация

Актуализировано 2026-10-21 по коду `6e577b6cced4e34d1c031353fbb9055909055185`.

## Текущие каналы

Conference WebSocket обслуживает присутствие, события встречи и сигнализацию
`media.*` для [SFU](media-sfu.md). Аудио/видео передаются по WebRTC браузер ↔ SFU,
не через WS, API или RabbitMQ.

Account WebSocket `/api/v1/ws` доставляет личные сообщения, presence аккаунтов,
события звонков и уведомления по персональным каналам. Отдельные SSE endpoints
уведомлений используют существующую инфраструктуру событий; REST остаётся
источником сохранённых данных при пропущенных уведомлениях.

Общие свойства: одноразовые билеты, проверка авторизации и прав, Redis Pub/Sub,
native ping/pong, bounded queues, reconnect и закрытие доступа после отзыва сессии.
Conference presence учитывает каждую физическую вкладку и не удаляет membership
при транспортном отключении.

## Архитектурные решения

ConferenceParticipant — постоянное членство. ParticipantSession — история отдельного подключения. Закрытие вкладки не вызывает REST leave и не удаляет membership. Несколько вкладок имеют разные connectionId и один participantId.

HTTP transport занимается аутентификацией и upgrade, WebSocket client — чтением, единственным write loop и ограниченной очередью. ConferenceHub предоставляет Register, Unregister, Broadcast, SendToParticipant, SendToConnection и GetActiveSessions. Он зависит от интерфейсов repository, store и socket, не от Pion.

Все межсерверные события, включая события своего API, доставляются только через Redis Pub/Sub. Локальной дополнительной доставки нет, поэтому publisher не создаёт дубликат. Локальный registry хранит socket objects с неизменяемой идентичностью сессии. Presence и routing не вычисляются только по нему.

После committed REST-изменения observer уведомляет hub. Ошибка публикации не превращает успешную транзакцию в ложный HTTP failure; проверка membership при signaling и heartbeat обеспечивает резервное закрытие. Миграции применяются в транзакции под PostgreSQL advisory lock, чтобы два запуска API не конкурировали за таблицы и триггеры.

WebSocket transport использует [Gorilla WebSocket](https://pkg.go.dev/github.com/gorilla/websocket). Ping, pong и close приложения проходят через один writer; reader не пишет application frames.

## Основные файлы

- `internal/domain/realtime/realtime.go`: сессия, identity, protocol envelope, snapshot, ICE config и внутренние bus messages.
- `internal/usecase/realtime/hub.go`: hub, routing, snapshots, janitor, registry и shutdown.
- `internal/infrastructure/redis/realtime.go`: atomic Lua presence, leases, tickets и Pub/Sub adapter.
- `internal/infrastructure/postgres/session_repository.go`: authorization, история подключения и восстановление после crash.
- `internal/transport/websocket/handler.go`: endpoints, upgrade, reader/writer, limits и heartbeat.
- `internal/config/realtime.go`, `.env.example`: отдельная конфигурация realtime для API.
- `database/migrations/000009_create_participant_sessions_table.*.sql`: схема и индексы.
- `internal/app/api.go`, `internal/transport/http/router.go`, `internal/infrastructure/security/jwt.go`: подключение слоя, graceful shutdown, безопасные логи и проверенная дата истечения JWT.
- `internal/usecase/conferences/service.go`: observer после REST mutations.
- `frontend/src/realtime.ts`, `frontend/src/components/RealtimePanel.tsx`: presence, reconnect и события; подключение в ConferencePage, типы, API методы и стили.
- `frontend/nginx.conf.template`, `dockers/https/nginx.conf`, `frontend/vite.config.ts`: WS proxy, сохранение Host с портом и исключение ticket query из access logs.
- `internal/usecase/media`, `internal/transport/mediaworker`, `internal/infrastructure/sfu`: текущая SFU сигнализация/медиа.

## PostgreSQL и Redis

Таблица `participant_sessions`: UUID id, conference_id, participant_id, user_id, уникальный UUID connection_id, status connected/disconnected, connected_at, disconnected_at, last_seen_at. Composite foreign key гарантирует, что participant, conference и user относятся к одному membership. CHECK constraints проверяют статус и порядок дат. Несколько сессий одного участника разрешены.

PostgreSQL записывается на open/close, не на каждый pong. При закрытии сохраняется последнее подтверждённое время жизни. У неудавшегося upgrade также остаётся закрытая запись истории. Janitor проверяет старые connected записи страницами по 500 и исправляет SQL-only crash между commit и регистрацией в Redis.

Redis namespace по умолчанию `go-recorder:realtime:v1`. Все API одной платформы должны использовать одинаковые PostgreSQL, Redis database и namespace.

| Ключ | Назначение |
| --- | --- |
| `:route:<connectionId>` | JSON сессии с физическим TTL, по умолчанию 5 секунд |
| `:conference:<conferenceId>` | Sorted set активных connectionId и сроков lease |
| `:expiry` и `:metadata` | Индекс истечения и сведения для crash cleanup |
| `:bus` | Pub/Sub канал внутренних событий |
| `:ticket:<sha256>` | Одноразовый билет с TTL до 45 секунд |
| `:ticket-limit:<userId>` | До 30 выдач билета за минуту |

Lua использует Redis TIME для проверки и ограничения абсолютного срока lease;
API сохраняет deadline последнего подтверждённого pong. Часы API и Redis должны
быть синхронизированы. Sorted set содержит индивидуальные сроки, а routing key имеет настоящий Redis TTL. Истёкший routing key никогда не считается online, даже до janitor. Metadata сохраняется до atomic prune, чтобы после crash можно было объявить disconnect и завершить историю. Janitor работает с интервалом не более 250 ms, удаляя до 100 истёкших записей за проход; при наличии backlog очистка индексных записей постепенная. Пустые hash и sorted set удаляются самим Redis.

## Протокол WebSocket

Текущие media команды — `media.join`, `media.offer` (SDP и publications),
`media.ready`, `media.ice`, `media.unpublish`, `media.leave`. Binding отправителя
сервер получает из WS session. `media.joined`, `media.answer`, `media.tracks`,
`media.renegotiate`, `media.policy` и состояния возвращаются через тот же канал.
Полный контракт — [media-sfu.md](media-sfu.md).

Также доставляются `participant.*`, сообщения/реакции, `recording.*` и `caption.*`.
Потеря события записи восстанавливается чтением recording API, чата — историей,
субтитров — сохранёнными финальными репликами; Pub/Sub не выполняет replay.

### Envelope

Browser subprotocol: `go-recorder.v1`. Native клиент может использовать Bearer header без subprotocol, но JSON version всегда равен 1.

```json
{
  "version": 1,
  "id": "7156348b-df27-49f6-a9b5-cc7e61e17e04",
  "type": "media.join",
  "conferenceId": "053c3098-a4ac-4438-a087-659c767e19f7",
  "timestamp": "2026-10-21T12:00:00Z",
  "data": {}
}
```

`id` сообщения должен быть UUID; `timestamp` обязателен. Сервер формирует
свои `id`/`timestamp`, а identity отправителя получает из аутентифицированной
сессии. Ответы связываются с запросом через `replyTo`; входящий `replyTo`
и подмена binding отправителя запрещены.

| Событие | Направление и данные |
| --- | --- |
| `conference.state` | Server → client: connectionId, participantId, status конференции, participants |
| `participant.connected` | Server → conference: актуальный snapshot после подключения |
| `participant.disconnected` | Server → conference: актуальный snapshot после удаления подключения |
| `ack` | Server → sender: публикация signaling принята брокером, replyTo |
| `error` | Server → sender: безопасный code и replyTo |

Каждый элемент participants содержит обычный ParticipantView и поля online, connections, connectionIds. Online требует joined membership и хотя бы одного живого routing lease. Presence event несёт snapshot текущего состояния, а не накопительный delta; поздний disconnect старой вкладки не делает переподключённого участника offline.

SFU media путь проверяет SDP, publications, кодеки и ICE перед Pion.
Неизвестные поля, несовпадающая conferenceId и нарушение membership отклоняются.
Совместимый relay `webrtc.offer/answer/ice` также остаётся в API, но текущий UI
использует `media.*`. Relay проверяет активность и общую конференцию sender/target;
его SDP не используется для обхода проверок SFU.

`WS_MAX_CONNECTIONS=1000` по умолчанию ограничивает физические conference WS
одного API (0 отключает этот cap). Это не число участников SFU.
Остальные defaults: входящий frame 270336 байт, SDP 128 KiB, ICE 4 KiB, исходящий data 1 MiB; 20 сообщений в секунду с burst 40. Control frames тоже учитываются в token bucket. Outbound queue содержит максимум 64 сообщения. Переполнение закрывает slow client и запускает обычную очистку; hub не ждёт свободной очереди. Oversized исходящий snapshot закрывает подключение, а не выдаёт неполный roster.

Причины закрытия включают `client_closed`, `presence_timeout`, `server_shutdown`, `authentication_expired`, `authentication_revoked`, `authentication_unavailable`, `membership_closed`, `rate_limited`, `slow_client`, `broker_unavailable` и `outbound_too_large`. Сервер отправляет нативный ping каждую секунду; после пяти секунд без подтверждённого pong физическая сессия отключается. Срок присутствия в Redis также равен пяти секундам и обновляется только подтверждённым pong, а не произвольными сообщениями приложения. Закрытие браузера удаляет присутствие при закрытии сокета, не ожидая тайм-аута. Очистка присутствия выполняется до ожидания отправки закрывающего кадра. При аварии процесса API истёкшие аренды дополнительно удаляются фоновым обработчиком с периодом не более 250 мс; доставка события зависит от доступности хранилища и сети.

Отключение относится к конкретному `connectionId`: если у участника остаётся другая действующая вкладка или устройство, он остаётся онлайн. Сохранённое членство и история сообщений не удаляются при потере связи; после нового подключения приходит актуальный снимок присутствия.

Дедлайн аренды сохраняется как абсолютная серверная отметка `время приёма pong + TTL`: задержка обработки не добавляет ещё пять секунд. Системные часы API и Redis должны быть синхронизированы. Для защиты от положительного сдвига часов API срок дополнительно ограничен `текущее время Redis + TTL`; уже истёкшая аренда не продлевается.

## Endpoints и конфигурация

- `POST /api/v1/conferences/:id/ws-ticket`: Bearer JWT, joined membership, открытая конференция. Ответ 201 `{ticket, expiresAt}`, Cache-Control no-store. Билет привязан к conference,
сессии авторизации и при личном звонке `clientId`, одноразовый через GETDEL;
срок по умолчанию не превышает 45 секунд и остаток JWT при выдаче.
- `GET /api/v1/conferences/:id/ws?ticket=...`: browser upgrade. JWT в URL не принимается. Можно вместо ticket передать Bearer header в native клиенте. Одновременное использование обоих способов отклоняется.
- `GET /api/v1/webrtc/config`: Bearer JWT, ответ с `iceServers`, `iceTransportPolicy` и при TURN `expiresAt`, Cache-Control no-store.

Created и active конференции открыты для realtime, finished и cancelled закрыты. Наличие membership со статусом left недостаточно: сначала нужен REST join. Конференция и права проверяются до 101; после upgrade проводится повторная проверка на race с finish/leave.

Origin по умолчанию должен иметь тот же host и порт, что WS request. HTTPS reverse proxy сохраняет исходный Host, включая порт. `WS_ALLOWED_ORIGINS` позволяет задать точные HTTP(S) origins через запятую. Отсутствующий Origin допустим для native клиента, но аутентификация всё равно обязательна. Cookies не используются.

Все параметры перечислены в `.env.example`. Сумма `WS_PING_INTERVAL + WS_PONG_TIMEOUT` не должна превышать пяти секунд; `WS_SESSION_TTL` должен быть не меньше этой суммы и не больше пяти секунд. Значения по умолчанию — `1s`, `4s`, `5s`. Несогласованные значения останавливают запуск при валидации конфигурации. Срок билета в конфигурации — 30–60 секунд. ICE-серверы задаются через `WEBRTC_ICE_SERVERS_JSON`, например:

```dotenv
WEBRTC_ICE_SERVERS_JSON=[{"urls":["stun:stun.example.com:3478"]}]
```

По умолчанию массив пуст: нет неявного обращения к стороннему STUN. TURN username/credential можно передать через env, не в frontend source; это credentials для браузера, поэтому они видны аутентифицированному клиенту. Для текущего Coturn используйте `TURN_URLS` и серверный `TURN_SHARED_SECRET`:
код выдаёт временные HMAC credentials, TTL по умолчанию 10 минут; общий secret
браузеру не выдаётся. `TURN_FORCE_RELAY=true` задаёт relay policy для новых соединений.

Локальная сборка:

```bash
docker compose build api frontend
docker compose up -d --no-deps api frontend
docker compose exec -T https nginx -t
docker compose exec -T https nginx -s reload
```

Не требуется перезапуск PostgreSQL, Redis, RabbitMQ, S3-хранилища или recorder-worker. UI: http://localhost:5173; HTTPS: https://localhost:18482. После join откройте конференцию в двух браузерах/вкладках и проверьте presence
и реальное медиа SFU. Для production используйте полный набор overlays своей поставки и процедуру drain.

## Безопасность

JWT не помещается в WebSocket URL. В Redis сохраняется SHA256 имени билета; его случайное значение имеет 256 бит энтропии. Билет одноразовый, scoped и short lived. Для аккаунта с `sid` WebSocket проверяет серверную сессию перед upgrade и при каждом pong/команде; истечение исходного JWT не меняет соединение и медиапривязку. Отзыв закрывает сокет. Гостевые и старые JWT без `sid` сохраняют исходный expiry. Перед upgrade и при работе проверяются joined membership и состояние конференции.

Backend logs содержат структурированные conference_id, participant_id, user_id, connection_id, event_type, но не JWT, SDP, ICE credentials или пароль. HTTP logs используют route template без query string. Recovery не печатает request dump. Nginx access logs исключают query, а WS location подавляет credential-bearing upstream error dumps. Клиентский журнал показывает только названия событий.

Короткий JWT остаётся в sessionStorage текущей вкладки. Секрет постоянной сессии хранится в HttpOnly cookie и обновляет JWT через `/auth/refresh`; logout отзывает сессию вместе с JWT. Подробности: [PERSISTENT_AUTH.md](PERSISTENT_AUTH.md). XSS остаётся существенным риском, поэтому CSP и отказ от недоверенного HTML важны. Для production нужны TLS, закрытая сеть Redis, настройка реальных trusted proxies и отдельные лимиты инфраструктуры.

## Команды записи и события

API сохраняет recording row и `recording_outbox` транзакционно. Dispatcher
отправляет `record.start`/`record.stop` через RabbitMQ с confirms. Recorder получает
RTP через отдельный SFU HTTP NDJSON egress, а события `recording.starting`,
`started`, `stopping`, `processing`, `ready`, `failed`, `deleted` публикуются через
Redis для conference WS. ACK команды/публикации не означает готовность файла.
Продуктовые задания живут отдельно в PostgreSQL `background_jobs`.

## Отказ Redis и границы доставки

Redis Pub/Sub не durable: подтверждение публикации не означает получение клиентом.
Conference Hub при сбое подписки закрывает текущие sockets `broker_unavailable`,
прекращает приём до восстановления и автоматически переподписывается. Backoff
растёт от 100 ms до 2 s, одна попытка ограничена 5 s; после подтверждённой подписки
Hub снова принимает подключения. Штатное восстановление не требует рестарта API.
Клиент reconnect получает текущий snapshot; сохранённую историю читает через REST.

Низкоприоритетные события чата/реакций проходят отдельную bounded очередь;
переполнение может пропустить event, но не удаляет сохранённое сообщение. Медленный
socket закрывается, а не блокирует весь Hub. Состояния presence не равны бизнес-
членству, а live media ownership имеет собственные TTL в Redis.

Lua storage рассчитан на обычный Redis, не Redis Cluster. Полные snapshots и
глобальный conference bus имеют явные лимиты размера; это не обещание неограниченного
масштаба. Metrics/health описаны в
[ARCHITECTURE.md](ARCHITECTURE.md). Планирование нагрузки —
[CAPACITY_BASELINE.md](CAPACITY_BASELINE.md).

## Account WebSocket личных сообщений

Дополнительно к conference WS есть независимый `POST /api/v1/ws-ticket` →
`GET /api/v1/ws?ticket=…`. Он разрешён зарегистрированным аккаунтам, подписан
на персональный Redis channel и передаёт `message.*`, `conversation.*`,
`user.presence`, `call.updated`, а также существующие notification events. Presence учитывает
каждую физическую вкладку с TTL и временем Redis. Конференционное presence,
signaling и Pion остаются в conference WS. Контракт билета, событий, cleanup
и постоянной сессии: [PERSONAL_MESSAGES.md](PERSONAL_MESSAGES.md).
