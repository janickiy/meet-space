# SFU: серверная передача аудио, видео и экрана

Актуализировано 2026-10-21 по коду `6e577b6cced4e34d1c031353fbb9055909055185`.

## Возможности

`cmd/media-worker` запускает SFU на Pion. Браузеры публикуют микрофон, камеру,
видео и звук демонстрации экрана. SFU пересылает кодированные RTP-пакеты подписчикам
и выдаёт отдельные потоки для записи. API хранит права, членство, сигнализацию и историю;
Go record-worker обрабатывает запись в отдельном процессе.
SFU используется как обычными конференциями, так и двухсторонними личными звонками.
Запись для `kind=direct_call` запрещена на уровне приложения.

Реализованы trickle ICE, ограниченные очереди, RTCP/PLI, сериализация переговоров,
повторное включение устройств без пересоздания PeerConnection, демонстрация экрана,
политики модерации и учёт владельца комнаты в Redis. Выбор источника и разрешения браузера
обрабатываются интерфейсом; источник `camera`/`video/screen` — заявленная клиентом семантика,
а не серверная аппаратная аттестация.

## Архитектура медиа

```text
Browser → same-origin WebSocket → API media controller → protected worker HTTP
Browser ← same-origin WebSocket ← API Hub ← Redis targeted events ← media-worker
Browser ⇄ encrypted WebRTC RTP/RTCP ⇄ media-worker SFU ⇄ other browsers
                               ├─ HTTP NDJSON → recorder → spool/FFmpeg/S3
```

SFU не зависит от PostgreSQL, RabbitMQ, S3-хранилища или FFmpeg. Его медиадвижок получает
проверенные данные участника через интерфейс; подписи, активная сессия и право обслуживать комнату проверяются
при обращении к обработчику. Создание и изменение конференций (CRUD) и прикладные права остаются вне SFU.
Room и MediaPeer не используют сессии приёма медиаданных прежнего обработчика записи.

Глобальные мьютексы защищают только короткие операции с реестром. Вызовы Pion и Redis, а также ожидание
закрытия не выполняются под глобальной блокировкой. Жизненный цикл peer исключает добавление
горутин после начала Wait; регистрация публикации атомарна относительно закрытия (close).
У каждого подписчика отдельная RTP-очередь ограниченного размера. Медленный подписчик не блокирует
остальных. Расширения MID/RID/TWCC отправителя не копируются в RTP-транспорт другого участника.

## Сигнализация

Сохраняется оболочка сообщения v1, UUID сообщения, conferenceId, timestamp и replyTo.
Команды управления медиасессией не принимают от клиента participant/session/connection IDs или targetConnectionId.
API определяет привязку отправителя (binding) по аутентифицированной WebSocket-сессии.

| Событие | Направление и данные |
| --- | --- |
| `media.join` | Браузер → API, `{}` |
| `media.joined` | Сервер → браузер: mediaPeerId, workerId, maxPeers, videoCapture, iceServers, tracks |
| `media.offer` | Браузер → обработчик через API: mediaPeerId, negotiationId, sdp, publications |
| `media.answer` | Обработчик → браузер через API: те же IDs и sdp, replyTo offer |
| `media.ready` | Браузер → обработчик через API: mediaPeerId, negotiationId после применения answer |
| `media.ice` | Оба направления: mediaPeerId, candidate object либо null |
| `media.renegotiate` | Обработчик → браузер: mediaPeerId, возрастающая revision |
| `media.tracks` | Обработчик → подписчик: mediaPeerId, revision, актуальный массив tracks |
| `media.published` | Обработчик → отправитель: mediaPeerId, track |
| `media.unpublished` | Обработчик → отправитель: mediaPeerId, trackId |
| `media.unpublish` | Браузер → обработчик: mediaPeerId, собственный trackId |
| `media.state` | Обработчик → браузер: mediaPeerId, состояние PeerConnection |
| `media.leave` | Браузер → обработчик: mediaPeerId; `{}` отменяет незавершённый вход текущей сессии |
| `media.left` | Сервер → браузер: mediaPeerId |

