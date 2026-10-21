# API и протоколы текущего выпуска

Актуализировано: **2026-10-21**. Это точка входа в документацию REST,
WebSocket, медиа и диагностики. Исполняемые маршруты и handlers — первичный
источник контракта; документы ниже описывают текущую реализацию.

Полный [каталог API](API_REFERENCE.md) содержит **177 серверных контрактов**:
149 маршрутов API-сервера (148 под `/api/v1` и `GET /health`) и 28 маршрутов
внутренних worker, operations и profiling. Общие operations считаются один раз,
хотя регистрируются на нескольких сервисах. [Postman-коллекция](postman/meetspace/MeetSpace.postman_collection.json)
содержит **178 запросов**: те же контракты плюс статический frontend `/version.json`.
Каталог включает доступ, запрос, ответ, ошибки и исходный файл для каждого пути.

## Быстрый старт REST

В [README](../README.md#api-meetspace) находится краткий каталог всех 148
маршрутов `/api/v1`, а не только API записи. Ниже — последовательности запросов
для основных функций. Примеры предназначены для локального стенда с уже
созданными аккаунтами; они **не выполнялись на рабочем сервере** при подготовке
документации. Запросы создания, отправки, закрепления и удаления меняют данные.

### 1. Войти и проверить профиль

Укажите адрес своего стенда. Для локального frontend это
`http://localhost:5173/api/v1`, для прямого локального API —
`http://localhost:8085/api/v1`. В production используйте HTTPS origin своего
приложения с суффиксом `/api/v1`.

```bash
API=http://localhost:5173/api/v1

curl --fail-with-body -X POST "$API/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"email":"you@example.com","password":"<your-password>"}'

ACCESS_TOKEN='<accessToken из ответа входа>'
curl --fail-with-body "$API/auth/me" \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

Вход возвращает `accessToken`, а не `token`. Регистрация
`POST /auth/register` создаёт пользователя, но не заменяет вход. Постоянные
браузерные сессии используют HttpOnly cookies и специальные auth-операции;
для refresh/switch/logout нужно сохранять и передавать cookie выбранного
аккаунта. В CLI без Origin auth-запросы разрешены; если Origin указан, он должен
точно совпадать с `PUBLIC_FRONTEND_URL`. Не публикуйте пароли, JWT и cookies.

### 2. Создать встречу и войти в неё

```bash
curl --fail-with-body -X POST "$API/conferences" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"title":"Встреча команды","waitingRoomEnabled":false}'

CONFERENCE_ID='<item.id из ответа создания>'
curl --fail-with-body -X POST "$API/conferences/$CONFERENCE_ID/start" \
  -H "Authorization: Bearer $ACCESS_TOKEN"
curl --fail-with-body -X POST "$API/conferences/$CONFERENCE_ID/join" \
  -H "Authorization: Bearer $ACCESS_TOKEN"
curl --fail-with-body "$API/conferences/$CONFERENCE_ID/participants/me" \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

Для планирования передайте `scheduledAt` в RFC3339 с будущим временем и,
при необходимости, `plannedDurationMin`; изменение расписания выполняется
`PUT /conferences/:id/schedule`. Персональные списки и календарь используют
`GET /me/conferences?view=upcoming|active|past` и данные расписания встречи.
Отдельного REST-префикса `/calendar` у приложения нет.

Подключение к медиа не создаётся одним REST-запросом: после допуска получите
`POST /conferences/:id/ws-ticket`, откройте WebSocket и выполните протокол SFU.
При включённом зале ожидания сначала дождитесь допуска. Организатор завершает
встречу через `POST /conferences/:id/finish`; обычный участник выходит через
`POST /conferences/:id/leave`. См. [контракты встреч](auth-conferences-api.md)
и [SFU](media-sfu.md).

### 3. Найти собеседника, открыть диалог и отправить сообщение

