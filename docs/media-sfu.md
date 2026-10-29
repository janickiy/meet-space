# SFU: серверная передача аудио, видео и экрана

Актуализировано 2026-10-21 по коду `6e577b6cced4e34d1c031353fbb9055909055185`.

## Возможности

`cmd/media-worker` запускает SFU на Pion. Браузеры публикуют микрофон, камеру,
видео и аудио экрана; SFU пересылает encoded RTP подписчикам и отдельным
потоков записи. API хранит права, членство, сигнализацию и историю;
Go record-worker обрабатывает запись в отдельном процессе.
SFU используется как обычными конференциями, так и двухсторонними личными звонками.
Запись для `kind=direct_call` запрещены на уровне приложения.

Реализованы trickle ICE, ограниченные очереди, RTCP/PLI, сериализация переговоров,
повторное включение устройств без пересоздания PeerConnection, screen sharing,
модераторские политики и Redis ownership. Выбор источника и разрешения браузера
обрабатывает UI; источник `camera`/`video/screen` — заявленная клиентом семантика,
а не серверная аппаратная аттестация.

## Архитектура медиа

```text
Browser → same-origin WebSocket → API media controller → protected worker HTTP
Browser ← same-origin WebSocket ← API Hub ← Redis targeted events ← media-worker
Browser ⇄ encrypted WebRTC RTP/RTCP ⇄ media-worker SFU ⇄ other browsers
                               ├─ HTTP NDJSON → recorder → spool/FFmpeg/S3
```

SFU не зависит от PostgreSQL, RabbitMQ, S3-хранилища или FFmpeg. Его engine принимает
проверенную identity через интерфейс; подписи, активная сессия и ownership проверяются
на границе worker. Данные conference CRUD и business permissions не перенесены в SFU.
Room и MediaPeer не переиспользуют recorder ingest sessions.

Глобальные mutex защищают только короткие операции с registry. Pion/Redis и ожидание
закрытия не выполняются под глобальным lock. Peer lifecycle исключает добавление
goroutines после начала Wait; регистрация публикации атомарна относительно close.
У каждого subscriber отдельная bounded RTP queue; медленный subscriber не блокирует
остальных. MID/RID/TWCC extensions publisher не копируются в чужой RTP transport.

## Сигнализация

Сохраняется envelope v1, UUID сообщения, conferenceId, timestamp и replyTo.
Media-команды не содержат client participant/session/connection IDs или targetConnectionId.
API получает binding из аутентифицированной WebSocket-сессии.

| Событие | Направление и данные |
| --- | --- |
| `media.join` | Browser → API, `{}` |
| `media.joined` | Server → browser: mediaPeerId, workerId, maxPeers, videoCapture, iceServers, tracks |
| `media.offer` | Browser → worker через API: mediaPeerId, negotiationId, sdp, publications |
| `media.answer` | Worker → browser через API: те же IDs и sdp, replyTo offer |
| `media.ready` | Browser → worker через API: mediaPeerId, negotiationId после применения answer |
| `media.ice` | Оба направления: mediaPeerId, candidate object либо null |
| `media.renegotiate` | Worker → browser: mediaPeerId, monotonic revision |
| `media.tracks` | Worker → subscriber: mediaPeerId, revision, актуальный массив tracks |
| `media.published` | Worker → publisher: mediaPeerId, track |
| `media.unpublished` | Worker → publisher: mediaPeerId, trackId |
| `media.unpublish` | Browser → worker: mediaPeerId, собственный trackId |
| `media.state` | Worker → browser: mediaPeerId, состояние PeerConnection |
| `media.leave` | Browser → worker: mediaPeerId; `{}` отменяет pending join текущей сессии |
| `media.left` | Server → browser: mediaPeerId |