Только браузер создаёт offer, в том числе при обновлении подписок. Он заранее
резервирует по `2 × (maxPeers−1)` слотов приёма каждого kind для двух аудио-
и двух видеоисточников удалённого peer. Обработчик последовательно выполняет операции SDP и
AddTrack/RemoveTrack. Полная обработка offer, включая освобождение устаревших ресурсов (retirement), также последовательна, чтобы старые
изменения не опережали новые. Изменения подписок объединяются в уведомления о новой revision.
Браузер ждёт соответствующий answer и объединяет последующие revisions.
Сервер не создаёт offer, поэтому конфликт одновременных предложений (glare) не требует случайного выбора инициатора.

Передача подписчику приостанавливается на время offer. После setRemoteDescription
браузер подтверждает `media.ready` с тем же negotiationId; только тогда worker
возобновляет RTP и запрашивает ключевой кадр. Устаревшее подтверждение отклоняется.
Подтверждение включает только подписки из соответствующего answer; новые публикации
ждут следующего offer/answer/ready и не могут начать RTP по старому подтверждению.
Это также предотвращает раннюю передачу необъявленного SSRC в Pion. Таймаут подтверждения закрывает endpoint.

Удалённые ICE-кандидаты до получения SDP сохраняются в буфере: максимум 64 сообщения до offer и 128 на PeerConnection
за жизненный цикл; null означает end-of-candidates и тоже учитывается в лимите.
Некорректные SDP и ICE, неподдерживаемые кодеки и переполнение приводят к ошибкам с безопасными кодами.
Медиаконтракт ограничен SDP 128 KiB и ICE 4 KiB, даже если legacy WS config разрешает больше.
ICE-кандидаты внутри SDP проверяются до Pion и ограничены 128 на offer;
для совместимости полного SDP допускаются компоненты 1/2, trickle принимает только 1.
Прежний `webrtc.*` relay остаётся в протоколе совместимости; текущий frontend SFU его не использует.

<a id="внутренний-transport-api-и-worker"></a>

## Внутренний обмен между API и медиасервером

Используется HTTP POST `/internal/media/{join,offer,ready,ice,unpublish,leave}`. Это короткие
сигнальные операции, не канал RTP. RabbitMQ не используется для ICE или SDP.
Запросы содержат UUID корреляции одновременно в теле и X-Request-ID, привязку отправителя
и маршрут медиасервера. Bearer `MEDIA_INTERNAL_SECRET` обязателен. Перенаправления запрещены,
размеры запросов и ответов ограничены, тайм-аут по умолчанию — 5 секунд. Исходные ошибки внутреннего сервиса не раскрываются.
Повтор join возвращает тот же endpoint. Повтор последнего offer с теми же negotiationId
и SDP использует сохранённый answer; изменение SDP с тем же ID отклоняется. Leave идемпотентен.

Внутренний билет медиасессии (media ticket) подписывается HS256, имеет issuer `go-recorder-api`, audience
`go-recorder-media`, purpose `media`, UUID jti, conference/participant/session/connection/user
binding, workerId и ownership leaseId. Admission exp — по умолчанию до 45 секунд и не позже `authorizationExpiresAt`.
Для гостевого/legacy JWT этот срок ограничен исходным JWT. У постоянного аккаунта
с `sid` conference WS после проверки сессии использует длительный authorization
horizon и регулярно проверяет отзыв; истечение короткого access JWT само по себе
не обрывает существующее медиа. Подробнее: [PERSISTENT_AUTH.md](PERSISTENT_AUTH.md).
Билет не выдаётся браузеру и не помещается в URL. Worker повторно проверяет живую
Redis route перед командами. Отзыв/закрытие сессии и истечение применимого
срока авторизации закрывают медиасессию.

<a id="модели-room-mediapeer-и-track"></a>

## Модели Room, MediaPeer и Track

Room привязан к conferenceId и содержит подключения (peers), опубликованные дорожки и подписчиков
исходящих потоков (egress). Комната удаляется, когда не остаётся peers и outputs. Подписчик
записи может сохранять комнату после выхода последнего peer.
MediaPeer содержит собственный UUID, binding, PeerConnection, publications и subscriptions.
Несколько вкладок одного membership — разные session/connection/mediaPeer IDs.
Собственные треки не возвращаются тому же participantId, включая его другие вкладки.

