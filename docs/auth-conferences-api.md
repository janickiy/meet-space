# Аккаунты, авторизация и конференции

Все пути в таблицах имеют префикс `/api/v1`.
Полный каталог — [API_REFERENCE.md](API_REFERENCE.md).

## Настройка и общий контракт

`JWT_SECRET` обязателен для API: минимум 32 байта. Его смена делает старые JWT
невалидными. Не помещайте секрет в Git или `VITE_*`. Для локального старта,
внешней Docker-сети и согласования HTTP/HTTPS используйте [README](../README.md).
API применяет миграции при `AUTO_MIGRATE=true` (локальное значение по умолчанию);
production использует отдельную миграцию до запуска обновлённого API.

`HTTP_TRUSTED_PROXIES` содержит только реальные IP/CIDR доверенных прокси.
По умолчанию forwarded IP не доверяется. Login/register ограничены Redis:
10/5 запросов в минуту на IP по умолчанию; `429` — превышение,
`500 rate limit unavailable` — недоступный limiter.

JSON использует camelCase. Идентификатор действующего пользователя берётся из
Bearer JWT. Неизвестные поля и несколько JSON-документов отвергаются;
обычный JSON decoder ограничивает тело 32 КиБ. У отдельных upload/telemetry
маршрутов собственные лимиты. Браузерные auth-операции проверяют точное совпадение
Origin с `PUBLIC_FRONTEND_URL` (включая схему и порт), отклоняют `Sec-Fetch-Site:
cross-site` и требуют `application/json` для непустого тела. CLI без Origin и
Fetch Metadata разрешён. Cookie не заменяет Bearer на остальных REST-маршрутах.

## Пользователь и сессия

Email обрезается по краям, приводится к нижнему регистру, допускает один адрес
без display name и не более 254 байт. Пароль — 8–128 Unicode-символов, без trim;
обязательных цифр или спецсимволов нет. `displayName` при регистрации необязателен,
после trim ограничен 100 символами. Пустое имя хранится как NULL; имя участника
по умолчанию — `Participant`. Изменение профиля требует непустое имя 1–100 символов.

Пароль хранится как Argon2id PHC hash: соль 16 байт, память 64 МиБ, 3 прохода,
4 потока, результат 32 байта. Unknown email и wrong password дают одинаковый
`401 invalid email or password`. DTO пользователя содержит `id`, `email`,
`displayName`, `isAdmin`, `createdAt`, `updatedAt`, необязательный
`guestConferenceId`; пароль, hash и refresh secret не возвращаются.

JWT HS256 имеет TTL 3600 секунд, `sub`, `iat`, `exp`, issuer `go-recorder`,
audience `go-recorder-api`; у постоянной сессии есть `sid`. API дополнительно
проверяет существование и отзыв SID в PostgreSQL. HttpOnly cookie позволяет
автоматически обновлять JWT без часового выхода; серверная сессия не имеет
тайм-аута простоя. Подробности cookie, reconnect и нескольких аккаунтов —
[PERSISTENT_AUTH.md](PERSISTENT_AUTH.md), [account-management.md](account-management.md).
Регистрация возвращает профиль; для начала сессии требуется login.

## API аккаунтов