Браузер — единственный инициатор offer, включая обновления подписок. Он заранее
создаёт по `2 × (maxPeers−1)` receive slots каждого kind для двух аудио-
и двух видеоисточников удалённого peer. Worker сериализует SDP и
AddTrack/RemoveTrack; полный offer с retirement также сериализован, чтобы старые
изменения не обгоняли новые. Subscription mutations вызывают coalesced revision
notifications. Браузер ждёт соответствующий answer и объединяет следующие revisions.
Server offer отсутствует, поэтому glare не требует случайного выбора владельца.

Передача subscriber приостанавливается на время offer. После setRemoteDescription
браузер подтверждает `media.ready` с тем же negotiationId; только тогда worker
возобновляет RTP и запрашивает ключевой кадр. Устаревшее подтверждение отклоняется.
Подтверждение включает только подписки из соответствующего answer; новые публикации
ждут следующего offer/answer/ready и не могут начать RTP по старому подтверждению.
Это также предотвращает раннюю передачу undeclared SSRC в Pion. Таймаут подтверждения закрывает endpoint.

Remote ICE до SDP буферизуется: максимум 64 сообщения до offer и 128 на PeerConnection
за lifecycle; null означает end-of-candidates и тоже учитывается в лимите.
Malformed SDP, unsupported codecs, ICE и переполнение дают безопасные coded errors.
Медиаконтракт ограничен SDP 128 KiB и ICE 4 KiB, даже если legacy WS config разрешает больше.
Встроенные SDP candidates валидируются до Pion и ограничены 128 на offer;
для совместимости полного SDP допускаются компоненты 1/2, trickle принимает только 1.
Прежний `webrtc.*` relay остаётся в протоколе совместимости; текущий frontend SFU его не использует.

## Внутренний transport API и worker

Используется HTTP POST `/internal/media/{join,offer,ready,ice,unpublish,leave}`. Это короткие
сигнальные операции, не канал RTP. RabbitMQ не используется для ICE или SDP.
Запросы содержат UUID correlation ID одновременно в body и X-Request-ID, binding
и worker route. Bearer `MEDIA_INTERNAL_SECRET` обязателен. Redirects запрещены,
body/response ограничены, default timeout 5 секунд; upstream raw errors не раскрываются.
Duplicate join возвращает тот же endpoint; duplicate last offer с тем же negotiationId
и SDP использует cached answer; изменение SDP под тем же ID отклоняется. Leave идемпотентен.

Media ticket подписывается HS256, имеет issuer `go-recorder-api`, audience
`go-recorder-media`, purpose `media`, UUID jti, conference/participant/session/connection/user
binding, workerId и ownership leaseId. Admission exp — по умолчанию до 45 секунд и не позже `authorizationExpiresAt`.
Для гостевого/legacy JWT этот срок ограничен исходным JWT. У постоянного аккаунта
с `sid` conference WS после проверки сессии использует длительный authorization
horizon и регулярно проверяет отзыв; истечение короткого access JWT само по себе
не обрывает существующее медиа. Подробнее: [PERSISTENT_AUTH.md](PERSISTENT_AUTH.md).
Ticket не выдаётся браузеру и не помещается в URL. Worker повторно проверяет живую
Redis route перед командами. Отзыв/закрытие сессии и истечение применимого
срока авторизации закрывают media.

## Модели Room MediaPeer и Track

Room привязан к conferenceId и содержит peers, published tracks и egress
подписчиков. Комната удаляется, когда не осталось peers и outputs; recording
subscriber может удерживать комнату после выхода последнего peer.
MediaPeer содержит собственный UUID, binding, PeerConnection, publications и subscriptions.
Несколько вкладок одного membership — разные session/connection/mediaPeer IDs.
Собственные треки не возвращаются тому же participantId, включая его другие вкладки.