Track имеет UUID, streamId publisher mediaPeerId, publisher mediaPeerId, participantId,
typed kind и source: `audio/microphone`, `video/camera`, `video/screen`,
`audio/screen`. Камера и экран имеют разные streamId; `media.offer.publications`
связывает SDP MID с source и поколением browser capture. Browser receiver track ID может быть другим после
preallocated transceiver; frontend использует streamId + kind для привязки и server UUID
для lifecycle. Снимок метаданных удаляет исчезнувшие потоки, даже если браузер не прислал событие ended.

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
двумя аккаунтами и владеющими медиа вкладками проверкой прав личного звонка; SFU работает
с теми же общими ресурсами. Второй экран сверх лимита получает `screen_sharing_conflict`.
Интерфейс откатывает offer и останавливает отклонённый захват, сохраняя остальное медиа.

Worker передаёт `videoCapture` в `media.joined`; браузер применяет целевые параметры к локальному
видео до первого offer. Это не серверная проверка разрешения и битрейта произвольного
клиента. Simulcast/SVC и H.264 в SFU не поддерживаются; конфигурационные лимиты не
подтверждают доступную производительность и не гарантируют защиту от любой перегрузки сетевого канала.

При скрытии входящего видео клиент добавляет SDP `a=x-meet-receive-video:0`.
SFU убирает видеоподписки, сохраняя входящее аудио и собственные публикации.
Снятие настройки возобновляет видео через negotiation/ready без повторного join;
подробности — [AUDIO_VIDEO_SETTINGS.md](AUDIO_VIDEO_SETTINGS.md).

<a id="ice-stun-turn-и-адреса"></a>

## ICE, STUN, TURN и сетевые адреса

Настройки SFU не зависят от порта 50000 обработчика записи. Базовый Compose использует UDP mux
50010 и необязательный TCP mux 50010 медиадвижка. Внутренняя сигнализация доступна на порту 8091 только в сети Docker.
`MEDIA_NAT_IPS` должен содержать IP, достижимый браузером. Для Docker Desktop/Firefox
укажите достижимый LAN IP хоста, а не IP контейнера/127.0.0.1;
конкретный адрес зависит от текущей сети. На сервере нужен корректный public/NAT IP
и проброс тех же портов без изменения номера. Для других компьютеров требуется HTTPS
origin с доверенным сертификатом, иначе getUserMedia будет недоступен.

`MEDIA_ICE_SERVERS_JSON` задаёт массив STUN/TURN; при отсутствии используется
`WEBRTC_ICE_SERVERS_JSON`. Явный `[]` отключает fallback. Неявного обращения к публичному STUN нет.
Coturn входит в Compose. `TURN_URLS` и `TURN_SHARED_SECRET` задаются оператором;
`TURN_CREDENTIAL_TTL=10m` по умолчанию (1m–1h), `TURN_FORCE_RELAY=false`.
API формирует временные учётные данные HMAC, передаваемые браузеру вместе с
`iceTransportPolicy` и `expiresAt`; общий секрет TURN не передаётся.

Базовый Compose публикует SFU 50010 UDP/TCP; значение по умолчанию `MEDIA_TCP_PORT=0`
может быть переопределён Compose. Диапазон ретрансляции Coturn в production примере —
49160–49259/UDP. Защищённый установщик создаёт `config/turn.yml` с сетевым режимом хоста (host network),
отменяя обычные настройки сетей и проброса портов. Настройте межсетевой экран и NAT в соответствии
с фактической дополнительной конфигурацией Compose и проверьте ретрансляцию реальным звонком; доступность порта
сама по себе этого не доказывает.

<a id="маршрутизация-и-ownership"></a>

## Маршрутизация и владение комнатой

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

Структурированные журналы содержат только IDs, тип события, kind/source и состояния.
Pion использует безопасный logger для транспортов, interceptors и UDP/TCP mux:
сырые сообщения, строки форматирования и аргументы отбрасываются без их преобразования.
Сохраняются только ограниченное имя компонента и уровень события; raw SDP, candidates,
JWT/tickets и ICE/TURN credentials не журналируются.

## Основные файлы