```bash
curl --fail-with-body --get "$API/users" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  --data-urlencode 'search=Александр'

PEER_ID='<id другого зарегистрированного пользователя>'
curl --fail-with-body -X POST "$API/conversations/direct" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"userId\":\"$PEER_ID\"}"

CONVERSATION_ID='<item.id диалога>'
CLIENT_REQUEST_ID="$(python3 -c 'import uuid; print(uuid.uuid4())')"
curl --fail-with-body -X POST "$API/conversations/$CONVERSATION_ID/messages" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"clientRequestId\":\"$CLIENT_REQUEST_ID\",\"text\":\"Здравствуйте! Обсудим проект?\"}"

curl --fail-with-body "$API/conversations/$CONVERSATION_ID/messages?limit=50" \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

В примере Python 3 нужен только для генерации UUID; в своём клиенте используйте
его генератор UUID. Новый `clientRequestId` нужен для нового сообщения, но при
повторе **того же** запроса сохраняйте прежний ключ. Повтор с другим содержимым
вернёт `409`. Ответ на сообщение задаётся полем `replyTo`, вложения —
`attachmentIds`. Для редактирования используется `PATCH /messages/:messageId`
относительно выбранного диалога, для удаления — `DELETE` по тому же пути.

Группа создаётся через `POST /conversations/group` с `clientRequestId`, `name`,
необязательным `description` и `memberIds`. Сообщения группы используют те же
маршруты `conversations/:id/messages`. Роли, передача владения, аватар,
участники и выход описаны в [GROUP_CHATS.md](GROUP_CHATS.md).

### 4. Закрепить сообщение, найти текст и отметить прочтение

```bash
MESSAGE_ID='<item.id отправленного текстового сообщения>'
curl --fail-with-body -X PUT "$API/conversations/$CONVERSATION_ID/pins/$MESSAGE_ID" \
  -H "Authorization: Bearer $ACCESS_TOKEN"
curl --fail-with-body "$API/conversations/$CONVERSATION_ID/pins" \
  -H "Authorization: Bearer $ACCESS_TOKEN"