| Метод и путь | Доступ и запрос | Ответ и значимые ошибки |
| --- | --- | --- |
| `POST /auth/register` | Публичный; same-origin.<br>JSON: email*; password* 8–128 символов; displayName ≤100, необязателен | 201: status, user.<br>409 email занят; 422 поля; 403 origin; 415 Content-Type |
| `POST /auth/login` | Публичный; same-origin.<br>JSON: email*, password* | 200: status, accessToken, tokenType, expiresIn, user; Set-Cookie.<br>401 неверные данные; 409 лимит 5 аккаунтов в браузере; 403 origin; 415 Content-Type |
| `POST /auth/refresh` | Cookie; same-origin.<br>Пусто или JSON {userId}; meetrix_session либо meetrix_account_<userId> | 200: LoginResponse; обновленные cookie.<br>401 отсутствует/отозвана сессия; 403 другой аккаунт/origin; 503 сессии выключены |
| `POST /auth/accounts/list` | Cookie; same-origin.<br>Пусто или {}; cookies meetrix_account_<id> | 200: status, items: User[]; недействительные cookies удаляются.<br>409 лимит аккаунтов; 503 сессии выключены; 403 origin |
| `POST /auth/accounts/switch` | Cookie; same-origin.<br>JSON: userId* канонический UUID; его meetrix_account_<id> | 200: LoginResponse; Set-Cookie meetrix_session.<br>401 cookie недействителен; 403 несовпадение владельца; 422 UUID; 503 функция выключена |
| `POST /auth/logout` | Cookie; same-origin; Bearer необязателен.<br>Пусто или JSON {userId}; cookie сеанса; опциональный Bearer для привязки доступа | 200: status=success; удаление соответствующих cookies.<br>403 origin; 400 JSON; 503 нет session service |
| `GET /auth/me` | Bearer; гость допустим в своей области.<br>Без тела и query | 200: status, user; у гостя email пустой, guestConferenceId.<br>401 отсутствует аккаунт/сессия |
| `PATCH /auth/me` | Bearer аккаунта.<br>JSON: displayName* 1–100 символов без управляющих символов | 200: status, user.<br>422 имя; 403 гостевой scope |
| `DELETE /auth/me` | Bearer аккаунта; same-origin.<br>JSON: password* | 200: status=success; удаление cookies аккаунта.<br>422 неверный/пустой пароль; 403 origin/гость; 503 нет account service |
| `POST /auth/session` | Bearer аккаунта; same-origin.<br>Пусто или {userId}; используется аккаунт Bearer, его cookie либо meetrix_session | 200: LoginResponse; cookies.<br>401 недействительный доступ; 403 гость/origin; 503 сессии выключены |

`LoginResponse` — `{status,accessToken,tokenType:"Bearer",expiresIn,user}`.
Выход возвращает `{status:"success"}`, без обязательного поля `message`.
Удаление аккаунта также может вернуть `409`, если остаются собственные группы,
незавершённые встречи или активное участие в звонке/конференции. Оно требует
пароль текущего аккаунта и отзывает все его сессии; общая история сохраняется
по правилам [удаления аккаунтов](account-management.md).

## API конференций и приглашений

| Метод и путь | Доступ и запрос | Ответ и значимые ошибки |
| --- | --- | --- |
| `POST /conferences` | Bearer аккаунта.<br>JSON: title* 1–200; waitingRoomEnabled; scheduledAt RFC3339 в будущем; plannedDurationMin 1–1440 только с scheduledAt | 201: status, item: Conference.<br>422 название/расписание; 403 гость |
| `GET /conferences` | Bearer аккаунта.<br>Query: limit 1–100 (20), offset ≥0 (0) | 200: status, items: Conference[].<br>422 pagination; 403 гость |
| `GET /conferences/:id` | Bearer; доступ к истории конференции.<br>id UUID | 200: status, item: Conference.<br>403 нет допуска; 404 конференция |
| `POST /conferences/:id/start` | Bearer; owner конференции.<br>id; пусто или {} | 200: status, item с status=active.<br>403 чужой owner; 409 недопустимый переход; 404 |
| `POST /conferences/:id/finish` | Bearer; owner конференции.<br>id; пусто или {} | 200: status, item с status=finished.<br>403; 409 если не active; 404 |
| `POST /conferences/:id/cancel` | Bearer; owner конференции.<br>id; пусто или {} | 200: status, item с status=cancelled.<br>403; 409 недопустимый переход; 404 |
| `POST /conferences/:id/join` | Bearer в разрешенной области.<br>id; пусто либо JSON {inviteCode}; для нового членства нужен действующий inviteCode | 200: status, item: Participant, включая admissionState.<br>403 неверное приглашение/rejected/kicked; 409 состояние конференции; 422 code |
| `POST /conferences/:id/leave` | Bearer; собственное членство.<br>id; пусто или {} | 200: status, item: Participant.<br>403 нет членства; 404 |
| `GET /conferences/:id/participants` | Bearer; доступ к истории.<br>id; limit 1–100 (20), offset ≥0 | 200: status, items: Participant[].<br>403 нет допуска; 404; 422 pagination |
| `GET /conferences/:id/participants/me` | Bearer; собственное членство.<br>id | 200: status, item: Participant.<br>403/404 нет доступного членства |
| `POST /conferences/:id/participants/:participantId/admission` | Bearer; вошедший owner/co_host.<br>id, participantId UUID; JSON decision*=admit/reject | 200: status, item: Participant.<br>403 роль; 409 состояние допуска/конференции; 422 decision; 404 |
| `PUT /conferences/:id/schedule` | Bearer; owner.<br>id; JSON scheduledAt* RFC3339 в будущем; plannedDurationMin 1–1440 | 200: status, item: Conference.<br>403 не owner; 409 конференция не scheduled; 422 время/длительность; 404 |
| `GET /conferences/:id/history` | Bearer; доступ к истории.<br>id | 200: status, item: conference, owner, durationSec, participantCount, participants, participantsTruncated, recordings, chatAvailable, chatReadOnly.<br>403; 409 еще не завершена; 404 |
| `GET /me/conferences` | Bearer аккаунта.<br>Query view=upcoming/active/past (upcoming), scope=all/owned/participating (all), status, from/to RFC3339, cursor, limit 1–100 (20) | 200: status, items: Conference[], nextCursor.<br>422 фильтры/курсор; 403 гость |
| `GET /conference-invites/:code` | Публичный.<br>code приглашения | 200: status, item: id,title,status,waitingRoomEnabled,scheduledAt.<br>404 неизвестный/недоступный code |
| `POST /conference-invites/:code/join` | Bearer аккаунта.<br>code; пусто или {} | 200: status, item: Participant.<br>403 запрет повторного входа; 404 code; 409 состояние |
| `PUT /conferences/:id/participants/me/media` | Bearer; собственное вошедшее членство.<br>id; JSON connectionId* UUID подключенной сессии, sequence* целое 1–9007199254740991; microphoneEnabled,cameraEnabled,screenSharing bool | 200: status,item: Participant; старый/повторный sequence игнорируется.<br>403 нет допуска/подключения/источник заблокирован; 409 комната не active/created; 422 поля |
| `POST /conferences/:id/participants/:participantId/moderation` | Bearer; owner; co_host ограниченно.<br>id,participantId; JSON action*=mute/camera/screen/kick/role; blocked* для медиа; role*=co_host/participant для role | 200: status, item: Participant.<br>403 запрещенная цель/роль; 422 несовместимые поля; 404 |