- [`internal/app/media_worker.go`](../internal/app/media_worker.go), `cmd/media-worker` — запуск и инициализация;
- [`internal/infrastructure/sfu`](../internal/infrastructure/sfu) — медиадвижок, жизненный цикл подключений и дорожек, SDP, политики и исходящие потоки;
- [`internal/transport/mediaworker`](../internal/transport/mediaworker) — защищённые команды и поток NDJSON;
- [`internal/usecase/media`](../internal/usecase/media) — контроллер API и HTTP-клиент;
- [`internal/domain/media`](../internal/domain/media) — контракты binding/route/publications/egress;
- [`internal/infrastructure/security/media_ticket.go`](../internal/infrastructure/security/media_ticket.go) и
  [`internal/infrastructure/redis/media.go`](../internal/infrastructure/redis/media.go) — билеты и владение комнатой;
- [`frontend/src/media.ts`](../frontend/src/media.ts), `frontend/src/useMedia.ts` — жизненный цикл медиасессии в браузере;
- [`internal/config/media.go`](../internal/config/media.go), [`internal/config/turn.go`](../internal/config/turn.go) — ограничения и ICE.

## Docker и запуск

В production задайте отдельные случайные MEDIA_TICKET_SECRET и MEDIA_INTERNAL_SECRET
не короче 32 байт. В local/dev пустые значения безопасно выводятся разными HMAC purposes
из существующего JWT_SECRET; общего секрета, зашитого в код, нет. Не сохраняйте секреты в Git.

```bash
docker compose up -d --build media-worker api frontend https
docker compose ps media-worker api frontend
```

UI: http://localhost:5173 или https://localhost:18482. Internal media HTTP/metrics
не опубликованы на host/gateway. Health endpoint возвращает только готовность,
`/metrics` и `/operations/drain` требуют `METRICS_SECRET`, media commands —
`MEDIA_INTERNAL_SECRET`. Новый Docker-образ работает без прав root и не содержит FFmpeg.
PostgreSQL/RabbitMQ/S3-хранилища/record-worker не требуется пересоздавать для обновления SFU.
После обновления проверяйте health/readiness и разрешения браузера.
В production используйте
полный набор Compose overlays/образов своей поставки и drain перед заменой SFU.

Базовый Compose публикует TCP mux; для UDP-only или диапазона используйте один override
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
декодирование, подключение после начала встречи, демонстрацию экрана, отключение и повторное включение
устройств, переподключение и завершение комнаты. Проверьте освобождение дорожек и
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
проверенные файлы в S3. RabbitMQ доставляет команды start/stop, но не медиаданные.
Полный жизненный цикл, четыре режима, временные права на задачи и сроки хранения: [conference-recording.md](conference-recording.md).

Legacy recorder ingest имеет отдельные PeerConnection и порты; текущий UI его
не использует. Производственный record-worker отключает legacy mux-порты и принимает
NDJSON от SFU. Наличие legacy кода не расширяет набор кодеков SFU.

## Границы отказоустойчивости и эксплуатации

- Redis Pub/Sub не хранит события. При потере подписки conference Hub закрывает
  текущие WS и сам восстанавливает подписку с backoff 100 ms–2 s; после этого
  клиенты переподключаются и получают снимок состояния. Перезапуск API для этого не обязателен.
- Потеря Redis ownership останавливает комнаты SFU. Автоматическое создание нового
  соединения не означает бесшовную миграцию существующего PeerConnection.
- Redis Lua рассчитан на обычный Redis, не на Redis Cluster. У каждого SFU должны
  быть уникальные `MEDIA_WORKER_ID`, endpoint и корректные media port mappings.
- Internal HTTP защищён секретом и сетью. Перед передачей за пределы доверенной
  сети требуются соответствующие TLS/mTLS и firewall; production env не должен
  использовать локально выведенные HMAC-секреты.
- Для совместной нагрузки конференций, экранов, записи рассчитывайте
  ресурсы целевого хоста: [лимиты и планирование](CAPACITY_BASELINE.md).
- Реальные Chrome/Firefox/Safari, NAT, TURN/TURNS и звук демонстрации экрана зависят от
  устройств и браузеров; их проверяют на целевой сети и клиентах перед выпуском.
