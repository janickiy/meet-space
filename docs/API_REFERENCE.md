# Полный справочник API MeetSpace

Актуализировано **21 октября 2026 года** по коду коммита `6e577b6`.
Справочник содержит **177 серверных регистраций**: 148 маршрутов `/api/v1`,
`GET /health` API и 28 общих/внутренних/диагностических регистраций.
Общие маршруты Runtime указаны один раз, хотя используются разными процессами.
Статический frontend `/version.json` в это число не входит.
Контракты сверены с регистрациями и обработчиками; запросы к рабочему серверу
при актуализации не выполнялись. Права проверяются на сервере для конкретного
ресурса. `*` означает, что Go HTTP handler не ограничивает метод; обычно
используется GET. Символ `*` после имени поля означает обязательное поле.
Этот документ дополняет [обзор API](API.md), [инструкции запуска](../README.md)
и [Postman-коллекцию](postman/meetspace/MeetSpace.postman_collection.json).
Краткий [каталог всех 148 маршрутов `/api/v1`](../README.md#каталог-маршрутов)
доступен прямо в README; [быстрый старт](API.md#быстрый-старт-rest) показывает
последовательности запросов для встреч, переписок и закреплённых сообщений.
Служебные адреса SFU/recorder/metrics не предназначены для публичного доступа.
Подписанные ссылки и токены берите из ответов доступного вам окружения.

Перечень ошибок у маршрута описывает его основные проверки. Общие middleware
и зависимости также могут вернуть `401`, `403`, `413`, `415`, `429`, `500`, `503`
или `504`; учитывайте `Retry-After`, если он присутствует. JSON основного API
использует camelCase; служебные HTTP handlers могут отвечать обычным текстом.
## Разделы

- [Личные звонки](#api-group-1) — 8
- [Субтитры и аналитика](#api-group-2) — 5
- [Чаты конференций](#api-group-3) — 12
- [Сообщения и вложения](#api-group-4) — 20
- [Записи конференций](#api-group-5) — 5
- [Контент и общий поиск](#api-group-6) — 6
- [Реакции и TURN](#api-group-7) — 1
- [Личные папки](#api-group-8) — 10
- [Гостевой доступ](#api-group-9) — 1
- [Интеграции](#api-group-10) — 12
- [Приглашения](#api-group-11) — 2
- [Уведомления](#api-group-12) — 3
- [Приватные ресурсы групп](#api-group-13) — 4
- [Личные и групповые диалоги](#api-group-14) — 21
- [Поиск в диалогах](#api-group-15) — 2
- [Аккаунты и конференции](#api-group-16) — 28
- [Возможности платформы](#api-group-17) — 2
- [Телеметрия](#api-group-18) — 1
- [API health](#api-group-19) — 1
- [WebSocket HTTP](#api-group-20) — 5
- [Media worker](#api-group-21) — 11
- [Operations](#api-group-22) — 6
- [Recorder worker](#api-group-23) — 4
- [Profiling (отдельный listener)](#api-group-24) — 7

<a id="api-group-1"></a>

## Личные звонки

### `POST /api/v1/calls`

Позвонить зарегистрированному пользователю.

- **Доступ:** Аккаунт

- **Запрос:** JSON targetUserId*,clientRequestId*,clientId* — канонические UUID; target не равен себе

- **Ответ:** 201 новый/200 повтор: status,item: Call (ringing)

- **Ошибки:** 422 поля; 409 пользователь занят/ключ использован иначе; 404 адресат

Источники: [internal/transport/http/call_routes.go:35](../internal/transport/http/call_routes.go#L35), [internal/app/calls/handler.go:31](../internal/app/calls/handler.go#L31).

### `GET /api/v1/calls/active`

Текущий звонок пользователя.

- **Доступ:** Аккаунт

- **Запрос:** Без тела/query

- **Ответ:** 200: status,item: Call либо null

- **Ошибки:** 401/403

Источники: [internal/transport/http/call_routes.go:36](../internal/transport/http/call_routes.go#L36), [internal/app/calls/handler.go:79](../internal/app/calls/handler.go#L79).

### `GET /api/v1/calls/:id`

Состояние звонка.

- **Доступ:** Аккаунт; caller или callee

- **Запрос:** id — канонический UUID

- **Ответ:** 200: status,item: Call

- **Ошибки:** 404 нет доступного звонка; 422 UUID

Источники: [internal/transport/http/call_routes.go:37](../internal/transport/http/call_routes.go#L37), [internal/app/calls/handler.go:61](../internal/app/calls/handler.go#L61).

### `POST /api/v1/calls/:id/accept`

Принять входящий звонок.

- **Доступ:** Аккаунт; callee

- **Запрос:** id; JSON clientId* UUID выбранного клиента

- **Ответ:** 200: status,item: Call (accepted), conferenceId для медиасоединения

- **Ошибки:** 403 не callee; 409 уже не ringing/занят другим клиентом; 422

Источники: [internal/transport/http/call_routes.go:39](../internal/transport/http/call_routes.go#L39), [internal/app/calls/handler.go:95](../internal/app/calls/handler.go#L95).

### `POST /api/v1/calls/:id/decline`

Отклонить входящий звонок.

- **Доступ:** Аккаунт; callee

- **Запрос:** id; JSON {} либо {clientId}; тело JSON обязательно, clientId необязателен

- **Ответ:** 200: status,item: Call (declined)

- **Ошибки:** 403 не callee; 409 состояние; 422 id

Источники: [internal/transport/http/call_routes.go:39](../internal/transport/http/call_routes.go#L39), [internal/app/calls/handler.go:95](../internal/app/calls/handler.go#L95).

### `POST /api/v1/calls/:id/cancel`

Отменить исходящий звонок.

- **Доступ:** Аккаунт; caller и его clientId

- **Запрос:** id; JSON clientId* UUID

- **Ответ:** 200: status,item: Call (cancelled)

- **Ошибки:** 403 не владелец клиента; 409 состояние; 422

Источники: [internal/transport/http/call_routes.go:39](../internal/transport/http/call_routes.go#L39), [internal/app/calls/handler.go:95](../internal/app/calls/handler.go#L95).

### `POST /api/v1/calls/:id/end`

Завершить принятый звонок.

- **Доступ:** Аккаунт; caller/callee, владеющий медиаклиентом

- **Запрос:** id; JSON clientId* UUID

- **Ответ:** 200: status,item: Call (ended)

- **Ошибки:** 403 не владелец клиента; 409 состояние; 422

Источники: [internal/transport/http/call_routes.go:39](../internal/transport/http/call_routes.go#L39), [internal/app/calls/handler.go:95](../internal/app/calls/handler.go#L95).

### `POST /api/v1/calls/:id/heartbeat`

Продлить lease звонка.

- **Доступ:** Аккаунт; владелец медиаклиента принятого звонка

- **Запрос:** id; JSON clientId* UUID

- **Ответ:** 200: status,item: Call

- **Ошибки:** 409 не accepted/чужой clientId; 422

Источники: [internal/transport/http/call_routes.go:39](../internal/transport/http/call_routes.go#L39), [internal/app/calls/handler.go:95](../internal/app/calls/handler.go#L95).


<a id="api-group-2"></a>

## Субтитры и аналитика

### `GET /api/v1/conferences/:id/captions`

Состояние живых субтитров.

- **Доступ:** Bearer; доступ к истории

- **Запрос:** id

- **Ответ:** 200: status,item: available,enabled,canManage,status,language,generation,sessionId,canonicalRecordingId

- **Ошибки:** 403; 404

Источники: [internal/transport/http/captions_routes.go:27](../internal/transport/http/captions_routes.go#L27), [internal/app/captions/handler.go:119](../internal/app/captions/handler.go#L119).

### `PUT /api/v1/conferences/:id/captions`

Включить/выключить живые субтитры.

- **Доступ:** Bearer; вошедший owner/co_host

- **Запрос:** id; JSON enabled* bool; language=auto/ru/en (auto)

- **Ответ:** 200: status,item: CaptionState

- **Ошибки:** 422 язык/поле; 403 роль; 409 конференция не active; 503 функция отключена; 429

Источники: [internal/transport/http/captions_routes.go:28](../internal/transport/http/captions_routes.go#L28), [internal/app/captions/handler.go:141](../internal/app/captions/handler.go#L141).

### `GET /api/v1/conferences/:id/captions/segments`

Сохраненные финальные субтитры после курсора.

- **Доступ:** Bearer; доступ к истории

- **Запрос:** id; afterCursor ≥0 (0),limit 1–200 (100)

- **Ответ:** 200: status,items: Caption[],nextCursor,hasMore

- **Ошибки:** 422 cursor/limit; 403; 404

Источники: [internal/transport/http/captions_routes.go:29](../internal/transport/http/captions_routes.go#L29), [internal/app/captions/handler.go:178](../internal/app/captions/handler.go#L178).

### `GET /api/v1/conferences/:id/analytics`

Аналитика конференции.

- **Доступ:** Bearer; доступ к истории

- **Запрос:** id

- **Ответ:** 200: status,item: ConferenceAnalytics (включая participants,timeline,approximateSpeaking)

- **Ошибки:** 403; 404

Источники: [internal/transport/http/captions_routes.go:30](../internal/transport/http/captions_routes.go#L30), [internal/app/captions/handler.go:63](../internal/app/captions/handler.go#L63).

### `POST /api/v1/conferences/:id/recordings/:recordingId/search/reindex`

Поставить повторную индексацию готовой транскрипции.

- **Доступ:** Bearer; owner/co_host с доступом к истории

- **Запрос:** id,recordingId; тело не читается

- **Ответ:** 202: status=success

- **Ошибки:** 403; 409 нет ready transcript/прошло <5 минут; 503 embeddings/index недоступны; 404

Источники: [internal/transport/http/captions_routes.go:31](../internal/transport/http/captions_routes.go#L31), [internal/app/captions/handler.go:80](../internal/app/captions/handler.go#L80).


<a id="api-group-3"></a>

## Чаты конференций

### `GET /api/v1/conferences/:id/chat/info`

Название, описание и права конференционного чата.

- **Доступ:** Bearer; допущенный участник joined/left, не вышедший из чата

- **Запрос:** id

- **Ответ:** 200: status,item: conferenceId,title,description,inviteUrl,participantCount,notificationsEnabled,canEdit,canInvite

- **Ошибки:** 403; 404

Источники: [internal/transport/http/chat_actions_routes.go:49](../internal/transport/http/chat_actions_routes.go#L49), [internal/app/chat/actions_handler.go:43](../internal/app/chat/actions_handler.go#L43).

### `GET /api/v1/conferences/:id/chat/members`

Участники чата и presence.

- **Доступ:** Bearer; допущенный участник joined/left, не вышедший из чата

- **Запрос:** id; after cursor, limit 1–100 (50)

- **Ответ:** 200: status,items: MemberView[],nextCursor

- **Ошибки:** 403; 422 cursor/limit; 404

Источники: [internal/transport/http/chat_actions_routes.go:50](../internal/transport/http/chat_actions_routes.go#L50), [internal/app/chat/actions_handler.go:60](../internal/app/chat/actions_handler.go#L60).

### `PATCH /api/v1/conferences/:id/chat/info`

Изменить метаданные чата/конференции.

- **Доступ:** Bearer; owner с доступом к чату

- **Запрос:** id; JSON title 1–200 и/или description ≤1000; хотя бы одно поле

- **Ответ:** 200: status,item: ChatInfo

- **Ошибки:** 403 право canEdit; 422 поля; 404

Источники: [internal/transport/http/chat_actions_routes.go:51](../internal/transport/http/chat_actions_routes.go#L51), [internal/app/chat/actions_handler.go:86](../internal/app/chat/actions_handler.go#L86).

### `GET /api/v1/conferences/:id/chat/preferences`

Личная настройка уведомлений.

- **Доступ:** Bearer; допущенный участник joined/left, не вышедший из чата

- **Запрос:** id

- **Ответ:** 200: status,item: notificationsEnabled

- **Ошибки:** 403; 404

Источники: [internal/transport/http/chat_actions_routes.go:52](../internal/transport/http/chat_actions_routes.go#L52), [internal/app/chat/actions_handler.go:107](../internal/app/chat/actions_handler.go#L107).

### `PUT /api/v1/conferences/:id/chat/preferences`

Изменить личную настройку уведомлений.

- **Доступ:** Bearer; допущенный участник joined/left, не вышедший из чата

- **Запрос:** id; JSON notificationsEnabled* bool

- **Ответ:** 200: status,item: notificationsEnabled

- **Ошибки:** 422 поле отсутствует; 403; 404

Источники: [internal/transport/http/chat_actions_routes.go:53](../internal/transport/http/chat_actions_routes.go#L53), [internal/app/chat/actions_handler.go:124](../internal/app/chat/actions_handler.go#L124).

### `DELETE /api/v1/conferences/:id/chat/membership`

Выйти из чата конференции.

- **Доступ:** Bearer; допущенное собственное членство

- **Запрос:** id; без тела

- **Ответ:** 200: status=success

- **Ошибки:** 403; 404

Источники: [internal/transport/http/chat_actions_routes.go:54](../internal/transport/http/chat_actions_routes.go#L54), [internal/app/chat/actions_handler.go:152](../internal/app/chat/actions_handler.go#L152).

### `GET /api/v1/conferences/:id/chat/messages/search`

Поиск сообщений конференции.

- **Доступ:** Bearer; допущенный участник joined/left, не вышедший из чата

- **Запрос:** id; q* 2–200 символов; before; limit 1–50 (20)

- **Ответ:** 200: status,items: Message[],nextCursor

- **Ошибки:** 422 q/cursor/limit; 403; 404

Источники: [internal/transport/http/chat_actions_routes.go:55](../internal/transport/http/chat_actions_routes.go#L55), [internal/app/chat/actions_handler.go:168](../internal/app/chat/actions_handler.go#L168).

### `GET /api/v1/conferences/:id/chat/messages/:messageId/context`

Окружение найденного сообщения.

- **Доступ:** Bearer; допущенный участник joined/left, не вышедший из чата

- **Запрос:** id,messageId

- **Ответ:** 200: status,items,nextCursor,unreadCount,lastReadMessageId

- **Ошибки:** 403; 404

Источники: [internal/transport/http/chat_actions_routes.go:56](../internal/transport/http/chat_actions_routes.go#L56), [internal/app/chat/actions_handler.go:189](../internal/app/chat/actions_handler.go#L189).

### `GET /api/v1/conferences/:id/chat/materials`

Изображения, файлы или ссылки из сообщений.

- **Доступ:** Bearer; допущенный участник joined/left, не вышедший из чата

- **Запрос:** id; kind*=image/file/link; before; limit 1–50 (20)

- **Ответ:** 200: status,items: Material[],nextCursor

- **Ошибки:** 422 kind/cursor/limit; 403; 404

Источники: [internal/transport/http/chat_actions_routes.go:57](../internal/transport/http/chat_actions_routes.go#L57), [internal/app/chat/actions_handler.go:210](../internal/app/chat/actions_handler.go#L210).

### `GET /api/v1/conferences/:id/chat/pins`

Личные важные сообщения конференции.

- **Доступ:** Bearer; допущенный участник joined/left, не вышедший из чата

- **Запрос:** id; before; limit 1–50 (20)

- **Ответ:** 200: status,items: Message[],nextCursor

- **Ошибки:** 403; 422 cursor/limit; 404

Источники: [internal/transport/http/chat_actions_routes.go:58](../internal/transport/http/chat_actions_routes.go#L58), [internal/app/chat/actions_handler.go:231](../internal/app/chat/actions_handler.go#L231).

### `PUT /api/v1/conferences/:id/chat/pins/:messageId`

Добавить сообщение в личные важные.

- **Доступ:** Bearer; допущенный участник joined/left, не вышедший из чата

- **Запрос:** id,messageId; без тела

- **Ответ:** 200: status=success

- **Ошибки:** 403; 404 сообщение удалено/не найдено

Источники: [internal/transport/http/chat_actions_routes.go:59](../internal/transport/http/chat_actions_routes.go#L59), [internal/app/chat/actions_handler.go:255](../internal/app/chat/actions_handler.go#L255).

### `DELETE /api/v1/conferences/:id/chat/pins/:messageId`

Удалить сообщение из личных важных.

- **Доступ:** Bearer; допущенный участник joined/left, не вышедший из чата

- **Запрос:** id,messageId; без тела

- **Ответ:** 200: status=success; идемпотентно

- **Ошибки:** 403; 422 UUID

Источники: [internal/transport/http/chat_actions_routes.go:60](../internal/transport/http/chat_actions_routes.go#L60), [internal/app/chat/actions_handler.go:255](../internal/app/chat/actions_handler.go#L255).


<a id="api-group-4"></a>

## Сообщения и вложения

### `GET /api/v1/conferences/:id/messages`

История сообщений.

- **Доступ:** Bearer; допущенный участник joined/left, не вышедший из чата

- **Запрос:** id; query before cursor, limit 1–100 (50)

- **Ответ:** 200: status, items: Message[], nextCursor, unreadCount, lastReadMessageId

- **Ошибки:** 403 членство/выход из чата; 422 cursor/limit; 404

Источники: [internal/transport/http/chat_routes.go:69](../internal/transport/http/chat_routes.go#L69), [internal/app/chat/handler.go:57](../internal/app/chat/handler.go#L57).

### `GET /api/v1/conversations/:id/messages`

История сообщений.

- **Доступ:** Bearer аккаунта; действующий член нескрытого conversation

- **Запрос:** id; query before cursor, limit 1–100 (50)

- **Ответ:** 200: status, items: Message[], nextCursor, unreadCount, lastReadMessageId

- **Ошибки:** 403 членство/выход из чата; 422 cursor/limit; 404

Источники: [internal/transport/http/chat_routes.go:69](../internal/transport/http/chat_routes.go#L69), [internal/app/chat/handler.go:57](../internal/app/chat/handler.go#L57).

### `POST /api/v1/conferences/:id/messages`

Отправить сообщение с идемпотентностью.

- **Доступ:** Bearer; допущенный joined участник active конференции, не вышедший из чата

- **Запрос:** id; JSON clientRequestId* UUID, text ≤4000 символов, replyTo UUID, attachmentIds ≤5; нужен текст или вложение

- **Ответ:** 201 новый/200 повтор: status,item: Message

- **Ошибки:** 409 ключ использован с другим содержимым/чат read-only/вложение не готово; 422 поля; 403

Источники: [internal/transport/http/chat_routes.go:70](../internal/transport/http/chat_routes.go#L70), [internal/app/chat/handler.go:84](../internal/app/chat/handler.go#L84).

### `POST /api/v1/conversations/:id/messages`

Отправить сообщение с идемпотентностью.

- **Доступ:** Bearer аккаунта; действующий член нескрытого conversation

- **Запрос:** id; JSON clientRequestId* UUID, text ≤4000 символов, replyTo UUID, attachmentIds ≤5; нужен текст или вложение

- **Ответ:** 201 новый/200 повтор: status,item: Message

- **Ошибки:** 409 ключ использован с другим содержимым/чат read-only/вложение не готово; 422 поля; 403

Источники: [internal/transport/http/chat_routes.go:70](../internal/transport/http/chat_routes.go#L70), [internal/app/chat/handler.go:84](../internal/app/chat/handler.go#L84).

### `PATCH /api/v1/conferences/:id/messages/:messageId`

Изменить текст сообщения.

- **Доступ:** Bearer; допущенный joined участник active конференции, не вышедший из чата; только автор

- **Запрос:** id,messageId; JSON text ≤4000; пусто только при вложениях

- **Ответ:** 200: status,item: Message с новой version

- **Ошибки:** 403 чужое/служебное сообщение; 409 удалено/read-only; 422 пустой текст; 404

Источники: [internal/transport/http/chat_routes.go:71](../internal/transport/http/chat_routes.go#L71), [internal/app/chat/handler.go:109](../internal/app/chat/handler.go#L109).

### `PATCH /api/v1/conversations/:id/messages/:messageId`

Изменить текст сообщения.

- **Доступ:** Bearer аккаунта; действующий член нескрытого conversation; только автор

- **Запрос:** id,messageId; JSON text ≤4000; пусто только при вложениях

- **Ответ:** 200: status,item: Message с новой version

Системная история звонка (`call`) не редактируется и не удаляется, в том числе автором: `403`.

- **Ошибки:** 403 чужое/служебное сообщение; 409 удалено/read-only; 422 пустой текст; 404

Источники: [internal/transport/http/chat_routes.go:71](../internal/transport/http/chat_routes.go#L71), [internal/app/chat/handler.go:109](../internal/app/chat/handler.go#L109).

### `DELETE /api/v1/conferences/:id/messages/:messageId`

Мягко удалить сообщение.

- **Доступ:** Bearer; допущенный joined участник active конференции, не вышедший из чата; автор; в конференции также owner/co_host

- **Запрос:** id,messageId; без тела

- **Ответ:** 200: status,item: Message с deletedAt и пустым text

- **Ошибки:** 403 чужое/история звонка; 409 read-only; 404

Источники: [internal/transport/http/chat_routes.go:72](../internal/transport/http/chat_routes.go#L72), [internal/app/chat/handler.go:135](../internal/app/chat/handler.go#L135).

### `DELETE /api/v1/conversations/:id/messages/:messageId`

Мягко удалить сообщение.

- **Доступ:** Bearer аккаунта; действующий член conversation; только автор сообщения

- **Запрос:** id,messageId; без тела

- **Ответ:** 200: status,item: Message с deletedAt и пустым text

Системная история звонка (`call`) не редактируется и не удаляется, в том числе автором: `403`.

- **Ошибки:** 403 чужое/история звонка; 409 read-only; 404

Источники: [internal/transport/http/chat_routes.go:72](../internal/transport/http/chat_routes.go#L72), [internal/app/chat/handler.go:135](../internal/app/chat/handler.go#L135).

### `GET /api/v1/conferences/:id/chat/read`

Текущая граница прочтения.

- **Доступ:** Bearer; допущенный участник joined/left, не вышедший из чата

- **Запрос:** id

- **Ответ:** 200: status,item: lastReadMessageId,unreadCount

- **Ошибки:** 403; 404

Источники: [internal/transport/http/chat_routes.go:73](../internal/transport/http/chat_routes.go#L73), [internal/app/chat/handler.go:156](../internal/app/chat/handler.go#L156).

### `GET /api/v1/conversations/:id/chat/read`

Текущая граница прочтения.

- **Доступ:** Bearer аккаунта; действующий член нескрытого conversation

- **Запрос:** id

- **Ответ:** 200: status,item: lastReadMessageId,unreadCount

- **Ошибки:** 403; 404

Источники: [internal/transport/http/chat_routes.go:73](../internal/transport/http/chat_routes.go#L73), [internal/app/chat/handler.go:156](../internal/app/chat/handler.go#L156).

### `PUT /api/v1/conferences/:id/chat/read`

Монотонно отметить прочитанное.

- **Доступ:** Bearer; допущенный участник joined/left, не вышедший из чата

- **Запрос:** id; JSON messageId* UUID

- **Ответ:** 200: status,item: lastReadMessageId,unreadCount

- **Ошибки:** 422 UUID; 403; 404 сообщение вне доступной истории

Источники: [internal/transport/http/chat_routes.go:74](../internal/transport/http/chat_routes.go#L74), [internal/app/chat/handler.go:174](../internal/app/chat/handler.go#L174).

### `PUT /api/v1/conversations/:id/chat/read`

Монотонно отметить прочитанное.

- **Доступ:** Bearer аккаунта; действующий член нескрытого conversation

- **Запрос:** id; JSON messageId* UUID

- **Ответ:** 200: status,item: lastReadMessageId,unreadCount

- **Ошибки:** 422 UUID; 403; 404 сообщение вне доступной истории

Источники: [internal/transport/http/chat_routes.go:74](../internal/transport/http/chat_routes.go#L74), [internal/app/chat/handler.go:174](../internal/app/chat/handler.go#L174).

### `POST /api/v1/conferences/:id/attachments/init`

Создать резервирование вложения.

- **Доступ:** Bearer; допущенный joined участник active конференции, не вышедший из чата; владелец загрузки

- **Запрос:** id; JSON clientRequestId* UUID, filename* 1–180 символов без путей, size* 1–10485760; mimeType необязателен; изображения, документы и архивы из [списка поддерживаемых вложений](ATTACHMENTS.md)

- **Ответ:** 201 новый/200 повтор: status,item: Attachment,uploadUrl

- **Ошибки:** 422 метаданные/MIME/размер; 409 конфликт ключа/read-only; 403

Источники: [internal/transport/http/chat_routes.go:75](../internal/transport/http/chat_routes.go#L75), [internal/app/chat/handler.go:196](../internal/app/chat/handler.go#L196).

### `POST /api/v1/conversations/:id/attachments/init`

Создать резервирование вложения.

- **Доступ:** Bearer аккаунта; действующий член нескрытого conversation; владелец загрузки

- **Запрос:** id; JSON clientRequestId* UUID, filename* 1–180 символов без путей, size* 1–10485760; mimeType необязателен; изображения, документы и архивы из [списка поддерживаемых вложений](ATTACHMENTS.md)

- **Ответ:** 201 новый/200 повтор: status,item: Attachment,uploadUrl

- **Ошибки:** 422 метаданные/MIME/размер; 409 конфликт ключа/read-only; 403

Источники: [internal/transport/http/chat_routes.go:75](../internal/transport/http/chat_routes.go#L75), [internal/app/chat/handler.go:196](../internal/app/chat/handler.go#L196).

### `PUT /api/v1/conferences/:id/attachments/:attachmentId/content`

Передать байты в зарезервированное вложение.

- **Доступ:** Bearer; допущенный joined участник active конференции, не вышедший из чата; владелец вложения

- **Запрос:** id,attachmentId; raw body, до 10 MiB; размер должен совпасть с init

- **Ответ:** 200: status,item: Attachment

- **Ошибки:** 422 размер/содержимое; 409 истекло/занято/read-only; 403; 404

Источники: [internal/transport/http/chat_routes.go:76](../internal/transport/http/chat_routes.go#L76), [internal/app/chat/handler.go:221](../internal/app/chat/handler.go#L221).

### `PUT /api/v1/conversations/:id/attachments/:attachmentId/content`

Передать байты в зарезервированное вложение.

- **Доступ:** Bearer аккаунта; действующий член нескрытого conversation; владелец вложения

- **Запрос:** id,attachmentId; raw body, до 10 MiB; размер должен совпасть с init

- **Ответ:** 200: status,item: Attachment

- **Ошибки:** 422 размер/содержимое; 409 истекло/занято/read-only; 403; 404

Источники: [internal/transport/http/chat_routes.go:76](../internal/transport/http/chat_routes.go#L76), [internal/app/chat/handler.go:221](../internal/app/chat/handler.go#L221).

### `POST /api/v1/conferences/:id/attachments/:attachmentId/finalize`

Проверить и завершить загрузку.

- **Доступ:** Bearer; допущенный joined участник active конференции, не вышедший из чата; владелец вложения

- **Запрос:** id,attachmentId; пусто или {}

- **Ответ:** 200: status,item: Attachment

- **Ошибки:** 409 файл не загружен/состояние; 422 содержимое; 403; 404

Источники: [internal/transport/http/chat_routes.go:77](../internal/transport/http/chat_routes.go#L77), [internal/app/chat/handler.go:248](../internal/app/chat/handler.go#L248).

### `POST /api/v1/conversations/:id/attachments/:attachmentId/finalize`

Проверить и завершить загрузку.

- **Доступ:** Bearer аккаунта; действующий член нескрытого conversation; владелец вложения

- **Запрос:** id,attachmentId; пусто или {}

- **Ответ:** 200: status,item: Attachment

- **Ошибки:** 409 файл не загружен/состояние; 422 содержимое; 403; 404

Источники: [internal/transport/http/chat_routes.go:77](../internal/transport/http/chat_routes.go#L77), [internal/app/chat/handler.go:248](../internal/app/chat/handler.go#L248).

### `GET /api/v1/conferences/:id/attachments/:attachmentId/download`

Получить способ скачивания вложения.

- **Доступ:** Bearer; допущенный участник joined/left, не вышедший из чата

- **Запрос:** id,attachmentId

- **Ответ:** 200: status,url,expiresAt (+5 минут); для group также authenticated=true, url ведет на защищенный content route

- **Ошибки:** 403; 404 недоступное/удаленное вложение

Источники: [internal/transport/http/chat_routes.go:78](../internal/transport/http/chat_routes.go#L78), [internal/app/chat/handler.go:272](../internal/app/chat/handler.go#L272).

### `GET /api/v1/conversations/:id/attachments/:attachmentId/download`

Получить способ скачивания вложения.

- **Доступ:** Bearer аккаунта; действующий член нескрытого conversation

- **Запрос:** id,attachmentId

- **Ответ:** 200: status,url,expiresAt (+5 минут); для group также authenticated=true, url ведет на защищенный content route

- **Ошибки:** 403; 404 недоступное/удаленное вложение

Источники: [internal/transport/http/chat_routes.go:78](../internal/transport/http/chat_routes.go#L78), [internal/app/chat/handler.go:272](../internal/app/chat/handler.go#L272).


<a id="api-group-5"></a>

## Записи конференций

### `POST /api/v1/conferences/:id/recordings`

Запросить запись активной конференции.

- **Доступ:** Bearer; вошедший зарегистрированный участник

- **Запрос:** id; пусто либо JSON mode=composite/audio_only/individual_tracks/screen_focus (composite), segmentDurationSec=2–30 (5)

- **Ответ:** 202: status, item: RecordCard; запуск асинхронный

- **Ошибки:** 403 гость/нет допуска; 409 конференция не active или запись уже активна; 422 mode/segment; 404

Источники: [internal/transport/http/conference_recording_routes.go:19](../internal/transport/http/conference_recording_routes.go#L19), [internal/app/recordings/handler.go:116](../internal/app/recordings/handler.go#L116).

### `GET /api/v1/conferences/:id/recordings`

Список записей конференции.

- **Доступ:** Bearer; доступ к истории

- **Запрос:** id; limit 1–100 (20), offset ≥0

- **Ответ:** 200: status, items: RecordCard[]

- **Ошибки:** 403; 404; 422

Источники: [internal/transport/http/conference_recording_routes.go:20](../internal/transport/http/conference_recording_routes.go#L20), [internal/app/recordings/handler.go:202](../internal/app/recordings/handler.go#L202).

### `GET /api/v1/conferences/:id/recordings/:recordingId`

Карточка записи и доступные файлы.

- **Доступ:** Bearer; доступ к истории

- **Запрос:** id,recordingId UUID

- **Ответ:** 200: status, item: RecordCard с files, segments и URL

- **Ошибки:** 403; 404 чужая/удаленная запись

Источники: [internal/transport/http/conference_recording_routes.go:21](../internal/transport/http/conference_recording_routes.go#L21), [internal/app/recordings/handler.go:181](../internal/app/recordings/handler.go#L181).

### `DELETE /api/v1/conferences/:id/recordings/:recordingId`

Удалить завершенную запись и ее артефакты.

- **Доступ:** Bearer; зарегистрированный owner или requestedBy; доступ к истории

- **Запрос:** id,recordingId; без тела

- **Ответ:** 200: status=success

- **Ошибки:** 403; 409 еще обрабатывается; 404; ошибка хранилища 500/503

Источники: [internal/transport/http/conference_recording_routes.go:22](../internal/transport/http/conference_recording_routes.go#L22), [internal/app/recordings/handler.go:161](../internal/app/recordings/handler.go#L161).

### `POST /api/v1/conferences/:id/recordings/:recordingId/stop`

Запросить остановку записи.

- **Доступ:** Bearer; зарегистрированный owner или requestedBy; доступ к истории

- **Запрос:** id,recordingId; пусто или {}

- **Ответ:** 202: status, item: RecordCard; остановка асинхронна и идемпотентна для terminal/processing

- **Ошибки:** 403; 404

Источники: [internal/transport/http/conference_recording_routes.go:23](../internal/transport/http/conference_recording_routes.go#L23), [internal/app/recordings/handler.go:137](../internal/app/recordings/handler.go#L137).


<a id="api-group-6"></a>

## Контент и общий поиск

### `GET /api/v1/conferences/:id/recordings/:recordingId/transcript`

Метаданные транскрипции записи.

- **Доступ:** Bearer; доступ к истории

- **Запрос:** id,recordingId

- **Ответ:** 200: status,item: Transcript либо null,enabled,canRetry,providerMode

- **Ошибки:** 403; 404 запись

Источники: [internal/transport/http/content_routes.go:36](../internal/transport/http/content_routes.go#L36), [internal/app/content/handler.go:170](../internal/app/content/handler.go#L170).

### `GET /api/v1/conferences/:id/recordings/:recordingId/transcript/segments`

Страница текста с временными метками.

- **Доступ:** Bearer; доступ к истории

- **Запрос:** id,recordingId; limit 1–200 (100),offset 0–100000 (0)

- **Ответ:** 200: status,items: Segment[],total,limit,offset

- **Ошибки:** 409 транскрипция не ready; 422 pagination; 403; 404

Источники: [internal/transport/http/content_routes.go:37](../internal/transport/http/content_routes.go#L37), [internal/app/content/handler.go:187](../internal/app/content/handler.go#L187).

### `POST /api/v1/conferences/:id/recordings/:recordingId/transcript/retry`

Поставить первую/повторную транскрипцию.

- **Доступ:** Bearer; owner/co_host с доступом к истории

- **Запрос:** id,recordingId; пусто или {}

- **Ответ:** 202: status,item: Transcript,enabled,canRetry=false,providerMode

- **Ошибки:** 409 запись не готова/не failed/лимит/пауза повторов; 503 STT выключен; 403; 404

Источники: [internal/transport/http/content_routes.go:38](../internal/transport/http/content_routes.go#L38), [internal/app/content/handler.go:230](../internal/app/content/handler.go#L230).

### `GET /api/v1/conferences/:id/recordings/:recordingId/summary`

Прочитать AI summary.

- **Доступ:** Bearer; доступ к истории

- **Запрос:** id,recordingId

- **Ответ:** 200: status,item: Summary либо null,enabled,canRegenerate,providerMode

- **Ошибки:** 403; 404

Источники: [internal/transport/http/content_routes.go:39](../internal/transport/http/content_routes.go#L39), [internal/app/content/handler.go:212](../internal/app/content/handler.go#L212).

### `POST /api/v1/conferences/:id/recordings/:recordingId/summary/regenerate`

Поставить повторную генерацию summary.

- **Доступ:** Bearer; owner/co_host с доступом к истории

- **Запрос:** id,recordingId; пусто или {}

- **Ответ:** 202: status,item: Summary,enabled,canRegenerate=false,providerMode

- **Ошибки:** 409 нет готовой транскрипции/лимит/пауза повторов; 503 AI выключен; 403; 404

Источники: [internal/transport/http/content_routes.go:40](../internal/transport/http/content_routes.go#L40), [internal/app/content/handler.go:247](../internal/app/content/handler.go#L247).

### `GET /api/v1/search`

Поиск конференций, транскрипций и summary.

- **Доступ:** Bearer аккаунта; фильтрация доступных конференций

- **Запрос:** Query q* ≤500 символов/2000 байт; source=all/conference/transcript/summary; mode=keyword/semantic/hybrid; conferenceId,participantId UUID; membership=all/owned/participating; from/to RFC3339; limit 1–100 (20),offset 0–10000

- **Ответ:** 200: status,items: SearchResult[],total,limit,offset,effectiveMode,fallbackReason

- **Ошибки:** 422 фильтры; 429; 503 инфраструктура; semantic/hybrid могут вернуться как fallback

Источники: [internal/transport/http/content_routes.go:41](../internal/transport/http/content_routes.go#L41), [internal/app/content/handler.go:264](../internal/app/content/handler.go#L264).


<a id="api-group-7"></a>

## Реакции и TURN

### `POST /api/v1/conferences/:id/reactions`

Разослать временную реакцию.

- **Доступ:** Bearer; вошедший допущенный участник active конференции

- **Запрос:** id; JSON emoji*=👍/👏/❤️/😂

- **Ответ:** 200: status=success; WS reaction.created

- **Ошибки:** 422 emoji; 403 допуск; 409 не active; 429 лимит реакций

Источники: [internal/transport/http/engagement_routes.go:24](../internal/transport/http/engagement_routes.go#L24), [internal/app/engagement/handler.go:117](../internal/app/engagement/handler.go#L117).


<a id="api-group-8"></a>

## Личные папки

### `GET /api/v1/folders`

Список папок и, опционально, принадлежность элемента.

- **Доступ:** Аккаунт; собственные папки

- **Запрос:** Query itemKind=conversation/conference и itemId UUID совместно

- **Ответ:** 200: status,items: Folder[]; contains при фильтре

- **Ошибки:** 422 фильтр; 403

Источники: [internal/transport/http/folder_routes.go:38](../internal/transport/http/folder_routes.go#L38), [internal/app/folders/handler.go:227](../internal/app/folders/handler.go#L227).

### `POST /api/v1/folders`

Создать папку.

- **Доступ:** Аккаунт

- **Запрос:** JSON name* 1–50 символов

- **Ответ:** 201: status,item: Folder

- **Ошибки:** 422 name; 409 лимит 100 папок/конфликт имени

Источники: [internal/transport/http/folder_routes.go:39](../internal/transport/http/folder_routes.go#L39), [internal/app/folders/handler.go:241](../internal/app/folders/handler.go#L241).

### `PUT /api/v1/folders/order`

Переставить папки.

- **Доступ:** Аккаунт; собственные папки

- **Запрос:** JSON ids* UUID[]; полный список без повторов

- **Ответ:** 200: status,items: Folder[]

- **Ошибки:** 422 список; 409 набор устарел; 403

Источники: [internal/transport/http/folder_routes.go:40](../internal/transport/http/folder_routes.go#L40), [internal/app/folders/handler.go:319](../internal/app/folders/handler.go#L319).

### `GET /api/v1/folders/:id`

Карточка папки.

- **Доступ:** Аккаунт; владелец папки

- **Запрос:** id

- **Ответ:** 200: status,item: Folder

- **Ошибки:** 404 чужая/нет папки

Источники: [internal/transport/http/folder_routes.go:41](../internal/transport/http/folder_routes.go#L41), [internal/app/folders/handler.go:261](../internal/app/folders/handler.go#L261).

### `PATCH /api/v1/folders/:id`

Переименовать папку.

- **Доступ:** Аккаунт; владелец папки

- **Запрос:** id; JSON name* 1–50

- **Ответ:** 200: status,item: Folder

- **Ошибки:** 422 имя; 409 конфликт имени; 404

Источники: [internal/transport/http/folder_routes.go:42](../internal/transport/http/folder_routes.go#L42), [internal/app/folders/handler.go:278](../internal/app/folders/handler.go#L278).

### `DELETE /api/v1/folders/:id`

Удалить папку.

- **Доступ:** Аккаунт; владелец папки

- **Запрос:** id

- **Ответ:** 200: status=success

- **Ошибки:** 404

Источники: [internal/transport/http/folder_routes.go:43](../internal/transport/http/folder_routes.go#L43), [internal/app/folders/handler.go:302](../internal/app/folders/handler.go#L302).

### `GET /api/v1/folders/:id/items`

Содержимое папки.

- **Доступ:** Аккаунт; владелец папки

- **Запрос:** id; before; limit 1–100 (50),type=all/conversation/conference,search

- **Ответ:** 200: items: {type,item,inFolder?}[],nextCursor

- **Ошибки:** 422 фильтр/cursor; 404

Источники: [internal/transport/http/folder_routes.go:44](../internal/transport/http/folder_routes.go#L44), [internal/app/folders/handler.go:408](../internal/app/folders/handler.go#L408).

### `GET /api/v1/folder-items`

Доступные элементы для добавления в папку.

- **Доступ:** Аккаунт

- **Запрос:** Query folderId UUID, before,limit 1–100 (50),type=all/conversation/conference,search

- **Ответ:** 200: items: {type,item,inFolder}[],nextCursor

- **Ошибки:** 422 фильтр/cursor; 404 folder

Источники: [internal/transport/http/folder_routes.go:45](../internal/transport/http/folder_routes.go#L45), [internal/app/folders/handler.go:430](../internal/app/folders/handler.go#L430).

### `PUT /api/v1/folders/:id/items/:kind/:itemId`

Добавить элемент в папку.

- **Доступ:** Аккаунт; владелец папки и доступ к элементу

- **Запрос:** id,itemId UUID; kind=conversation/conference; без тела

- **Ответ:** 200: status,item: Folder

- **Ошибки:** 422 kind; 403 элемент недоступен; 404

Источники: [internal/transport/http/folder_routes.go:46](../internal/transport/http/folder_routes.go#L46), [internal/app/folders/handler.go:343](../internal/app/folders/handler.go#L343).

### `DELETE /api/v1/folders/:id/items/:kind/:itemId`

Убрать элемент из папки.

- **Доступ:** Аккаунт; владелец папки

- **Запрос:** id,itemId UUID; kind=conversation/conference

- **Ответ:** 200: status,item: Folder

- **Ошибки:** 422 kind; 403/404 недоступно

Источники: [internal/transport/http/folder_routes.go:47](../internal/transport/http/folder_routes.go#L47), [internal/app/folders/handler.go:350](../internal/app/folders/handler.go#L350).


<a id="api-group-9"></a>

## Гостевой доступ

### `POST /api/v1/conference-invites/:code/guest`

Войти гостем по приглашению.

- **Доступ:** Публичный; при возобновлении гостевой Bearer

- **Запрос:** code; JSON displayName*; Authorization необязателен и должен содержать гостевой токен

- **Ответ:** 200: status,accessToken,tokenType=Bearer,expiresIn=3600,user,item: Participant

- **Ошибки:** 401 ошибочный токен; 403 обычный аккаунт/другая конференция/отклонен; 404 code; 409 завершена; 422 имя; 429

Источники: [internal/transport/http/guest_routes.go:23](../internal/transport/http/guest_routes.go#L23), [internal/app/conferences/guest_handler.go:23](../internal/app/conferences/guest_handler.go#L23).


<a id="api-group-10"></a>

## Интеграции

### `GET /api/v1/notifications/preferences`

Настройки каналов и видов уведомлений.

- **Доступ:** Bearer аккаунта

- **Запрос:** Без тела/query

- **Ответ:** 200: объект invitation,reminder,recording,summary,email,push (без status)

- **Ошибки:** 401/403

Источники: [internal/transport/http/integration_routes.go:52](../internal/transport/http/integration_routes.go#L52), [internal/app/integrations/handler.go:39](../internal/app/integrations/handler.go#L39).

### `PUT /api/v1/notifications/preferences`

Сохранить полный набор настроек уведомлений.

- **Доступ:** Bearer аккаунта

- **Запрос:** Пусто либо JSON invitation,reminder,recording,summary,email,push bool; пропущенные bool получают false

- **Ответ:** 200: сохраненный объект Preferences (без status)

- **Ошибки:** 400 JSON; 401/403

Источники: [internal/transport/http/integration_routes.go:53](../internal/transport/http/integration_routes.go#L53), [internal/app/integrations/handler.go:53](../internal/app/integrations/handler.go#L53).

### `GET /api/v1/notifications/devices`

Зарегистрированные push-устройства.

- **Доступ:** Bearer аккаунта

- **Запрос:** Без тела/query

- **Ответ:** 200: items: Device[] без токенов

- **Ошибки:** 401/403

Источники: [internal/transport/http/integration_routes.go:54](../internal/transport/http/integration_routes.go#L54), [internal/app/integrations/handler.go:70](../internal/app/integrations/handler.go#L70).

### `POST /api/v1/notifications/devices`

Зарегистрировать push-устройство.

- **Доступ:** Bearer аккаунта

- **Запрос:** JSON platform*=web/ios/android,token* 8–8192 байт

- **Ответ:** 201: item: Device без токена

- **Ошибки:** 422 поля; 503 push/cipher недоступны

Источники: [internal/transport/http/integration_routes.go:55](../internal/transport/http/integration_routes.go#L55), [internal/app/integrations/handler.go:85](../internal/app/integrations/handler.go#L85).

### `DELETE /api/v1/notifications/devices/:deviceId`

Отозвать push-устройство.

- **Доступ:** Bearer аккаунта; собственное устройство

- **Запрос:** deviceId UUID

- **Ответ:** 204 без тела

- **Ошибки:** 422 UUID; 404 чужое/не найдено

Источники: [internal/transport/http/integration_routes.go:56](../internal/transport/http/integration_routes.go#L56), [internal/app/integrations/handler.go:102](../internal/app/integrations/handler.go#L102).

### `GET /api/v1/integrations/capabilities`

Настроенные провайдеры интеграций.

- **Доступ:** Bearer аккаунта

- **Запрос:** Без тела/query

- **Ответ:** 200: email,push,calendar,calendarOAuthConfigured,mockConnectAllowed

- **Ошибки:** 401/403

Источники: [internal/transport/http/integration_routes.go:58](../internal/transport/http/integration_routes.go#L58), [internal/app/integrations/handler.go:30](../internal/app/integrations/handler.go#L30).

### `GET /api/v1/integrations/calendars`

Список календарных подключений.

- **Доступ:** Bearer аккаунта

- **Запрос:** Без тела/query

- **Ответ:** 200: items: CalendarConnection[] без секретов

- **Ошибки:** 401/403

Источники: [internal/transport/http/integration_routes.go:59](../internal/transport/http/integration_routes.go#L59), [internal/app/integrations/handler.go:119](../internal/app/integrations/handler.go#L119).

### `POST /api/v1/integrations/calendars/mock`

Создать mock-подключение календаря.

- **Доступ:** Bearer аккаунта; разрешенный mock-режим

- **Запрос:** Пусто или {}

- **Ответ:** 201: item: CalendarConnection

- **Ошибки:** 503 mock connect выключен

Источники: [internal/transport/http/integration_routes.go:60](../internal/transport/http/integration_routes.go#L60), [internal/app/integrations/handler.go:134](../internal/app/integrations/handler.go#L134).

### `GET /api/v1/integrations/calendars/:provider/connect`

Начать OAuth календаря.

- **Доступ:** Bearer аккаунта

- **Запрос:** provider=generic

- **Ответ:** 200: authUrl,state; одноразовый state живет 10 минут

- **Ошибки:** 503 иной provider/OAuth/cipher недоступны

Источники: [internal/transport/http/integration_routes.go:61](../internal/transport/http/integration_routes.go#L61), [internal/app/integrations/handler.go:151](../internal/app/integrations/handler.go#L151).

### `POST /api/v1/integrations/calendars/:provider/callback`

Завершить OAuth календаря.

- **Доступ:** Bearer аккаунта

- **Запрос:** provider=generic; JSON code* 1–4096 байт,state* 43 символа

- **Ответ:** 200: item: CalendarConnection

- **Ошибки:** 422 поля/истекший или использованный state; 503 provider/cipher/обмен OAuth

Источники: [internal/transport/http/integration_routes.go:62](../internal/transport/http/integration_routes.go#L62), [internal/app/integrations/handler.go:172](../internal/app/integrations/handler.go#L172).

### `DELETE /api/v1/integrations/calendars/:provider`

Отключить календарь.

- **Доступ:** Bearer аккаунта; свое подключение

- **Запрос:** Параметр с именем provider фактически UUID подключения из calendars.items[].id

- **Ответ:** 204 без тела

- **Ошибки:** 422 если передать generic вместо UUID; 404 подключение

Источники: [internal/transport/http/integration_routes.go:63](../internal/transport/http/integration_routes.go#L63), [internal/app/integrations/handler.go:190](../internal/app/integrations/handler.go#L190).

### `GET /api/v1/conferences/:id/calendar`

Календарные события конференции.

- **Доступ:** Bearer; owner/co_host с доступом к истории

- **Запрос:** id UUID

- **Ответ:** 200: items: CalendarMapping[], не более 10

- **Ошибки:** 403 нет роли/допуска; 422 UUID

Источники: [internal/transport/http/integration_routes.go:66](../internal/transport/http/integration_routes.go#L66), [internal/app/integrations/handler.go:206](../internal/app/integrations/handler.go#L206).


<a id="api-group-11"></a>

## Приглашения

### `GET /api/v1/conferences/:id/invitation-users`

Найти адресатов приглашения.

- **Доступ:** Аккаунт; owner либо вошедший co_host в active конференции

- **Запрос:** id; query* 2–100 символов

- **Ответ:** 200: status,items: id,email,displayName

- **Ошибки:** 422 query; 403 роль; 409 конференция завершена; 404

Источники: [internal/transport/http/invitation_routes.go:32](../internal/transport/http/invitation_routes.go#L32), [internal/app/conferences/invitation_handler.go:46](../internal/app/conferences/invitation_handler.go#L46).

### `POST /api/v1/conferences/:id/invitations`

Поставить доставку приглашений.

- **Доступ:** Аккаунт; owner либо вошедший co_host в active конференции

- **Запрос:** id; JSON emails[] и/или userIds[] UUID; всего 1–20 адресатов

- **Ответ:** 202: status,items: InvitationResult[] (id,email,userId,status)

- **Ошибки:** 422 список/email; 403 роль; 409 конференция закрыта; 404

Источники: [internal/transport/http/invitation_routes.go:33](../internal/transport/http/invitation_routes.go#L33), [internal/app/conferences/invitation_handler.go:63](../internal/app/conferences/invitation_handler.go#L63).


<a id="api-group-12"></a>

## Уведомления

### `GET /api/v1/notifications`

Лента уведомлений.

- **Доступ:** Bearer аккаунта; только свои видимые уведомления

- **Запрос:** Query cursor,limit 1–100 (30)

- **Ответ:** 200: items: Notification[],nextCursor,unreadCount (без status)

- **Ошибки:** 422 cursor/limit; 403 гость

Источники: [internal/transport/http/notification_routes.go:18](../internal/transport/http/notification_routes.go#L18), [internal/app/notifications/handler.go:50](../internal/app/notifications/handler.go#L50).

### `GET /api/v1/notifications/events`

Поток SSE уведомлений.

- **Доступ:** Bearer аккаунта в заголовке; query запрещен

- **Запрос:** Без query/body; Authorization: Bearer; Last-Event-ID не воспроизводит историю

- **Ответ:** 200: text/event-stream; data: Envelope для notification.created/read; keepalive 15 c

- **Ошибки:** 401 query/токен; 429 >4 потоков на пользователя или >12 подключений/мин; 503 >256 потоков/Redis

Источники: [internal/transport/http/notification_routes.go:19](../internal/transport/http/notification_routes.go#L19), [internal/app/notifications/handler.go:94](../internal/app/notifications/handler.go#L94).

### `POST /api/v1/notifications/:notificationId/read`

Отметить уведомление прочитанным.

- **Доступ:** Bearer аккаунта; собственное видимое уведомление

- **Запрос:** notificationId UUID; пусто или {}

- **Ответ:** 200: item: Notification (без status); идемпотентно

- **Ошибки:** 404 недоступное уведомление; 422 UUID

Источники: [internal/transport/http/notification_routes.go:20](../internal/transport/http/notification_routes.go#L20), [internal/app/notifications/handler.go:73](../internal/app/notifications/handler.go#L73).


<a id="api-group-13"></a>

## Приватные ресурсы групп

### `PUT /api/v1/conversations/:id/avatar`

Загрузить аватар группы.

- **Доступ:** Аккаунт; owner/admin группы

- **Запрос:** id; raw JPEG/PNG; Content-Type image/jpeg или image/png; ≤2 MiB, стороны ≤2048 и ≤4 Mi pixels

- **Ответ:** 200: status,item: Conversation с avatarVersion

- **Ошибки:** 422 формат/размер/изображение; 403 роль; 404

Источники: [internal/transport/http/personal_asset_routes.go:41](../internal/transport/http/personal_asset_routes.go#L41), [internal/app/personal/assets_handler.go:53](../internal/app/personal/assets_handler.go#L53).

### `DELETE /api/v1/conversations/:id/avatar`

Удалить аватар группы.

- **Доступ:** Аккаунт; owner/admin группы

- **Запрос:** id

- **Ответ:** 200: status,item: Conversation

- **Ошибки:** 403; 404

Источники: [internal/transport/http/personal_asset_routes.go:42](../internal/transport/http/personal_asset_routes.go#L42), [internal/app/personal/assets_handler.go:75](../internal/app/personal/assets_handler.go#L75).

### `GET /api/v1/conversations/:id/avatar/content`

Прочитать аватар через авторизованный поток.

- **Доступ:** Аккаунт; действующий член группы

- **Запрос:** id

- **Ответ:** 200: байты image/jpeg или image/png, Content-Length, private/no-store

- **Ошибки:** 403; 404 отсутствует; поток может оборваться при отзыве доступа

Источники: [internal/transport/http/personal_asset_routes.go:43](../internal/transport/http/personal_asset_routes.go#L43), [internal/app/personal/assets_handler.go:92](../internal/app/personal/assets_handler.go#L92).

### `GET /api/v1/conversations/:id/attachments/:attachmentId/content`

Скачать вложение через авторизованный поток.

- **Доступ:** Аккаунт; действующий член группы

- **Запрос:** id,attachmentId

- **Ответ:** 200: application/octet-stream, Content-Disposition attachment, Content-Length

- **Ошибки:** 403; 404 вложение недоступно; ошибка после начала прерывает поток

Источники: [internal/transport/http/personal_asset_routes.go:44](../internal/transport/http/personal_asset_routes.go#L44), [internal/app/personal/assets_handler.go:105](../internal/app/personal/assets_handler.go#L105).


<a id="api-group-14"></a>

## Личные и групповые диалоги

### `GET /api/v1/users`

Найти пользователей для диалога.

- **Доступ:** Аккаунт

- **Запрос:** Query search* 2–100 символов; поиск по displayName; до 20 других зарегистрированных пользователей

- **Ответ:** 200: status,items: Peer[] (id,displayName)

- **Ошибки:** 422 search; 403 гость

Источники: [internal/transport/http/personal_routes.go:41](../internal/transport/http/personal_routes.go#L41), [internal/app/personal/handler.go:285](../internal/app/personal/handler.go#L285).

### `GET /api/v1/conversations`

Список личных/групповых диалогов.

- **Доступ:** Аккаунт

- **Запрос:** Query before,limit 1–100 (50),type пусто/direct/group; значение all недопустимо (422),search ≤100,unreadOnly bool

- **Ответ:** 200: items: Conversation[],nextCursor,unreadCount (без status)

- **Ошибки:** 422 фильтры/cursor; 403

Источники: [internal/transport/http/personal_routes.go:42](../internal/transport/http/personal_routes.go#L42), [internal/app/personal/handler.go:236](../internal/app/personal/handler.go#L236).

### `POST /api/v1/conversations/direct`

Открыть или создать личный диалог.

- **Доступ:** Аккаунт

- **Запрос:** JSON userId* UUID другого зарегистрированного пользователя

- **Ответ:** 201 новый/200 существующий: status,item: Conversation

- **Ошибки:** 422 UUID/сам с собой; 403/404 адресат недоступен

Источники: [internal/transport/http/personal_routes.go:43](../internal/transport/http/personal_routes.go#L43), [internal/app/personal/handler.go:204](../internal/app/personal/handler.go#L204).

### `POST /api/v1/conversations/group`

Создать группу с идемпотентностью.

- **Доступ:** Аккаунт

- **Запрос:** JSON clientRequestId* UUID,name* 1–50,description ≤200,memberIds UUID[]; всего ≤100 участников

- **Ответ:** 201 новый/200 повтор: status,item: Conversation

- **Ошибки:** 422 поля; 409 ключ с другим запросом/лимит; 404 участник

Источники: [internal/transport/http/personal_routes.go:44](../internal/transport/http/personal_routes.go#L44), [internal/app/personal/group_handler.go:172](../internal/app/personal/group_handler.go#L172).

### `GET /api/v1/conversations/:id`

Карточка диалога.

- **Доступ:** Аккаунт; член conversation

- **Запрос:** id

- **Ответ:** 200: status,item: Conversation

- **Ошибки:** 403 нет членства/скрыт; 404

Источники: [internal/transport/http/personal_routes.go:45](../internal/transport/http/personal_routes.go#L45), [internal/app/personal/handler.go:267](../internal/app/personal/handler.go#L267).

### `GET /api/v1/conversations/:id/peer-presence`

Онлайн-статус собеседника.

- **Доступ:** Аккаунт; член direct conversation

- **Запрос:** id

- **Ответ:** 200: status,item: conversationId,peerId,online

- **Ошибки:** 400 неверный id; 403 доступ/не direct

Источники: [internal/transport/http/personal_routes.go:46](../internal/transport/http/personal_routes.go#L46), [internal/app/personal/peer_presence_handler.go:18](../internal/app/personal/peer_presence_handler.go#L18).

### `PATCH /api/v1/conversations/:id/preferences`

Личная настройка уведомлений.

- **Доступ:** Аккаунт; член conversation

- **Запрос:** id; JSON notificationsEnabled* bool

- **Ответ:** 200: status,item: Conversation

- **Ошибки:** 422 поле отсутствует; 403

Источники: [internal/transport/http/personal_routes.go:47](../internal/transport/http/personal_routes.go#L47), [internal/app/personal/direct_actions_handler.go:40](../internal/app/personal/direct_actions_handler.go#L40).

### `POST /api/v1/conversations/:id/clear-history`

Скрыть прежнюю историю только для себя.

- **Доступ:** Аккаунт; член direct conversation

- **Запрос:** id; без тела

- **Ответ:** 200: status,item: Conversation с historyClearedThrough

- **Ошибки:** 403 доступ/не direct

Источники: [internal/transport/http/personal_routes.go:48](../internal/transport/http/personal_routes.go#L48), [internal/app/personal/direct_actions_handler.go:69](../internal/app/personal/direct_actions_handler.go#L69).

### `POST /api/v1/conversations/:id/hide`

Скрыть диалог для себя.

- **Доступ:** Аккаунт; член direct conversation

- **Запрос:** id; без тела

- **Ответ:** 200: status,hidden=true,historyClearedThrough

- **Ошибки:** 403 доступ/не direct

Источники: [internal/transport/http/personal_routes.go:49](../internal/transport/http/personal_routes.go#L49), [internal/app/personal/direct_actions_handler.go:88](../internal/app/personal/direct_actions_handler.go#L88).

### `GET /api/v1/conversations/:id/pins`

Общие закрепленные сообщения диалога.

- **Доступ:** Аккаунт; член conversation

- **Запрос:** id; before; limit 1–100 (50)

- **Ответ:** 200: items: Message[],nextCursor (без status)

- **Ошибки:** 403; 422 cursor; 404

Источники: [internal/transport/http/personal_routes.go:50](../internal/transport/http/personal_routes.go#L50), [internal/app/personal/pins_handler.go:18](../internal/app/personal/pins_handler.go#L18).

### `PUT /api/v1/conversations/:id/pins/:messageId`

Закрепить доступное текстовое сообщение для диалога.

- **Доступ:** Аккаунт; член conversation

- **Запрос:** id,messageId; без тела

- **Ответ:** 200: status=success

- **Ошибки:** 409 уже 20 закрепленных; 404 удалено/служебное/за границей истории; 403

Источники: [internal/transport/http/personal_routes.go:51](../internal/transport/http/personal_routes.go#L51), [internal/app/personal/pins_handler.go:48](../internal/app/personal/pins_handler.go#L48).

### `DELETE /api/v1/conversations/:id/pins/:messageId`

Открепить сообщение диалога.

- **Доступ:** Аккаунт; член conversation

- **Запрос:** id,messageId; без тела

- **Ответ:** 200: status=success

- **Ошибки:** 403; 404 сообщение недоступно

Источники: [internal/transport/http/personal_routes.go:52](../internal/transport/http/personal_routes.go#L52), [internal/app/personal/pins_handler.go:48](../internal/app/personal/pins_handler.go#L48).

### `PATCH /api/v1/conversations/:id`

Изменить имя и описание группы.

- **Доступ:** Аккаунт; owner/admin группы

- **Запрос:** id; JSON name 1–50 и/или description ≤200

- **Ответ:** 200: status,item: Conversation

- **Ошибки:** 403 роль; 422 поля; 404

Источники: [internal/transport/http/personal_routes.go:53](../internal/transport/http/personal_routes.go#L53), [internal/app/personal/group_handler.go:197](../internal/app/personal/group_handler.go#L197).

### `DELETE /api/v1/conversations/:id`

Удалить группу.

- **Доступ:** Аккаунт; owner группы

- **Запрос:** id; без тела

- **Ответ:** 200: status=success

- **Ошибки:** 403 роль/не группа; 404

Источники: [internal/transport/http/personal_routes.go:54](../internal/transport/http/personal_routes.go#L54), [internal/app/personal/group_handler.go:368](../internal/app/personal/group_handler.go#L368).

### `GET /api/v1/conversations/:id/members`

Список участников группы.

- **Доступ:** Аккаунт; член группы

- **Запрос:** id

- **Ответ:** 200: status,items: Member[] (id,displayName,role,online)

- **Ошибки:** 403; 404

Источники: [internal/transport/http/personal_routes.go:55](../internal/transport/http/personal_routes.go#L55), [internal/app/personal/group_handler.go:219](../internal/app/personal/group_handler.go#L219).

### `POST /api/v1/conversations/:id/members`

Добавить зарегистрированных участников.

- **Доступ:** Аккаунт; owner/admin группы

- **Запрос:** id; JSON userIds* UUID[]; итог ≤100 участников

- **Ответ:** 200: status,item: Conversation

- **Ошибки:** 403 роль; 409 лимит; 422 список; 404 пользователь

Источники: [internal/transport/http/personal_routes.go:56](../internal/transport/http/personal_routes.go#L56), [internal/app/personal/group_handler.go:236](../internal/app/personal/group_handler.go#L236).

### `DELETE /api/v1/conversations/:id/members/:userId`

Удалить участника; admin может удалить только member.

- **Доступ:** Аккаунт; owner/admin группы

- **Запрос:** id,userId; без тела

- **Ответ:** 200: status,item: Conversation

- **Ошибки:** 403 владелец/недостаточная роль; 409 самоудаление (использовать leave); 404

Источники: [internal/transport/http/personal_routes.go:57](../internal/transport/http/personal_routes.go#L57), [internal/app/personal/group_handler.go:262](../internal/app/personal/group_handler.go#L262).

### `PATCH /api/v1/conversations/:id/members/:userId`

Назначить admin/member.

- **Доступ:** Аккаунт; owner группы

- **Запрос:** id,userId; JSON role*=admin/member

- **Ответ:** 200: status,item: Conversation

- **Ошибки:** 403 собственная роль/owner/не владелец; 422 role; 404

Источники: [internal/transport/http/personal_routes.go:58](../internal/transport/http/personal_routes.go#L58), [internal/app/personal/group_handler.go:286](../internal/app/personal/group_handler.go#L286).

### `POST /api/v1/conversations/:id/leave`

Выйти из группы.

- **Доступ:** Аккаунт; член группы

- **Запрос:** id; пусто или {}

- **Ответ:** 200: status=success

- **Ошибки:** 409 owner должен передать владение, если остаются участники; 403

Источники: [internal/transport/http/personal_routes.go:59](../internal/transport/http/personal_routes.go#L59), [internal/app/personal/group_handler.go:345](../internal/app/personal/group_handler.go#L345).

### `POST /api/v1/conversations/:id/ownership`

Передать владение другому участнику; прежний owner становится admin.

- **Доступ:** Аккаунт; owner группы

- **Запрос:** id; JSON userId* UUID участника

- **Ответ:** 200: status,item: Conversation

- **Ошибки:** 403 сам себе/не owner; 404 участник; 422 UUID

Источники: [internal/transport/http/personal_routes.go:60](../internal/transport/http/personal_routes.go#L60), [internal/app/personal/group_handler.go:314](../internal/app/personal/group_handler.go#L314).

### `POST /api/v1/conversations/:id/read`

Алиас отметки прочитанного.

- **Доступ:** Аккаунт; член conversation

- **Запрос:** id; JSON messageId* UUID

- **Ответ:** 200: status,item: lastReadMessageId,unreadCount

- **Ошибки:** 403; 404 сообщение; 422 UUID

Источники: [internal/transport/http/personal_routes.go:61](../internal/transport/http/personal_routes.go#L61), [internal/app/chat/handler.go:174](../internal/app/chat/handler.go#L174).


<a id="api-group-15"></a>

## Поиск в диалогах

### `GET /api/v1/conversations/:id/messages/search`

Поиск сообщений личного/группового диалога.

- **Доступ:** Аккаунт; член conversation

- **Запрос:** id; q* 2–200 символов; before; limit 1–50 (20)

- **Ответ:** 200: status,items,nextCursor,unreadCount,lastReadMessageId

- **Ошибки:** 403; 422 q/cursor/limit; 404

Источники: [internal/transport/http/personal_search_routes.go:36](../internal/transport/http/personal_search_routes.go#L36), [internal/app/chat/search_handler.go:51](../internal/app/chat/search_handler.go#L51).

### `GET /api/v1/conversations/:id/messages/:messageId/context`

Контекст сообщения диалога.

- **Доступ:** Аккаунт; член conversation

- **Запрос:** id,messageId

- **Ответ:** 200: status,items,nextCursor,unreadCount,lastReadMessageId

- **Ошибки:** 403; 404

Источники: [internal/transport/http/personal_search_routes.go:37](../internal/transport/http/personal_search_routes.go#L37), [internal/app/chat/search_handler.go:72](../internal/app/chat/search_handler.go#L72).


<a id="api-group-16"></a>

## Аккаунты и конференции

### `POST /api/v1/auth/register`

Регистрация учетной записи.

- **Доступ:** Публичный; same-origin

- **Запрос:** JSON: email*; password* 8–128 символов; displayName ≤100, необязателен

- **Ответ:** 201: status, user

- **Ошибки:** 409 email занят; 422 поля; 403 origin; 415 Content-Type

Источники: [internal/transport/http/platform_routes.go:22](../internal/transport/http/platform_routes.go#L22), [internal/app/auth/handler.go:86](../internal/app/auth/handler.go#L86).

### `POST /api/v1/auth/login`

Вход и сохранение браузерного сеанса.

- **Доступ:** Публичный; same-origin

- **Запрос:** JSON: email*, password*

- **Ответ:** 200: status, accessToken, tokenType, expiresIn, user; Set-Cookie

- **Ошибки:** 401 неверные данные; 409 лимит 5 аккаунтов в браузере; 403 origin; 415 Content-Type

Источники: [internal/transport/http/platform_routes.go:23](../internal/transport/http/platform_routes.go#L23), [internal/app/auth/handler.go:107](../internal/app/auth/handler.go#L107).

### `POST /api/v1/auth/refresh`

Новый access token из постоянной сессии.

- **Доступ:** Cookie; same-origin

- **Запрос:** Пусто или JSON {userId}; meetrix_session либо meetrix_account_<userId>

- **Ответ:** 200: LoginResponse; обновленные cookie

- **Ошибки:** 401 отсутствует/отозвана сессия; 403 другой аккаунт/origin; 503 сессии выключены

Источники: [internal/transport/http/platform_routes.go:24](../internal/transport/http/platform_routes.go#L24), [internal/app/auth/sessions.go:187](../internal/app/auth/sessions.go#L187).

### `POST /api/v1/auth/accounts/list`

Список сохраненных аккаунтов браузера.

- **Доступ:** Cookie; same-origin

- **Запрос:** Пусто или {}; cookies meetrix_account_<id>

- **Ответ:** 200: status, items: User[]; недействительные cookies удаляются

- **Ошибки:** 409 лимит аккаунтов; 503 сессии выключены; 403 origin

Источники: [internal/transport/http/platform_routes.go:25](../internal/transport/http/platform_routes.go#L25), [internal/app/auth/accounts.go:172](../internal/app/auth/accounts.go#L172).

### `POST /api/v1/auth/accounts/switch`

Выбрать активный аккаунт.

- **Доступ:** Cookie; same-origin

- **Запрос:** JSON: userId* канонический UUID; его meetrix_account_<id>

- **Ответ:** 200: LoginResponse; Set-Cookie meetrix_session

- **Ошибки:** 401 cookie недействителен; 403 несовпадение владельца; 422 UUID; 503 функция выключена

Источники: [internal/transport/http/platform_routes.go:26](../internal/transport/http/platform_routes.go#L26), [internal/app/auth/accounts.go:192](../internal/app/auth/accounts.go#L192).

### `POST /api/v1/auth/logout`

Отозвать выбранный постоянный сеанс.

- **Доступ:** Cookie; same-origin; Bearer необязателен

- **Запрос:** Пусто или JSON {userId}; cookie сеанса; опциональный Bearer для привязки доступа

- **Ответ:** 200: status=success; удаление соответствующих cookies

- **Ошибки:** 403 origin; 400 JSON; 503 нет session service

Источники: [internal/transport/http/platform_routes.go:28](../internal/transport/http/platform_routes.go#L28), [internal/app/auth/handler.go:169](../internal/app/auth/handler.go#L169).

### `GET /api/v1/auth/me`

Профиль текущего пользователя.

- **Доступ:** Bearer; гость допустим в своей области

- **Запрос:** Без тела и query

- **Ответ:** 200: status, user; у гостя email пустой, guestConferenceId

- **Ошибки:** 401 отсутствует аккаунт/сессия

Источники: [internal/transport/http/platform_routes.go:31](../internal/transport/http/platform_routes.go#L31), [internal/app/auth/handler.go:137](../internal/app/auth/handler.go#L137).

### `PATCH /api/v1/auth/me`

Изменить отображаемое имя.

- **Доступ:** Bearer аккаунта

- **Запрос:** JSON: displayName* 1–100 символов без управляющих символов

- **Ответ:** 200: status, user

- **Ошибки:** 422 имя; 403 гостевой scope

Источники: [internal/transport/http/platform_routes.go:32](../internal/transport/http/platform_routes.go#L32), [internal/app/auth/handler.go:151](../internal/app/auth/handler.go#L151).

### `DELETE /api/v1/auth/me`

Удалить аккаунт с проверкой пароля.

- **Доступ:** Bearer аккаунта; same-origin

- **Запрос:** JSON: password*

- **Ответ:** 200: status=success; удаление cookies аккаунта

- **Ошибки:** 422 неверный/пустой пароль; 403 origin/гость; 409 активный звонок/подключение, собственная действующая группа/встреча или изменение пароля во время операции; 503 нет account service

Источники: [internal/transport/http/platform_routes.go:33](../internal/transport/http/platform_routes.go#L33), [internal/app/auth/accounts.go:230](../internal/app/auth/accounts.go#L230).

### `POST /api/v1/auth/session`

Перенести существующий доступ в постоянную сессию.

- **Доступ:** Bearer аккаунта; same-origin

- **Запрос:** Пусто или {userId}; используется аккаунт Bearer, его cookie либо meetrix_session

- **Ответ:** 200: LoginResponse; cookies

- **Ошибки:** 401 недействительный доступ; 403 гость/origin; 503 сессии выключены

Источники: [internal/transport/http/platform_routes.go:34](../internal/transport/http/platform_routes.go#L34), [internal/app/auth/sessions.go:193](../internal/app/auth/sessions.go#L193).

### `POST /api/v1/conferences`

Создать конференцию.

- **Доступ:** Bearer аккаунта

- **Запрос:** JSON: title* 1–200; waitingRoomEnabled; scheduledAt RFC3339 в будущем; plannedDurationMin 1–1440 только с scheduledAt

- **Ответ:** 201: status, item: Conference

- **Ошибки:** 422 название/расписание; 403 гость

Источники: [internal/transport/http/platform_routes.go:40](../internal/transport/http/platform_routes.go#L40), [internal/app/conferences/handler.go:221](../internal/app/conferences/handler.go#L221).

### `GET /api/v1/conferences`

Доступные конференции.

- **Доступ:** Bearer аккаунта

- **Запрос:** Query: limit 1–100 (20), offset ≥0 (0)

- **Ответ:** 200: status, items: Conference[]

- **Ошибки:** 422 pagination; 403 гость

Источники: [internal/transport/http/platform_routes.go:41](../internal/transport/http/platform_routes.go#L41), [internal/app/conferences/handler.go:238](../internal/app/conferences/handler.go#L238).

### `GET /api/v1/conferences/:id`

Карточка конференции.

- **Доступ:** Bearer; доступ к истории конференции

- **Запрос:** id UUID

- **Ответ:** 200: status, item: Conference

- **Ошибки:** 403 нет допуска; 404 конференция

Источники: [internal/transport/http/platform_routes.go:42](../internal/transport/http/platform_routes.go#L42), [internal/app/conferences/handler.go:255](../internal/app/conferences/handler.go#L255).

### `POST /api/v1/conferences/:id/start`

Начать created/scheduled конференцию.

- **Доступ:** Bearer; owner конференции

- **Запрос:** id; пусто или {}

- **Ответ:** 200: status, item с status=active

- **Ошибки:** 403 чужой owner; 409 недопустимый переход; 404

Источники: [internal/transport/http/platform_routes.go:43](../internal/transport/http/platform_routes.go#L43), [internal/app/conferences/handler.go:276](../internal/app/conferences/handler.go#L276).

### `POST /api/v1/conferences/:id/finish`

Завершить active конференцию.

- **Доступ:** Bearer; owner конференции

- **Запрос:** id; пусто или {}

- **Ответ:** 200: status, item с status=finished

- **Ошибки:** 403; 409 если не active; 404

Источники: [internal/transport/http/platform_routes.go:44](../internal/transport/http/platform_routes.go#L44), [internal/app/conferences/handler.go:276](../internal/app/conferences/handler.go#L276).

### `POST /api/v1/conferences/:id/cancel`

Отменить created/scheduled конференцию.

- **Доступ:** Bearer; owner конференции

- **Запрос:** id; пусто или {}

- **Ответ:** 200: status, item с status=cancelled

- **Ошибки:** 403; 409 недопустимый переход; 404

Источники: [internal/transport/http/platform_routes.go:45](../internal/transport/http/platform_routes.go#L45), [internal/app/conferences/handler.go:276](../internal/app/conferences/handler.go#L276).

### `POST /api/v1/conferences/:id/join`

Создать/возобновить членство.

- **Доступ:** Bearer в разрешенной области

- **Запрос:** id; пусто либо JSON {inviteCode}; для нового членства нужен действующий inviteCode

- **Ответ:** 200: status, item: Participant, включая admissionState

- **Ошибки:** 403 неверное приглашение/rejected/kicked; 409 состояние конференции; 422 code

Источники: [internal/transport/http/platform_routes.go:46](../internal/transport/http/platform_routes.go#L46), [internal/app/conferences/handler.go:321](../internal/app/conferences/handler.go#L321).

### `POST /api/v1/conferences/:id/leave`

Выйти из конференции.

- **Доступ:** Bearer; собственное членство

- **Запрос:** id; пусто или {}

- **Ответ:** 200: status, item: Participant

- **Ошибки:** 403 нет членства; 404

Источники: [internal/transport/http/platform_routes.go:47](../internal/transport/http/platform_routes.go#L47), [internal/app/conferences/handler.go:343](../internal/app/conferences/handler.go#L343).

### `GET /api/v1/conferences/:id/participants`

Список участников; waiting/rejected видны owner/co_host.

- **Доступ:** Bearer; доступ к истории

- **Запрос:** id; limit 1–100 (20), offset ≥0

- **Ответ:** 200: status, items: Participant[]

- **Ошибки:** 403 нет допуска; 404; 422 pagination

Источники: [internal/transport/http/platform_routes.go:48](../internal/transport/http/platform_routes.go#L48), [internal/app/conferences/handler.go:299](../internal/app/conferences/handler.go#L299).

### `GET /api/v1/conferences/:id/participants/me`

Состояние допуска текущего участника.

- **Доступ:** Bearer; собственное членство

- **Запрос:** id

- **Ответ:** 200: status, item: Participant

- **Ошибки:** 403/404 нет доступного членства

Источники: [internal/transport/http/platform_routes.go:49](../internal/transport/http/platform_routes.go#L49), [internal/app/conferences/product_handler.go:20](../internal/app/conferences/product_handler.go#L20).

### `POST /api/v1/conferences/:id/participants/:participantId/admission`

Принять/отклонить ожидающего участника.

- **Доступ:** Bearer; вошедший owner/co_host

- **Запрос:** id, participantId UUID; JSON decision*=admit/reject

- **Ответ:** 200: status, item: Participant

- **Ошибки:** 403 роль; 409 состояние допуска/конференции; 422 decision; 404

Источники: [internal/transport/http/platform_routes.go:50](../internal/transport/http/platform_routes.go#L50), [internal/app/conferences/product_handler.go:37](../internal/app/conferences/product_handler.go#L37).

### `PUT /api/v1/conferences/:id/schedule`

Изменить расписание scheduled конференции.

- **Доступ:** Bearer; owner

- **Запрос:** id; JSON scheduledAt* RFC3339 в будущем; plannedDurationMin 1–1440

- **Ответ:** 200: status, item: Conference

- **Ошибки:** 403 не owner; 409 конференция не scheduled; 422 время/длительность; 404

Источники: [internal/transport/http/platform_routes.go:51](../internal/transport/http/platform_routes.go#L51), [internal/app/conferences/product_handler.go:63](../internal/app/conferences/product_handler.go#L63).

### `GET /api/v1/conferences/:id/history`

История finished/cancelled конференции.

- **Доступ:** Bearer; доступ к истории

- **Запрос:** id

- **Ответ:** 200: status, item: conference, owner, durationSec, participantCount, participants, participantsTruncated, recordings, chatAvailable, chatReadOnly

- **Ошибки:** 403; 409 еще не завершена; 404

Источники: [internal/transport/http/platform_routes.go:52](../internal/transport/http/platform_routes.go#L52), [internal/app/conferences/product_handler.go:84](../internal/app/conferences/product_handler.go#L84).

### `GET /api/v1/me/conferences`

Лента конференций пользователя.

- **Доступ:** Bearer аккаунта

- **Запрос:** Query view=upcoming/active/past (upcoming), scope=all/owned/participating (all), sort=newest/oldest (newest), status, from/to RFC3339, cursor, limit 1–100 (20). Порядок и границы дат применяются на сервере до пагинации; при изменении сортировки курсор необходимо сбросить.

- **Ответ:** 200: status, items: Conference[], nextCursor

- **Ошибки:** 422 фильтры/курсор; 403 гость

Источники: [internal/transport/http/platform_routes.go:53](../internal/transport/http/platform_routes.go#L53), [internal/app/conferences/product_handler.go:101](../internal/app/conferences/product_handler.go#L101).

### `GET /api/v1/conference-invites/:code`

Ограниченная карточка по приглашению.

- **Доступ:** Публичный

- **Запрос:** code приглашения

- **Ответ:** 200: status, item: id,title,status,waitingRoomEnabled,scheduledAt

- **Ошибки:** 404 неизвестный/недоступный code

Источники: [internal/transport/http/platform_routes.go:58](../internal/transport/http/platform_routes.go#L58), [internal/app/conferences/handler.go:364](../internal/app/conferences/handler.go#L364).

### `POST /api/v1/conference-invites/:code/join`

Войти по коду приглашения.

- **Доступ:** Bearer аккаунта

- **Запрос:** code; пусто или {}

- **Ответ:** 200: status, item: Participant

- **Ошибки:** 403 запрет повторного входа; 404 code; 409 состояние

Источники: [internal/transport/http/platform_routes.go:59](../internal/transport/http/platform_routes.go#L59), [internal/app/conferences/handler.go:378](../internal/app/conferences/handler.go#L378).

### `PUT /api/v1/conferences/:id/participants/me/media`

Обновить заявленное состояние устройств.

- **Доступ:** Bearer; собственное вошедшее членство

- **Запрос:** id; JSON connectionId* UUID подключенной сессии, sequence* целое 1–9007199254740991; microphoneEnabled,cameraEnabled,screenSharing bool

- **Ответ:** 200: status,item: Participant; старый/повторный sequence игнорируется

- **Ошибки:** 403 нет допуска/подключения/источник заблокирован; 409 комната не active/created; 422 поля

Источники: [internal/transport/http/platform_routes.go:74](../internal/transport/http/platform_routes.go#L74), [internal/app/conferences/control_handler.go:97](../internal/app/conferences/control_handler.go#L97).

### `POST /api/v1/conferences/:id/participants/:participantId/moderation`

Управлять участником.

- **Доступ:** Bearer; owner; co_host ограниченно

- **Запрос:** id,participantId; JSON action*=mute/camera/screen/kick/role; blocked* для медиа; role*=co_host/participant для role

- **Ответ:** 200: status, item: Participant

- **Ошибки:** 403 запрещенная цель/роль; 422 несовместимые поля; 404

Источники: [internal/transport/http/platform_routes.go:75](../internal/transport/http/platform_routes.go#L75), [internal/app/conferences/control_handler.go:71](../internal/app/conferences/control_handler.go#L71).


<a id="api-group-17"></a>

## Возможности платформы

### `GET /api/v1/capabilities`

Функции платформы при текущей конфигурации.

- **Доступ:** Bearer; гостям разрешено

- **Запрос:** Без тела/query

- **Ответ:** 200: status,capabilities: liveCaptions,transcription,aiSummary,semanticSearch,meetingAnalytics,recordingModes; buildVersion

- **Ошибки:** 401

Источники: [internal/transport/http/platform_status_routes.go:25](../internal/transport/http/platform_status_routes.go#L25), [internal/app/platform/handler.go:44](../internal/app/platform/handler.go#L44).

### `GET /api/v1/admin/summary`

Операционная сводка администратора.

- **Доступ:** Bearer аккаунта; users.is_admin=true, проверка БД на каждом запросе

- **Запрос:** Без тела/query

- **Ответ:** 200: status,item: asOf,activeConferences,joinedParticipants,activeRecordings,queuedJobs,failed*24h,apiReady,mediaWorkerReady,dependencies,recentFailures

- **Ошибки:** 401; 403 не global admin; 503 зависимости

Источники: [internal/transport/http/platform_status_routes.go:27](../internal/transport/http/platform_status_routes.go#L27), [internal/app/platform/handler.go:53](../internal/app/platform/handler.go#L53).


<a id="api-group-18"></a>

## Телеметрия

### `POST /api/v1/client-errors`

Принять ограниченный отчет об ошибке браузера.

- **Доступ:** Публичный; CLIENT_TELEMETRY_ENABLED=true

- **Запрос:** Content-Type application/json; ≤8192 байт; JSON version*,route* из allowlist,code*=uncaught_error/unhandled_rejection/react_render_error,browser*=chromium/firefox/safari/edge/other,stack ≤5 {file,line,column}; file — имя сборочного JS asset

- **Ответ:** 202 без тела

- **Ошибки:** 404 выключено; 415 MIME; 413 размер; 400 JSON/allowlist; 429 30/IP и 300 глобально в минуту

Источники: [internal/transport/http/telemetry_routes.go:21](../internal/transport/http/telemetry_routes.go#L21), [internal/app/telemetry/handler.go:76](../internal/app/telemetry/handler.go#L76).


<a id="api-group-19"></a>

## API health

### `GET /health`

Простой health основного API; без проверки зависимостей.

- **Доступ:** Публичный

- **Запрос:** Без тела/query

- **Ответ:** 200: {status:"ok"}

- **Ошибки:** При общем drain middleware — 503

Источники: [internal/transport/http/router.go:47](../internal/transport/http/router.go#L47).


<a id="api-group-20"></a>

## WebSocket HTTP

### `POST /api/v1/conferences/:id/ws-ticket`

Получить билет подключения к конференции.

- **Доступ:** Bearer access token; гость только своей конференции; AuthorizeClient проверяет участие

- **Запрос:** id: ненулевой UUID. Необязательный JSON {clientId?: UUID}, максимум 1024 байта; пустое тело допустимо.

- **Ответ:** 201 {ticket:string,expiresAt:RFC3339}; Cache-Control:no-store. Одноразовый билет привязан к конференции; TTL=min(WS_TICKET_TTL,остаток срока access token).

- **Ошибки:** HTTP JSON {status:"failed",message}; 401 невалидная авторизация, 403 запрет, 404 отсутствующий ресурс, 409 конфликт, 422 невалидные параметры, 429 ограничение, 500 внутренняя ошибка, 503 недоступность, 504 тайм-аут — по возвращённой доменной ошибке. При 30 билетах/мин на пользователя: 429 {error:"too many connection tickets"}, Retry-After:60.

Источники: [internal/transport/websocket/handler.go:271](../internal/transport/websocket/handler.go#L271).

### `GET /api/v1/conferences/:id/ws`

Открыть канал конференции, сигнализации и media-команд.

- **Доступ:** Одноразовый ticket ИЛИ Bearer access token; активное участие/сессия; Origin проверяется

- **Запрос:** id: UUID; query только ticket (один параметр, raw query <=512 байт), либо Authorization без ticket. WebSocket Upgrade; поддерживается subprotocol go-recorder.v1.

- **Ответ:** 101 Switching Protocols; начальное conference.state; затем события и ответы JSON Envelope v1.

- **Ошибки:** HTTP JSON {status:"failed",message}; 401 невалидная авторизация, 403 запрет, 404 отсутствующий ресурс, 409 конфликт, 422 невалидные параметры, 429 ограничение, 500 внутренняя ошибка, 503 недоступность, 504 тайм-аут — по возвращённой доменной ошибке. 503 без тела при MaxConnections; ошибки Upgrade 400/403/405 по Gorilla. После Upgrade ошибки/закрытие уже WS.

Источники: [internal/transport/websocket/handler.go:379](../internal/transport/websocket/handler.go#L379).

### `GET /api/v1/webrtc/config`

Получить ICE/TURN конфигурацию клиента.

- **Доступ:** Bearer access token; разрешено гостям

- **Запрос:** Без тела и обязательных query.

- **Ответ:** 200 {iceServers:[{urls:string[],username?:string,credential?:string}],iceTransportPolicy?:string,expiresAt?:UnixSeconds}; Cache-Control:no-store.

- **Ошибки:** HTTP JSON {status:"failed",message}; 401 невалидная авторизация, 403 запрет, 404 отсутствующий ресурс, 409 конфликт, 422 невалидные параметры, 429 ограничение, 500 внутренняя ошибка, 503 недоступность, 504 тайм-аут — по возвращённой доменной ошибке.

Источники: [internal/transport/websocket/handler.go:338](../internal/transport/websocket/handler.go#L338).

### `POST /api/v1/ws-ticket`

Получить одноразовый билет глобального канала пользователя.

- **Доступ:** Bearer зарегистрированного аккаунта; Account и активная серверная сессия; гости запрещены

- **Запрос:** Тело не читается.

- **Ответ:** 201 {ticket:string,expiresAt:RFC3339}; Cache-Control:no-store. TTL=WS_TICKET_TTL; scope global.

- **Ошибки:** HTTP JSON {status:"failed",message}; 401 невалидная авторизация, 403 запрет, 404 отсутствующий ресурс, 409 конфликт, 422 невалидные параметры, 429 ограничение, 500 внутренняя ошибка, 503 недоступность, 504 тайм-аут — по возвращённой доменной ошибке. 429 без тела при 30 билетах/мин, Retry-After:60. Ошибка limiter:503.

Источники: [internal/transport/websocket/user_handler.go:199](../internal/transport/websocket/user_handler.go#L199).

### `GET /api/v1/ws`

Открыть глобальный канал диалогов, звонков, уведомлений и присутствия.

- **Доступ:** Только одноразовый global ticket; Authorization запрещён; Account и активная сессия; Origin проверяется

- **Запрос:** Ровно query ticket длиной 43 символа; raw query <=512 байт; WebSocket Upgrade.

- **Ответ:** 101; односторонние JSON Envelope v1 с conferenceId:"". Серверные Ping и клиентские Pong.

- **Ошибки:** HTTP JSON {status:"failed",message}; 401 невалидная авторизация, 403 запрет, 404 отсутствующий ресурс, 409 конфликт, 422 невалидные параметры, 429 ограничение, 500 внутренняя ошибка, 503 недоступность, 504 тайм-аут — по возвращённой доменной ошибке. 503 без тела при drain/MaxConnections; неверный query:401. Прикладное клиентское сообщение завершает соединение.

Источники: [internal/transport/websocket/user_handler.go:241](../internal/transport/websocket/user_handler.go#L241).


<a id="api-group-21"></a>

## Media worker

### `GET /healthz`

Готовность media worker.

- **Доступ:** Не требуется

- **Запрос:** Без тела.

- **Ответ:** 204 без тела при Ready.

- **Ошибки:** 503 без тела при неготовности.

Источники: [internal/transport/mediaworker/handler.go:382](../internal/transport/mediaworker/handler.go#L382).

### `GET /internal/media/metrics`

Снимок счётчиков SFU.

- **Доступ:** Authorization: Bearer <MEDIA_INTERNAL_SECRET>, точное значение; Cache-Control:no-store

- **Запрос:** Без тела.

- **Ответ:** 200 JSON Stats: roomsActive,mediaPeersActive,tracksPublished,subscriptions,peerConnectionTotal,peerConnectionFailures,relayConnections,directConnections,packetsForwarded,bytesForwarded,packetsDropped,recordingOutputs,recordingDrops,audioTaps,audioTapDrops.

- **Ошибки:** 401 {code:"media_unauthorized"}.

Источники: [internal/transport/mediaworker/handler.go:391](../internal/transport/mediaworker/handler.go#L391).

### `POST /internal/media/join`

Создать/получить peer и конфигурацию SFU.

- **Доступ:** Authorization: Bearer <MEDIA_INTERNAL_SECRET>, точное значение; Cache-Control:no-store

- **Запрос:** Строгий JSON Command <=1 MiB: requestId UUID совпадает с X-Request-ID; binding={conferenceId,participantId,sessionId,connectionId,userId:canonical UUID,authorizationExpiresAt:RFC3339}; route={workerId,endpoint,leaseId:canonical UUID}. ticket — подписанный media ticket для binding/route; policy?={version,microphoneBlocked,cameraBlocked,screenBlocked,kicked}.

- **Ответ:** 200 Result: mediaPeerId,workerId,maxPeers,iceServers?,iceTransportPolicy?,iceExpiresAt?,tracks?,videoCapture,policy?.

- **Ошибки:** JSON {code}; 400 invalid_media_signal (неизвестные ошибки также могут иметь code media_unavailable при HTTP 400), 401 media_unauthorized, 403 media_policy_blocked, 404 media_peer_not_found, 409 media_ownership_lost/media_negotiation_conflict/screen_sharing_conflict, 429 media_limit_exceeded, 503 media_unavailable. Набор конкретных ошибок зависит от операции/движка.

Источники: [internal/transport/mediaworker/handler.go:524](../internal/transport/mediaworker/handler.go#L524).

### `POST /internal/media/offer`

Применить SDP offer; повтор той же negotiationId с тем же SDP/publications возвращает сохранённый answer, с другим содержимым — конфликт.

- **Доступ:** Authorization: Bearer <MEDIA_INTERNAL_SECRET>, точное значение; Cache-Control:no-store

- **Запрос:** Строгий JSON Command <=1 MiB: requestId UUID совпадает с X-Request-ID; binding={conferenceId,participantId,sessionId,connectionId,userId:canonical UUID,authorizationExpiresAt:RFC3339}; route={workerId,endpoint,leaseId:canonical UUID}. mediaPeerId UUID; negotiationId UUID; sdp непустой <=128 KiB; publications:[{mid,source,trackId?}] или null; policy? .

- **Ответ:** 200 JSON Result: mediaPeerId и videoCapture:{maxWidth,maxHeight,maxFrameRate} всегда сериализуются; прочие поля omitempty. Дополнительно negotiationId,sdp ответа.

- **Ошибки:** JSON {code}; 400 invalid_media_signal (неизвестные ошибки также могут иметь code media_unavailable при HTTP 400), 401 media_unauthorized, 403 media_policy_blocked, 404 media_peer_not_found, 409 media_ownership_lost/media_negotiation_conflict/screen_sharing_conflict, 429 media_limit_exceeded, 503 media_unavailable. Набор конкретных ошибок зависит от операции/движка.

Источники: [internal/transport/mediaworker/handler.go:524](../internal/transport/mediaworker/handler.go#L524).

### `POST /internal/media/ready`

Подтвердить применение answer клиентом.

- **Доступ:** Authorization: Bearer <MEDIA_INTERNAL_SECRET>, точное значение; Cache-Control:no-store

- **Запрос:** Строгий JSON Command <=1 MiB: requestId UUID совпадает с X-Request-ID; binding={conferenceId,participantId,sessionId,connectionId,userId:canonical UUID,authorizationExpiresAt:RFC3339}; route={workerId,endpoint,leaseId:canonical UUID}. mediaPeerId,negotiationId UUID; sdp,candidate,trackId должны отсутствовать/быть пустыми.

- **Ответ:** 200 JSON Result: mediaPeerId и videoCapture:{maxWidth,maxHeight,maxFrameRate} всегда сериализуются; прочие поля omitempty.

- **Ошибки:** JSON {code}; 400 invalid_media_signal (неизвестные ошибки также могут иметь code media_unavailable при HTTP 400), 401 media_unauthorized, 403 media_policy_blocked, 404 media_peer_not_found, 409 media_ownership_lost/media_negotiation_conflict/screen_sharing_conflict, 429 media_limit_exceeded, 503 media_unavailable. Набор конкретных ошибок зависит от операции/движка.

Источники: [internal/transport/mediaworker/handler.go:524](../internal/transport/mediaworker/handler.go#L524).

### `POST /internal/media/ice`

Добавить ICE candidate либо завершить сбор кандидатов.

- **Доступ:** Authorization: Bearer <MEDIA_INTERNAL_SECRET>, точное значение; Cache-Control:no-store

- **Запрос:** Строгий JSON Command <=1 MiB: requestId UUID совпадает с X-Request-ID; binding={conferenceId,participantId,sessionId,connectionId,userId:canonical UUID,authorizationExpiresAt:RFC3339}; route={workerId,endpoint,leaseId:canonical UUID}. mediaPeerId UUID; candidate: объект ICECandidateInit либо null, <=4096 байт.

- **Ответ:** 200 JSON Result: mediaPeerId и videoCapture:{maxWidth,maxHeight,maxFrameRate} всегда сериализуются; прочие поля omitempty.

- **Ошибки:** JSON {code}; 400 invalid_media_signal (неизвестные ошибки также могут иметь code media_unavailable при HTTP 400), 401 media_unauthorized, 403 media_policy_blocked, 404 media_peer_not_found, 409 media_ownership_lost/media_negotiation_conflict/screen_sharing_conflict, 429 media_limit_exceeded, 503 media_unavailable. Набор конкретных ошибок зависит от операции/движка.

Источники: [internal/transport/mediaworker/handler.go:524](../internal/transport/mediaworker/handler.go#L524).

### `POST /internal/media/leave`

Удалить peer. Допускается cleanup после потери readiness/lease/срока сессии; чужая binding запрещена.

- **Доступ:** Authorization: Bearer <MEDIA_INTERNAL_SECRET>, точное значение; Cache-Control:no-store

- **Запрос:** Строгий JSON Command <=1 MiB: requestId UUID совпадает с X-Request-ID; binding={conferenceId,participantId,sessionId,connectionId,userId:canonical UUID,authorizationExpiresAt:RFC3339}; route={workerId,endpoint,leaseId:canonical UUID}. mediaPeerId UUID.

- **Ответ:** 200 JSON Result: mediaPeerId и videoCapture:{maxWidth,maxHeight,maxFrameRate} всегда сериализуются; прочие поля omitempty.

- **Ошибки:** JSON {code}; 400 invalid_media_signal (неизвестные ошибки также могут иметь code media_unavailable при HTTP 400), 401 media_unauthorized, 403 media_policy_blocked, 404 media_peer_not_found, 409 media_ownership_lost/media_negotiation_conflict/screen_sharing_conflict, 429 media_limit_exceeded, 503 media_unavailable. Набор конкретных ошибок зависит от операции/движка.

Источники: [internal/transport/mediaworker/handler.go:524](../internal/transport/mediaworker/handler.go#L524).

### `POST /internal/media/unpublish`

Снять опубликованный серверный track.

- **Доступ:** Authorization: Bearer <MEDIA_INTERNAL_SECRET>, точное значение; Cache-Control:no-store

- **Запрос:** Строгий JSON Command <=1 MiB: requestId UUID совпадает с X-Request-ID; binding={conferenceId,participantId,sessionId,connectionId,userId:canonical UUID,authorizationExpiresAt:RFC3339}; route={workerId,endpoint,leaseId:canonical UUID}. mediaPeerId UUID; trackId canonical UUID.

- **Ответ:** 200 JSON Result: mediaPeerId и videoCapture:{maxWidth,maxHeight,maxFrameRate} всегда сериализуются; прочие поля omitempty.

- **Ошибки:** JSON {code}; 400 invalid_media_signal (неизвестные ошибки также могут иметь code media_unavailable при HTTP 400), 401 media_unauthorized, 403 media_policy_blocked, 404 media_peer_not_found, 409 media_ownership_lost/media_negotiation_conflict/screen_sharing_conflict, 429 media_limit_exceeded, 503 media_unavailable. Набор конкретных ошибок зависит от операции/движка.

Источники: [internal/transport/mediaworker/handler.go:524](../internal/transport/mediaworker/handler.go#L524).

### `POST /internal/media/policy`

Обновить ограничения источников участника.

- **Доступ:** Authorization: Bearer <MEDIA_INTERNAL_SECRET>, точное значение; Cache-Control:no-store

- **Запрос:** Строгий Command <=1 MiB; requestId UUID = X-Request-ID; conferenceId UUID; route={workerId,endpoint,leaseId UUID}; mediaPeerId,sdp,candidate,ticket запрещены. participantId UUID; policy с version>=0 и булевыми блокировками.

- **Ответ:** 200 Result с mediaPeerId:"" и videoCapture с нулевыми значениями.

- **Ошибки:** JSON {code}; 400 invalid_media_signal (неизвестные ошибки также могут иметь code media_unavailable при HTTP 400), 401 media_unauthorized, 403 media_policy_blocked, 404 media_peer_not_found, 409 media_ownership_lost/media_negotiation_conflict/screen_sharing_conflict, 429 media_limit_exceeded, 503 media_unavailable. Набор конкретных ошибок зависит от операции/движка.

Источники: [internal/transport/mediaworker/control_egress.go:21](../internal/transport/mediaworker/control_egress.go#L21).

### `POST /internal/media/close`

Закрыть медиакомнату.

- **Доступ:** Authorization: Bearer <MEDIA_INTERNAL_SECRET>, точное значение; Cache-Control:no-store

- **Запрос:** Строгий Command <=1 MiB; requestId UUID = X-Request-ID; conferenceId UUID; route={workerId,endpoint,leaseId UUID}; mediaPeerId,sdp,candidate,ticket запрещены. Дополнительные поля не требуются.

- **Ответ:** 200 Result с mediaPeerId:"" и videoCapture с нулевыми значениями.

- **Ошибки:** JSON {code}; 400 invalid_media_signal (неизвестные ошибки также могут иметь code media_unavailable при HTTP 400), 401 media_unauthorized, 403 media_policy_blocked, 404 media_peer_not_found, 409 media_ownership_lost/media_negotiation_conflict/screen_sharing_conflict, 429 media_limit_exceeded, 503 media_unavailable. Набор конкретных ошибок зависит от операции/движка.

Источники: [internal/transport/mediaworker/control_egress.go:21](../internal/transport/mediaworker/control_egress.go#L21).

### `POST /internal/media/egress`

Пассивно получать RTP для записи либо только аудио для captions; не WebSocket.

- **Доступ:** Authorization: Bearer <MEDIA_INTERNAL_SECRET>, точное значение; Cache-Control:no-store

- **Запрос:** Строгий JSON <=4096 байт: {requestId:UUID,conferenceId:UUID,recordingId:UUID,route:{workerId,endpoint,leaseId:UUID},segmentDurationSec:1..30,purpose?:"recording"|"captions"}; requestId=X-Request-ID. Пустой purpose означает запись.

- **Ответ:** 200 поток application/x-ndjson; X-Request-ID. Frame={type,sequence,capturedAt:UnixNano,track?:EgressTrack,trackId?:UUID,rtp?:base64,code?:string}; type: hello,track,rtp,track.end,ping,error. EgressTrack=Track+mimeType,clockRate,channels,payloadType,ssrc.

- **Ошибки:** JSON {code}; 400 invalid_media_signal (неизвестные ошибки также могут иметь code media_unavailable при HTTP 400), 401 media_unauthorized, 403 media_policy_blocked, 404 media_peer_not_found, 409 media_ownership_lost/media_negotiation_conflict/screen_sharing_conflict, 429 media_limit_exceeded, 503 media_unavailable. Набор конкретных ошибок зависит от операции/движка. После начала потока возможен frame {type:"error",code:"recording_egress_failed",capturedAt,...}; затем поток закрывается.

Источники: [internal/transport/mediaworker/control_egress.go:86](../internal/transport/mediaworker/control_egress.go#L86).


<a id="api-group-22"></a>

## Operations

### `GET /version`

Информация о сборке; общий маршрут сервисов.

- **Доступ:** Не требуется

- **Запрос:** Без тела.

- **Ответ:** 200 {version,commit,buildTime}; dev/unknown при отсутствии build metadata; Cache-Control:no-store; X-Content-Type-Options:nosniff.

- **Ошибки:** Специальных ошибок handler нет.

Источники: [internal/operations/runtime.go:258](../internal/operations/runtime.go#L258).

### `GET /health/live`

Проверка живости процесса.

- **Доступ:** Не требуется

- **Запрос:** Без тела.

- **Ответ:** 200 {status:"alive"}; Cache-Control:no-store.

- **Ошибки:** Специальных ошибок handler нет.

Источники: [internal/operations/runtime.go:195](../internal/operations/runtime.go#L195).

### `GET /health/ready`

Готовность зависимостей и процесса.

- **Доступ:** Не требуется

- **Запрос:** Без тела.

- **Ответ:** 200 {status:"ready"}; Cache-Control:no-store.

- **Ошибки:** 503 {status:"not_ready"}, включая drain.

Источники: [internal/operations/runtime.go:203](../internal/operations/runtime.go#L203).

### `GET /operations/drain`

Запросить статус graceful drain.

- **Доступ:** Authorization: Bearer <METRICS_SECRET>, секрет минимум 32 байта

- **Запрос:** Без тела.

- **Ответ:** 200 {draining:boolean,active:number,ready:boolean}; Cache-Control:no-store.

- **Ошибки:** 404 text/plain при неверном/отсутствующем секрете.

Источники: [internal/operations/drain.go:24](../internal/operations/drain.go#L24).

### `POST /operations/drain`

Включить drain, снять readiness и вызвать service hook; вернуть статус.

- **Доступ:** Authorization: Bearer <METRICS_SECRET>, секрет минимум 32 байта

- **Запрос:** Без тела.

- **Ответ:** 200 {draining:boolean,active:number,ready:boolean}; Cache-Control:no-store.

- **Ошибки:** 404 text/plain при неверном/отсутствующем секрете.

Источники: [internal/operations/drain.go:24](../internal/operations/drain.go#L24).

### `GET /metrics`

Метрики процесса/приложения; не JSON.

- **Доступ:** Authorization: Bearer <METRICS_SECRET>, секрет минимум 32 байта

- **Запрос:** Без тела; HTTP negotiation формата Prometheus.

- **Ответ:** 200 метрики Prometheus text/openmetrics.

- **Ошибки:** 404 text/plain без секрета; promhttp ограничивает 2 параллельных запроса и 5 s, возможен 503 при перегрузке/тайм-ауте, 500 при ошибке сбора.

Источники: [internal/operations/runtime.go:245](../internal/operations/runtime.go#L245).


<a id="api-group-23"></a>

## Recorder worker

### `* /health`

Legacy health recorder-worker.

- **Доступ:** Не требуется

- **Запрос:** Метод не ограничен; тело не читается.

- **Ответ:** 200 {status:"ok"}.

- **Ошибки:** Специальных ошибок нет; readiness не проверяется.

Источники: [internal/app/worker.go:201](../internal/app/worker.go#L201).

### `POST /records/:id/start`

Передать legacy record.start; устаревшие переходы идемпотентны.

- **Доступ:** Authorization: Bearer <WORKER_INTERNAL_SECRET>, секрет минимум 32 байта; worker Ready

- **Запрос:** id: существующая legacy recording, не composite. JSON Command: reason?:string,segmentDurationSec?:int; type и recordId из тела игнорируются и заменяются маршрутом. Пустое тело не допускается. Ограничение HTTP_MAX_BODY_BYTES.

- **Ответ:** 200 {status:"success"}.

- **Ошибки:** 404 text/plain: auth, неправильный метод/путь, отсутствующая/composite запись; 503 "worker unavailable"; 400 text/plain при Decode или ошибке HandleCommand.

Источники: [internal/app/worker.go:269](../internal/app/worker.go#L269).

### `POST /records/:id/stop`

Передать legacy record.stop; устаревшие переходы идемпотентны.

- **Доступ:** Authorization: Bearer <WORKER_INTERNAL_SECRET>, секрет минимум 32 байта; worker Ready

- **Запрос:** id: существующая legacy recording, не composite. JSON Command: reason?:string,segmentDurationSec?:int; type и recordId из тела игнорируются и заменяются маршрутом. Пустое тело не допускается. Ограничение HTTP_MAX_BODY_BYTES.

- **Ответ:** 200 {status:"success"}.

- **Ошибки:** 404 text/plain: auth, неправильный метод/путь, отсутствующая/composite запись; 503 "worker unavailable"; 400 text/plain при Decode или ошибке HandleCommand.

Источники: [internal/app/worker.go:269](../internal/app/worker.go#L269).

### `POST /records/:id/webrtc/offer`

Передать SDP legacy WebRTC ingest. GET/download в этом /records/ handler не реализован.

- **Доступ:** Authorization: Bearer <WORKER_INTERNAL_SECRET>, секрет минимум 32 байта; worker Ready

- **Запрос:** id: существующая legacy запись; JSON {type:"offer",sdp:string}; ограничение HTTP_MAX_BODY_BYTES.

- **Ответ:** 200 {type:"answer",sdp:string}.

- **Ошибки:** 404 text/plain: auth/запись/composite/метод; 503 "worker unavailable"; 400 text/plain при JSON/ingest ошибке.

Источники: [internal/app/worker.go:347](../internal/app/worker.go#L347).


<a id="api-group-24"></a>

## Profiling (отдельный listener)

### `* /debug/pprof/`

Одна из 7 явных регистраций profiling. /debug/pprof/ — также prefix fallback для имён runtime profiles.

- **Доступ:** Без token; только 127.0.0.1:PPROF_PORT, отключено при порте 0

- **Запрос:** Маршрутизатор не ограничивает метод; обычно GET. Необязательный seconds должен быть числом 0<seconds<=60. Остальные параметры стандартного net/http/pprof.

- **Ответ:** 200 HTML index для /; для профиля — стандартный текст/бинарный профиль; для trace — trace.

- **Ошибки:** 400 text/plain при недопустимом seconds; стандартные ошибки pprof (например, неизвестный профиль 404).

Источники: [internal/operations/runtime.go:477](../internal/operations/runtime.go#L477).

### `* /debug/pprof/profile`

Одна из 7 явных регистраций profiling. /debug/pprof/ — также prefix fallback для имён runtime profiles.

- **Доступ:** Без token; только 127.0.0.1:PPROF_PORT, отключено при порте 0

- **Запрос:** Маршрутизатор не ограничивает метод; обычно GET. Необязательный seconds должен быть числом 0<seconds<=60. Остальные параметры стандартного net/http/pprof.

- **Ответ:** 200 HTML index для /; для профиля — стандартный текст/бинарный профиль; для trace — trace.

- **Ошибки:** 400 text/plain при недопустимом seconds; стандартные ошибки pprof (например, неизвестный профиль 404).

Источники: [internal/operations/runtime.go:477](../internal/operations/runtime.go#L477).

### `* /debug/pprof/trace`

Одна из 7 явных регистраций profiling. /debug/pprof/ — также prefix fallback для имён runtime profiles.

- **Доступ:** Без token; только 127.0.0.1:PPROF_PORT, отключено при порте 0

- **Запрос:** Маршрутизатор не ограничивает метод; обычно GET. Необязательный seconds должен быть числом 0<seconds<=60. Остальные параметры стандартного net/http/pprof.

- **Ответ:** 200 HTML index для /; для профиля — стандартный текст/бинарный профиль; для trace — trace.

- **Ошибки:** 400 text/plain при недопустимом seconds; стандартные ошибки pprof (например, неизвестный профиль 404).

Источники: [internal/operations/runtime.go:477](../internal/operations/runtime.go#L477).

### `* /debug/pprof/heap`

Одна из 7 явных регистраций profiling. /debug/pprof/ — также prefix fallback для имён runtime profiles.

- **Доступ:** Без token; только 127.0.0.1:PPROF_PORT, отключено при порте 0

- **Запрос:** Маршрутизатор не ограничивает метод; обычно GET. Необязательный seconds должен быть числом 0<seconds<=60. Остальные параметры стандартного net/http/pprof.

- **Ответ:** 200 HTML index для /; для профиля — стандартный текст/бинарный профиль; для trace — trace.

- **Ошибки:** 400 text/plain при недопустимом seconds; стандартные ошибки pprof (например, неизвестный профиль 404).

Источники: [internal/operations/runtime.go:477](../internal/operations/runtime.go#L477).

### `* /debug/pprof/goroutine`

Одна из 7 явных регистраций profiling. /debug/pprof/ — также prefix fallback для имён runtime profiles.

- **Доступ:** Без token; только 127.0.0.1:PPROF_PORT, отключено при порте 0

- **Запрос:** Маршрутизатор не ограничивает метод; обычно GET. Необязательный seconds должен быть числом 0<seconds<=60. Остальные параметры стандартного net/http/pprof.

- **Ответ:** 200 HTML index для /; для профиля — стандартный текст/бинарный профиль; для trace — trace.

- **Ошибки:** 400 text/plain при недопустимом seconds; стандартные ошибки pprof (например, неизвестный профиль 404).

Источники: [internal/operations/runtime.go:477](../internal/operations/runtime.go#L477).

### `* /debug/pprof/mutex`

Одна из 7 явных регистраций profiling. /debug/pprof/ — также prefix fallback для имён runtime profiles.

- **Доступ:** Без token; только 127.0.0.1:PPROF_PORT, отключено при порте 0

- **Запрос:** Маршрутизатор не ограничивает метод; обычно GET. Необязательный seconds должен быть числом 0<seconds<=60. Остальные параметры стандартного net/http/pprof.

- **Ответ:** 200 HTML index для /; для профиля — стандартный текст/бинарный профиль; для trace — trace.

- **Ошибки:** 400 text/plain при недопустимом seconds; стандартные ошибки pprof (например, неизвестный профиль 404).

Источники: [internal/operations/runtime.go:477](../internal/operations/runtime.go#L477).

### `* /debug/pprof/block`

Одна из 7 явных регистраций profiling. /debug/pprof/ — также prefix fallback для имён runtime profiles.

- **Доступ:** Без token; только 127.0.0.1:PPROF_PORT, отключено при порте 0

- **Запрос:** Маршрутизатор не ограничивает метод; обычно GET. Необязательный seconds должен быть числом 0<seconds<=60. Остальные параметры стандартного net/http/pprof.

- **Ответ:** 200 HTML index для /; для профиля — стандартный текст/бинарный профиль; для trace — trace.

- **Ошибки:** 400 text/plain при недопустимом seconds; стандартные ошибки pprof (например, неизвестный профиль 404).

Источники: [internal/operations/runtime.go:477](../internal/operations/runtime.go#L477).