| Метод и путь | Доступ и запрос | Ответ и значимые ошибки |
| --- | --- | --- |
| `POST /conference-invites/:code/guest` | Публичный; при возобновлении гостевой Bearer.<br>code; JSON displayName*; Authorization необязателен и должен содержать гостевой токен | 200: status,accessToken,tokenType=Bearer,expiresIn=3600,user,item: Participant.<br>401 ошибочный токен; 403 обычный аккаунт/другая конференция/отклонен; 404 code; 409 завершена; 422 имя; 429 |
| `GET /conferences/:id/invitation-users` | Аккаунт; owner либо вошедший co_host в active конференции.<br>id; query* 2–100 символов | 200: status,items: id,email,displayName.<br>422 query; 403 роль; 409 конференция завершена; 404 |
| `POST /conferences/:id/invitations` | Аккаунт; owner либо вошедший co_host в active конференции.<br>id; JSON emails[] и/или userIds[] UUID; всего 1–20 адресатов | 202: status,items: InvitationResult[] (id,email,userId,status).<br>422 список/email; 403 роль; 409 конференция закрыта; 404 |

`GET /conferences` — список сохранённых членств с offset pagination, а не список
живых подключений. Для вкладок интерфейса используется `GET /me/conferences`
с `view=upcoming|active|past` и cursor pagination. История доступна по
`GET /conferences/:id/history`, а не `/history/:id`. Сводка истории возвращается
только после завершения/отмены. `invitation-users` — поиск адресатов организатором;
его email-проекция отличается от безопасного поиска собеседников `GET /users`.

### DTO

Conference: `id`, `ownerId`, `title`, `inviteCode`, `inviteUrl`, `status`,
`createdAt`, `updatedAt`, nullable `startedAt`/`finishedAt`, `waitingRoomEnabled`,
`scheduledAt`, `plannedDurationMin`. Название после trim — 1–200 символов.
`inviteUrl` — относительный backend lookup `/api/v1/conference-invites/:code`;
страница браузера — `/i/:code`. Секрет приглашения: 24 случайных байта,
base64url без padding (32 символа); публичный lookup раскрывает только
`id,title,status,waitingRoomEnabled,scheduledAt`.