Track имеет UUID, streamId publisher mediaPeerId, publisher mediaPeerId, participantId,
typed kind и source: `audio/microphone`, `video/camera`, `video/screen`,
`audio/screen`. Камера и экран имеют разные streamId; `media.offer.publications`
связывает SDP MID с source и поколением browser capture. Browser receiver track ID может быть другим после
preallocated transceiver; frontend использует streamId + kind для привязки и server UUID
для lifecycle. Metadata snapshot удаляет исчезнувшие streams, даже если браузер не прислал ended.

## Кодеки и ограничения

SFU регистрирует только Opus 48 kHz stereo и VP8 90 kHz. Пакеты не декодируются и не
перекодируются; Pion переписывает SSRC/PT отдельного sender. RTCP sender читается,
PLI/FIR subscribers превращаются в ограниченные запросы ключевого кадра publisher.
Ключевой кадр запрашивается при подписке и после negotiation.

Текущие значения [config/media.go](../internal/config/media.go):

| Переменная | По умолчанию | Допустимый диапазон |
| --- | --- | --- |
| `MEDIA_MAX_PEERS` | 10 на комнату | 2–32 |
| `MEDIA_MAX_ROOMS` | 100 на SFU | 1–10000 |
| `MEDIA_MAX_PUBLISHED_TRACKS` | 4 на peer | 1–4 |
| `MEDIA_MAX_AUDIO_TRACKS` / `MEDIA_MAX_VIDEO_TRACKS` | 2 / 2 на peer | 1–2 каждое |
| `MEDIA_MAX_SCREEN_SHARERS` | 1 на комнату | 1–4 |
| `MEDIA_EGRESS_QUEUE_SIZE` | 2048 | 128–8192 |
| `MEDIA_VIDEO_MAX_WIDTH/HEIGHT/FPS` | 1280 / 720 / 30 | 160–1280 / 90–720 / 1–30 |

Вкладки расходуют лимит MediaPeers отдельно. Личные звонки дополнительно ограничены
двумя аккаунтами и владеющими медиа вкладками через call authorization; SFU работает
с теми же общими ресурсами. Второй экран сверх лимита получает `screen_sharing_conflict`.
UI откатывает offer и останавливает отклонённый захват, сохраняя остальное медиа.

Worker передаёт `videoCapture` в `media.joined`; браузер применяет target к локальному
видео до первого offer. Это не серверная проверка разрешения/bitrate произвольного
клиента. Simulcast/SVC и H.264 в SFU не поддерживаются; конфигурационные лимиты не
подтверждают capacity или защиту от любого bandwidth flooding.

При скрытии входящего видео клиент добавляет SDP `a=x-meet-receive-video:0`.
SFU убирает видеоподписки, сохраняя входящее аудио и собственные публикации.
Снятие настройки возобновляет видео через negotiation/ready без повторного join;
подробности — [AUDIO_VIDEO_SETTINGS.md](AUDIO_VIDEO_SETTINGS.md).

## ICE STUN TURN и адреса

Настройки SFU независимы от recorder порта 50000. Base Compose использует UDP mux
50010 и optional engine TCP mux 50010. Internal signaling — 8091, только Docker network.
`MEDIA_NAT_IPS` должен содержать IP, достижимый браузером. Для Docker Desktop/Firefox
укажите достижимый LAN IP хоста, а не IP контейнера/127.0.0.1;
конкретный адрес зависит от текущей сети. На сервере нужен корректный public/NAT IP
и проброс тех же портов без изменения номера. Для других компьютеров требуется HTTPS
origin с доверенным сертификатом, иначе getUserMedia будет недоступен.

`MEDIA_ICE_SERVERS_JSON` задаёт массив STUN/TURN; при отсутствии используется
`WEBRTC_ICE_SERVERS_JSON`. Явный `[]` отключает fallback. Неявного public STUN нет.
Coturn входит в Compose. `TURN_URLS` и `TURN_SHARED_SECRET` задаются оператором;
`TURN_CREDENTIAL_TTL=10m` по умолчанию (1m–1h), `TURN_FORCE_RELAY=false`.
API формирует временные HMAC credentials, передаваемые браузеру вместе с
`iceTransportPolicy` и `expiresAt`; общий TURN secret не передаётся.