curl --fail-with-body --get "$API/conversations/$CONVERSATION_ID/messages/search" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  --data-urlencode 'q=проект' --data-urlencode 'limit=20'
curl --fail-with-body -X PUT "$API/conversations/$CONVERSATION_ID/chat/read" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"messageId\":\"$MESSAGE_ID\"}"
```

Закрепления диалога общие для его участников; важные сообщения конференционного
чата — личные. Открепление выполняется `DELETE /conversations/:id/pins/:messageId`.
Чат встречи использует `conferences/:id/messages`, а поиск, материалы и личные
важные — `conferences/:id/chat/*`. Это разные пространства, их ID и маршруты
нельзя смешивать. Подробности: [личные сообщения](PERSONAL_MESSAGES.md),
[поиск](personal-chat-search.md), [чат конференции](collaboration.md).

### Остальные сценарии

| Сценарий | Последовательность / документация |
| --- | --- |
| Загрузка файла | `POST attachments/init` → `PUT attachments/:attachmentId/content` с байтами → `POST attachments/:attachmentId/finalize` → отправка сообщения с `attachmentIds`; [контракт](API_REFERENCE.md#api-group-4) |
| Личный звонок | `POST /calls` с `targetUserId`, `clientRequestId`, `clientId` → `accept/decline/cancel/end` и heartbeat; медиа — через WebSocket/WebRTC; [протокол](direct-calls.md) |
| Папки | `/folders`, `/folder-items`, `/folders/:id/items/:kind/:itemId`; [контракт](PERSONAL_FOLDERS.md) |
| Уведомления | `/notifications`, `/:notificationId/read`, `/notifications/events` (SSE с Bearer); настройки и устройства — `/notifications/preferences` и `/notifications/devices` |
| Календарные провайдеры | `/integrations/calendars`, OAuth connect/callback и `/conferences/:id/calendar`; [настройка провайдеров](content-integrations.md) |
| Запись встречи | `/conferences/:id/recordings` → `/:recordingId/stop` → чтение состояния и файлов; [контракт](conference-recording.md) |
| Текст и итоги встречи | `transcript`, `transcript/segments`, `summary` относительно записи; `captions` и `analytics` относительно конференции; сначала проверьте `/capabilities` |

Пути в этой таблице указаны относительно `/api/v1`; `attachments/*` дополнительно
относится к выбранному `/conferences/:id` или `/conversations/:id`.
Служебные API worker/metrics/drain не заменяют пользовательские endpoints и не
должны вызываться через публичный proxy. Не запускайте всю коллекцию Postman
пакетно на рабочем окружении: она содержит изменяющие и удаляющие запросы.

## Контракты по областям

| Область | Документ | Исполняемые маршруты / источник |
| --- | --- | --- |
| Аккаунты, JWT, встречи, приглашения | [auth-conferences-api.md](auth-conferences-api.md) | [platform_routes.go](../internal/transport/http/platform_routes.go) |
| Присутствие, WS tickets, конверт событий и reconnect | [realtime.md](realtime.md) | [WebSocket transport](../internal/transport/websocket), [realtime config](../internal/config/realtime.go) |
| Личные сообщения, поиск собеседников, account WS | [PERSONAL_MESSAGES.md](PERSONAL_MESSAGES.md) | [personal routes](../internal/transport/http/personal_routes.go), [account WS](../internal/transport/websocket/user_handler.go) |
| Группы, папки, поиск сообщений и личные звонки | [GROUP_CHATS.md](GROUP_CHATS.md), [PERSONAL_FOLDERS.md](PERSONAL_FOLDERS.md), [personal-chat-search.md](personal-chat-search.md), [звонки](direct-calls.md) | [personal routes](../internal/transport/http/personal_routes.go), [folder routes](../internal/transport/http/folder_routes.go), [call routes](../internal/transport/http/call_routes.go) |
| SFU, `media.*`, ICE/TURN, внутреннее управление | [media-sfu.md](media-sfu.md) | [mediaworker transport](../internal/transport/mediaworker), [TURN config](../internal/config/turn.go) |
| Запись, модерация, экран, recorder egress | [conference-recording.md](conference-recording.md) | [recording routes](../internal/transport/http/conference_recording_routes.go), [record models](../internal/domain/records/models.go) |
| Зал ожидания, расписание, чат, вложения, реакции, история, уведомления | [collaboration.md](collaboration.md) | [HTTP routes](../internal/transport/http), [chat routes и лимиты](../internal/transport/http/chat_routes.go) |
| Email/push/calendar, STT, ИИ, поиск | [content-integrations.md](content-integrations.md) | [integration routes](../internal/transport/http/integration_routes.go), [content routes](../internal/transport/http/content_routes.go) |
| Live captions, аналитика, переиндексация | Этот индекс и прикладные типы | [captions routes](../internal/transport/http/captions_routes.go), [captions types](../internal/domain/captions/captions.go), [intelligence config](../internal/config/meeting_intelligence.go) |
| Возможности клиента и admin | [FRONTEND_GUIDE.md](FRONTEND_GUIDE.md), [admin.md](operations/admin.md) | [platform status routes](../internal/transport/http/platform_status_routes.go) |

Продуктовые REST-маршруты начинаются с `/api/v1`. JSON использует camelCase.
Вход возвращает **`accessToken`**, `tokenType`, `expiresIn` и `user`; дальнейшие
запросы используют `Authorization: Bearer …`. Пароль — 8–128 Unicode-символов.
Аккаунты сохраняют вход через серверную сессию и HttpOnly cookie. Короткий JWT
обновляется через `/auth/refresh` с выбранным `userId`; `/auth/logout` отзывает
выбранную сессию, сохраняя другие аккаунты браузера. Поддерживается до пяти
подключённых аккаунтов; cookie-операции проверяют точный Origin и JSON.
Гостевой вход по публичной ссылке выдаёт отдельный JWT одной встречи.
Контракты и ограничения: [постоянная авторизация](PERSISTENT_AUTH.md).
Права на материалы проверяются по текущему членству и допуску, а не по наличию UUID.

WebSocket: клиент получает одноразовый билет для разрешённой конференции и
открывает `/api/v1/conferences/{id}/ws`; небраузерный клиент может использовать
Bearer-заголовок. Нельзя журналировать query с билетом. При reconnect клиент
получает свежий снимок, поскольку Redis Pub/Sub не обеспечивает историю доставки.
Личные сообщения получают отдельный account ticket через `POST /api/v1/ws-ticket`
и `/api/v1/ws?ticket=…`; постоянный JWT не передаётся в URL.
Уведомления используют авторизованный fetch-SSE `/api/v1/notifications/events`;
account WS также подписан на общий персональный канал, но frontend оставляет
обработку уведомлений в SSE, чтобы не создавать второй toast.

Для записи используются защищённые `/api/v1/conferences/{id}/recordings*`.
Прежние публичные `/api/v1/records*`, `/api/records*`, `/debug/records/*` и
`/debug/webrtc-smoke` не зарегистрированы и возвращают `404`.
Закрытие API не удаляет старые объекты из хранилища и не отзывает уже выданные
подписанные ссылки: они действуют до истечения срока.

## Пути истории, поиска и уведомлений

- Список встреч: `GET /api/v1/me/conferences?view=upcoming|active|past`; история
  одной закрытой встречи: `GET /api/v1/conferences/:id/history`.
- Текст сообщений direct/group: `GET /api/v1/conversations/:id/messages/search`;
  конференционный чат: `GET /api/v1/conferences/:id/chat/messages/search`.
  Оба используют `q`, `before`, `limit`; это отдельные контракты от поиска
  материалов `/api/v1/search`, который зависит от серверных возможностей.
- Уведомления: `GET /api/v1/notifications`,
  `POST /api/v1/notifications/:notificationId/read`,
  `GET /api/v1/notifications/events`. Список и read возвращают JSON без `status`;
  SSE требует Bearer в заголовке и запрещает query с токеном.
- Старые страницы браузера `/history/*`, `/search`, `/app/search` перенаправляют
  в актуальные разделы UI. Они не являются REST endpoints.

## Возможности, запись и дополнительные результаты

`GET /api/v1/capabilities` требует JWT и возвращает
`{status, capabilities, buildVersion}`. Флаги: `liveCaptions`, `transcription`,
`aiSummary`, `semanticSearch`, `meetingAnalytics`; `recordingModes` сейчас
содержит `composite`, `audio_only`, `individual_tracks`, `screen_focus`.
Семантический поиск дополнительно требует доступного pgvector. Этот ответ не
предоставляет права пользователю и не доказывает готовность каждого провайдера.
Клиент также обрабатывает текущие статусы и ошибки отдельных операций.

Запись создаётся `POST /api/v1/conferences/{id}/recordings` с выбранным `mode`;
запустить её может любой joined/admitted участник с постоянным аккаунтом. Гостям
(`guest_conference_id`) запуск и остановка запрещены. Остановить запись может
инициатор (`requested_by`) или owner. Остановка асинхронная.
DTO может содержать `starting`, `recording`, `degraded`, `stopping`, `processing`,
`ready`, `partial_ready`, `failed`, `cancelled`; внутренние `finalizing` и
`uploading` отображаются как `processing`. Наличие статуса в модели не означает,
что каждый режим проходит через него. Доступные файлы
различаются по режиму; нельзя требовать MP4 и preview от аудиозаписи.
Подписанные URL выдаются после проверки доступа и имеют ограниченный срок.

`DELETE /api/v1/conferences/{id}/recordings/{recordingId}` удаляет завершённую
запись (`ready`, `partial_ready`, `failed`, `cancelled`). Действие доступно
организатору встречи или инициатору записи с постоянным аккаунтом и действующим
правом читать историю встречи. Гости и исключённые участники получают `403`.
Активная запись или ещё действующая аренда recorder-worker дают `409`.
Удаление выполняется после подтверждения в меню записи. Сервер удаляет файлы,
превью и все версии объектов в пределах UUID записи; затем удаляет ссылки на
артефакты и выставляет `deleted_at`. Сама строка остаётся для внутренних связей,
но чтение и списки больше её не возвращают. Ошибка хранилища позволяет повторить
запрос; успешный повтор возвращает `200 {"status":"success"}`. После фиксации
изменения участникам отправляется событие `recording.deleted`.

| Endpoint | Назначение |
| --- | --- |
| `GET/PUT /api/v1/conferences/{id}/captions` | Состояние и управление живым распознаванием согласно `canManage` |
| `GET /api/v1/conferences/{id}/captions/segments` | Сохранённые финальные реплики с курсором |
| `GET /api/v1/conferences/{id}/analytics` | Разрешённые агрегаты встречи |
| `POST /api/v1/conferences/{id}/recordings/{recordingId}/search/reindex` | Разрешённый повтор индексации |
| `GET /api/v1/admin/summary` | Агрегаты для `users.is_admin`; обычному пользователю — `403` |

Внутренние протоколы SFU/recorder используют отдельные service secrets.
Не публикуйте их через пользовательский API или proxy. Описание передачи
содержимого провайдерам: [DATA_FLOWS.md](DATA_FLOWS.md).

## Версия и эксплуатационные endpoints

| Endpoint | Доступ и ответ |
| --- | --- |
| `GET /version` | Публичные метки API: `{version,commit,buildTime}`; `Cache-Control: no-store` |
| `GET /version.json` | Публичные метки frontend-артефакта с теми же полями; ответ требует повторной проверки кеша |
| `GET /health/live` | Внутри сети сервиса; жив ли процесс |
| `GET /health/ready` | Внутри сети сервиса; готовность зависимостей и отсутствие drain |
| `GET /metrics` | Внутри сети, Bearer `METRICS_SECRET`; Prometheus |
| `GET/POST /operations/drain` | Внутри сети, Bearer `METRICS_SECRET`; чтение/включение drain, ответ `{draining,active,ready}` |

`dev`/`unknown` обозначают неполные локальные метки, а не валидированный релиз.
Frontend и API могут обновляться отдельно, поэтому проверка выпуска сверяет
обе версии с манифестом. Публичный production proxy запрещает health, metrics,
operations, internal и debug; эти проверки выполняются через закрытый доступ.
Снятие drain обратным HTTP-запросом не реализовано.

## Ошибки frontend

`POST /api/v1/client-errors` работает без JWT, чтобы учитывать отказы до входа.
Приём включается серверным `CLIENT_TELEMETRY_ENABLED`; отправка — флагом сборки
frontend. По умолчанию оба выключены. Только `Content-Type: application/json`,
один JSON-объект до 8192 байт, неизвестные поля запрещены.

```json
{
  "version": "v1.2.3",
  "route": "/meetings/:id",
  "code": "react_render_error",
  "browser": "firefox",
  "stack": [{"file": "index-abcdefgh.js", "line": 42, "column": 8}]
}
```

`code`: `uncaught_error`, `unhandled_rejection`, `react_render_error`.
`browser`: `chromium`, `firefox`, `safari`, `edge`, `other`.
`route` — фиксированный шаблон известного маршрута либо `unknown`, никогда не
реальный UUID/код приглашения. `version` соответствует
`^[A-Za-z0-9][A-Za-z0-9._+-]{0,79}$`. `stack` — максимум пять кадров: basename
собственного JS-файла до 160 байт, целые `line`/`column` от 1 до 10 000 000.
Имена функций, исходные сообщения, внешние URL, query/hash, SDP и пользовательский
текст не принимаются. Полный allowlist —
[telemetry/handler.go](../internal/app/telemetry/handler.go).

Успех — `202` без тела. Отклонение: `400` неверная схема, `413` слишком большое
тело, `415` неверный Content-Type; выключенный приём — `404` после middleware.
Redis limiter допускает 30 запросов/IP/минуту и 300 запросов/минуту суммарно;
при превышении — `429`, при недоступности limiter запрос не проходит.
Клиент дополнительно ограничивает поток пятью событиями/минуту и двадцатью за
страницу, не отправляет credentials/Referer и не следует перенаправлениям.
Счётчик: `recorder_client_errors_total{code,browser,route}`.

## Текущие ограничения по умолчанию

Это значения реализации, а не гарантии производительности. Операторские env
могут менять часть из них; актуальный файл окружения выпуска хранится вне Git.

| Область | Значение по умолчанию | Источник |
| --- | --- | --- |
| Login / register | 10 / 5 запросов за минуту на IP | [config.go](../internal/config/config.go), `RATE_LIMIT_AUTH_*` |
| Входящие WS сообщения | 20/секунду, burst 40, до 64 КиБ на сообщение | [realtime.go](../internal/config/realtime.go), `WS_*` |
| Билет WS | До 45 секунд, одноразовый | [realtime.go](../internal/config/realtime.go) |
| SFU | 10 peers на комнату, 100 комнат, 1 screen sharer | [media.go](../internal/config/media.go), `MEDIA_MAX_*` |
| Запись | 2 активные, 1 FFmpeg-конвейер, 4 часа, 10 ГиБ | [composite.go](../internal/config/composite.go), `RECORDING_*` |
| Чат: чтение / отправка / правка | 120 / 60 / 30 в минуту на user+conference | [chat_routes.go](../internal/transport/http/chat_routes.go) |
| Вложения | 10 МиБ/файл, 5/сообщение; upload init/content по 10/минуту | [collaboration.md](collaboration.md), [chat routes](../internal/transport/http/chat_routes.go) |
| Поиск | 30 запросов/минуту на пользователя | [content_routes.go](../internal/transport/http/content_routes.go) |
| OAuth start / callback | 5 / 10 запросов/минуту на пользователя | [integration_routes.go](../internal/transport/http/integration_routes.go) |
| STT записи | 2 часа, 1 ГиБ исходного видео, 256 МиБ извлечённого аудио | [product.go](../internal/config/product.go) |
| Live STT | 2 конференции, 10 дорожек на процесс, до 2 часов | [meeting_intelligence.go](../internal/config/meeting_intelligence.go) |

Обработка более длинной записи не следует автоматически из увеличения одного
лимита recorder: бюджеты STT, temporary storage и провайдера отдельные.
Не игнорируйте `429` и `Retry-After`; повторять изменяющие операции следует
только с предусмотренной контрактом идемпотентностью.

Запросы [коллекции MeetSpace](postman/meetspace/MeetSpace.postman_collection.json)
выполняйте по отдельности: среди них есть операции удаления, drain и длительные
потоки. Служебные запросы требуют закрытого адреса и отдельной авторизации.
Порядок приёмки конкретного артефакта — [RELEASE_PROCESS.md](RELEASE_PROCESS.md).