Participant: `id`, `conferenceId`, `userId`, `displayName`, `role`, `status`,
`joinedAt`, `leftAt`, `createdAt`, `updatedAt`, `admissionState`,
`admissionDecidedAt`, `admissionVersion`, флаги `microphoneEnabled`,
`cameraEnabled`, `screenSharing`, `microphoneBlocked`, `cameraBlocked`,
`screenBlocked` и `mediaPolicyVersion`. Членство не равно WebRTC-присутствию;
физические подключения отслеживаются отдельно.

### Жизненный цикл и права

- `created|scheduled → active → finished`; отмена разрешена из `created|scheduled`.
  Lifecycle меняет только owner, даже если он ещё не joined или уже left.
  Owner membership создаётся как `left/admitted` с пустыми датами подключения.
- `scheduledAt` — будущий RFC3339 timestamp с часовым поясом; длительность
  1–1440 минут возможна только вместе с расписанием. Изменить время можно
  только в `scheduled`. Автоматического начала/окончания по времени нет.
- Новый зарегистрированный участник входит только с действующим кодом.
  Приглашение сразу даёт `admitted`, в том числе прежнему `waiting`, независимо
  от `waitingRoomEnabled`. В `scheduled` аккаунт получает/сохраняет членство
  без подключения (`left`); после старта входит во встречу.
- Join без кода разрешён существующему членству и сохраняет прежний допуск.
  `rejected`/`kicked` не обходятся повторным приглашением. Администратор приложения
  не получает автоматически полномочия организатора конкретной встречи.
- Повтор joined/leave идемпотентен. Обычный выход сохраняет разрешённую историю;
  завершение закрывает joined-членства и запускает остановку активной записи.
- Чтение истории требует `admitted` и `joined|left`. Для живого участия требуется
  `joined`. Ожидающие не получают медиакомнату и приватную историю; состояние
  своего членства читают через `/participants/me`.
- Admission управляют вошедшие owner/co_host. Для модерации co_host ограничен
  действиями mute/screen/kick против обычных участников; назначение ролей и
  camera control принадлежат owner. Ни одна команда не включает физическое
  устройство удалённо: снятие запрета разрешает участнику включить его самому.

### Гостевой вход

`/i/:code` открывает prejoin без создания участника. На публичном экране приглашения браузер запрашивает камеру и микрофон
согласно сохранённым настройкам; пользователь может отказать или выключить их. Guest API
вызывается после «Подключиться». Гость получает обычный user UUID с
`guestConferenceId`, JWT на один час и роль `guest`, но не постоянную cookie-сессию.
Повтор `/guest` с гостевым Bearer той же встречи сохраняет participant ID и допуск;
новое посещение без прежней сессии создаёт новую гостевую идентичность.

Гость не входит в ещё запланированную или закрытую встречу. Разрешены только API
его встречи и `/auth/me`, `/auth/logout`, `/capabilities`, `/webrtc/config`.
Создание встреч, личные/групповые чаты, папки, звонки и admin закрыты.
Начать/остановить/удалить запись гость не может. Приглашение не отменяет
исключение/отказ текущей гостевой идентичности.

## Ошибки и совместимость

Обычная ошибка: `{status:"failed",message:"безопасное описание"}`.
`400` — некорректный JSON; `401` — токен/сессия/логин; `403` — права или Origin;
`404` — недоступный ресурс; `409` — конфликт состояния/повтора; `415` — MIME;
`422` — поля/UUID/пагинация; `429` — лимит; `500/503` — отказ обработки/зависимости.
Конкретные права проверяются заново сервером; UUID ресурса их не предоставляет.

Публичные `/api/v1/records*`, `/api/records*` и debug recorder API удалены.
Используйте `/api/v1/conferences/:id/recordings*`. Старые записи не получают
фиктивных владельцев; ранее выданные signed URLs действуют до исходного срока.
Таблицы сессий добавлены миграцией 26, guest scope — 23; текущая схема также
содержит изменения чатов, звонков, групп и папок. Откат отдельных ранних миграций
не является безопасным способом отката текущего приложения.

## Исходный код

Основные файлы: [platform_routes.go](../internal/transport/http/platform_routes.go),
[auth handlers](../internal/app/auth), [conference handlers](../internal/app/conferences),
[модели](../internal/domain/conferences),
[транзакции членства](../internal/infrastructure/postgres/conference_repository.go).