Base Compose публикует SFU 50010 UDP/TCP; дефолт конфигурации `MEDIA_TCP_PORT=0`
может быть переопределён Compose. Relay диапазон Coturn в production примере —
49160–49259/UDP. Hardened installer создаёт `config/turn.yml` с host network,
сбрасывая обычные network/port mappings. Настройте firewall/NAT в соответствии
с фактическим overlay и подтвердите релей реальным звонком; доступность порта
сама по себе этого не доказывает.

## Маршрутизация и ownership

Redis namespace SFU по умолчанию `go-recorder:media:v1`, отдельно от realtime.
Worker heartbeat регистрирует identity/endpoint с TTL; API выбирает healthy worker
и атомарно закрепляет conference. Существующий живой owner сохраняется независимо
от API instance. Mapping содержит lease UUID; renew/release сравнивают worker и lease.
Старый owner не может продлить или удалить новую lease. Каждый replica должен иметь
уникальный MEDIA_WORKER_ID и собственный endpoint; общий ID двух живых процессов запрещён.

Worker регулярно проверяет активную WS route и применимый authorization horizon. Hub disconnect
дополнительно вызывает короткий internal leave. Потерянный HTTP cleanup исправляется sweep.
Ownership loss, Redis outage и истечение локального срока владения останавливают rooms;
новый join требует повторной проверки и действующей lease. Это минимальный routing,
не распределённый scheduler и не бесшовная миграция активной room.

Структурированные logs содержат только IDs, тип события, kind/source и состояния.
Pion использует безопасный logger для транспортов, interceptors и UDP/TCP mux:
сырые сообщения, строки форматирования и аргументы отбрасываются без их преобразования.
Сохраняются только ограниченное имя компонента и уровень события; raw SDP, candidates,
JWT/tickets и ICE/TURN credentials не журналируются.

## Основные файлы

- [`internal/app/media_worker.go`](../internal/app/media_worker.go), `cmd/media-worker` — bootstrap;
- [`internal/infrastructure/sfu`](../internal/infrastructure/sfu) — engine, peer/track lifecycle, SDP, policy, egress;
- [`internal/transport/mediaworker`](../internal/transport/mediaworker) — защищённые команды и поток NDJSON;
- [`internal/usecase/media`](../internal/usecase/media) — API controller и HTTP client;
- [`internal/domain/media`](../internal/domain/media) — контракты binding/route/publications/egress;
- [`internal/infrastructure/security/media_ticket.go`](../internal/infrastructure/security/media_ticket.go) и
  [`internal/infrastructure/redis/media.go`](../internal/infrastructure/redis/media.go) — билеты и ownership;
- [`frontend/src/media.ts`](../frontend/src/media.ts), `frontend/src/useMedia.ts` — браузерный lifecycle;
- [`internal/config/media.go`](../internal/config/media.go), [`internal/config/turn.go`](../internal/config/turn.go) — limits и ICE.

## Docker и запуск

В production задайте отдельные случайные MEDIA_TICKET_SECRET и MEDIA_INTERNAL_SECRET
не короче 32 байт. В local/dev пустые значения безопасно выводятся разными HMAC purposes
из существующего JWT_SECRET; hardcoded общего секрета нет. Не сохраняйте секреты в Git.

```bash
docker compose up -d --build media-worker api frontend https
docker compose ps media-worker api frontend
```

UI: http://localhost:5173 или https://localhost:18482. Internal media HTTP/metrics
не опубликованы на host/gateway. Health endpoint возвращает только готовность,
`/metrics` и `/operations/drain` требуют `METRICS_SECRET`, media commands —
`MEDIA_INTERNAL_SECRET`. Новый Docker image non-root и без FFmpeg.
PostgreSQL/RabbitMQ/S3-хранилища/record-worker не требуется пересоздавать для обновления SFU.
После обновления проверяйте health/readiness и разрешения браузера.
В production используйте
полный набор Compose overlays/образов своей поставки и drain перед заменой SFU.

