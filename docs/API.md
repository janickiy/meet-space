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