Base Compose публикует TCP mux; для UDP-only или диапазона используйте один override
(нужен Compose с поддержкой `!override`):

```bash
docker compose -f docker-compose.yml -f docker-compose.media-udp.yml up -d --build
docker compose -f docker-compose.yml -f docker-compose.media-range.yml up -d --build
```

Range override использует MEDIA_RANGE_MIN/MAX, по умолчанию 50100–50200, отключает mux
и TCP. Не пытайтесь только выставить MEDIA_TCP_PORT=0 в base Compose: нужно убрать
TCP mapping через override. Firewall должен разрешать выбранный UDP порт/диапазон.

## Проверка подключения

После обновления проверьте два/три браузера с реальным SFU: получение RTP и
декодирование, поздний join, демонстрацию экрана, отключение и повторное включение
устройств, reconnect и завершение комнаты. Проверьте освобождение tracks и
PeerConnections, затем отдельно forced relay через TURN/TURNS. Разрешения ОС,
выбор системного аудио и качество физических устройств проверяются в браузере.

## Поток записи

`POST /internal/media/egress` принимает `EgressRequest`: UUID конференции, записи
и запроса, текущую route/lease и длительность сегмента.
`Authorization: Bearer MEDIA_INTERNAL_SECRET` и совпадающий `X-Request-ID`
обязательны. `application/x-ndjson` содержит `hello`, `track`, `rtp`, `track.end`,
`ping`, `error`; поле `rtp` сериализует байты в base64. `capturedAt` — Unix ns;
sequence монотонно увеличивается. Сервер не запускает FFmpeg и не пишет spool.

Каждая подписка имеет отдельную ограниченную очередь. Переполнение recording
output закрывает его и приводит к ошибке записи, не блокируя RTP участников.


Go record-worker после `hello` переходит в `recording` (первый RTP может прийти
позднее), пишет локальные IVF/Ogg/manifests, запускает compositor/FFmpeg и публикует
проверенные файлы в S3. RabbitMQ доставляет команды start/stop, но не медиабайты.
Полный lifecycle, четыре режима, leases и retention: [conference-recording.md](conference-recording.md).

Legacy recorder ingest имеет отдельные PeerConnection и порты; текущий UI его
не использует. Производственный record-worker отключает legacy mux-порты и принимает
NDJSON от SFU. Наличие legacy кода не расширяет набор кодеков SFU.

## Границы отказоустойчивости и эксплуатации

- Redis Pub/Sub не хранит события. При потере подписки conference Hub закрывает
  текущие WS и сам восстанавливает подписку с backoff 100 ms–2 s; после этого
  клиенты переподключаются и читают snapshot. Перезапуск API для этого не обязателен.
- Потеря Redis ownership останавливает комнаты SFU. Автоматическое создание нового
  соединения не означает бесшовную миграцию существующего PeerConnection.
- Redis Lua рассчитан на обычный Redis, не на Redis Cluster. У каждого SFU должны
  быть уникальные `MEDIA_WORKER_ID`, endpoint и корректные media port mappings.
- Internal HTTP защищён секретом и сетью. Перед передачей за пределы доверенной
  сети требуются соответствующие TLS/mTLS и firewall; production env не должен
  использовать локально выведенные HMAC-секреты.
- Для совместной нагрузки конференций, экранов, записи рассчитывайте
  ресурсы целевого хоста: [лимиты и планирование](CAPACITY_BASELINE.md).
- Реальные Chrome/Firefox/Safari, NAT, TURN/TURNS и screen audio зависят от
  устройств и браузеров; их проверяют на целевой сети и клиентах перед выпуском.
