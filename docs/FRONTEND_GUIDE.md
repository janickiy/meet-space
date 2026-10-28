# MeetSpace: устройство и запуск frontend

## Архитектура

Frontend находится в `frontend/`. Стек: React, TypeScript, Vite, React Router,
TanStack Query, Lucide и локальные шрифты Inter. Точные версии закреплены в
`frontend/package-lock.json`; команды и поддержанные версии Node заданы в
`frontend/package.json`.

`main.tsx` устанавливает провайдеры авторизации, QueryClient, Router и общий
ErrorBoundary. `App.tsx` содержит маршруты и ленивые импорты страниц.
`components/Layout.tsx` отвечает за общую навигацию, профиль и
уведомления; создаёт account WebSocket личных сообщений через
`usePersonalRealtime`, не создавая медиасоединение.

`ConferencePage` выбирает `ActiveConferenceRoom` либо `ConferenceOverview`;
общие данные, команды и состояние панелей находятся в hooks/conference.
Медиасцена использует `RealtimePanel` и memoized `MediaTile`. Чат, участники,
приглашения и управление записью находятся в панелях и диалогах комнаты.
Prejoin вынесен в `PreJoinPage`, просмотр готового файла — в `RecordingDetailPage`.

## Маршруты

| Путь | Назначение |
| --- | --- |
| `/` | Публичная главная без входа; кабинет с быстрыми действиями для авторизованного пользователя |
| `/login`, `/register`, `/register/success` | Email/password авторизация и результат регистрации |
| `/app` | Совместимый старый адрес: перенаправление на `/` |
| `/meetings`, `/conferences` | Фильтрованный список встреч |
| `/personal`, `/personal/:id` | Личные и групповые переписки; общий MessageThread/AttachmentUploader |
| `/folders`, `/folders/:id` | Личные папки встреч и переписок |
| `/calendar` | Предстоящие запланированные встречи: неделя, месяц, мобильный список |
| `/conferences/new`, `/meetings/new`; диалог на главной | Создание/планирование и переход по ссылке |
| `/i/:code` | Приглашение и prejoin: вход с аккаунтом либо гостем |
| `/conferences/:id/join`, `/meetings/:id/join` | Prejoin, устройства и допуск |
| `/conferences/:id`, `/meetings/:id` | Комната и управление встречей |
| `/conferences?view=past` | Завершённые встречи |
| `/recordings`, `/app/recordings` | Записи выбранной встречи |
| `/recordings/:id?conference=:conferenceId` | Просмотр записи и её файлов в контексте встречи |
| `/history/*` | Перенаправление на `/conferences?view=past` |
| `/analytics` | Аналитика одной завершённой или активной встречи |
| `/search`, `/app/search` | Перенаправление на `/` |
| `/settings`, `/app/settings` | Профиль, аккаунты, аудио, видео, уведомления и оформление |
| `/notifications`, `/admin` | Уведомления и защищённые операции администратора |
| `/app/settings/calendar/:provider/callback` | Callback существующего календарного OAuth |

Источник текущих маршрутов — `src/App.tsx`, HTTP-контракт описан в [API](API.md).
Старые history/search URL не открывают отдельные страницы материалов или поиска.
Аналитика использует `view=past` по
умолчанию, `view=active` — по выбору; отсутствие фильтра нельзя трактовать как «все».

## Данные и жизненный цикл соединений

- `api.ts` — same-origin `/api/v1`, Bearer-токен, AbortSignal, нормализованные ошибки,
  безопасные signed URLs; ответы сервера не подменяются демонстрационными данными.
- `auth.tsx` — короткий токен в памяти/sessionStorage и постоянная серверная
  сессия зарегистрированного аккаунта через HttpOnly cookie. `POST /auth/refresh`
  обновляет JWT с указанием текущего пользователя; смена аккаунта очищает Query
  cache и соединения прежней личности; ключ хранения — `meet.session.v1`.
  Гостевая сессия ограничена одной встречей и сроком JWT. Контракты:
  [постоянная авторизация](PERSISTENT_AUTH.md), [аккаунты](account-management.md).
- `queries.ts` — user-scoped query keys, серверные фильтры, cursor/offset pagination.
  Главная содержит быстрые действия; списки встреч загружаются в своих разделах.
  Поиск названий в списке встреч применяется к загруженным страницам.
- `realtime.ts` (включая `useRealtime`) — ticket, WS, snapshot/reconnect,
  reconciliation и cleanup. События обновляют локальное состояние комнаты;
  snapshot и reconciliation восстанавливают его по данным сервера.
- `media.ts`/`useMedia.ts` — SFU/WebRTC client: admission → signaling → publication;
  SDP/ICE, server policy, exclusive screen sharing и reconnect. MediaStream не
  сохраняется в Query cache.
- `MediaTile` меняет `srcObject` при изменении самого потока, а не каждого события
  чата/участника; при удалении видео отвязывает поток. Управление tracks остаётся
  у владельца media lifecycle.
- На публичном prejoin по приглашению устройства запрашиваются автоматически
  согласно сохранённым настройкам текущего пользователя. Браузер контролирует
  разрешения; отказ не мешает входу без устройств. Обычный prejoin без invitation
  использует кнопки управления. Preview освобождается при переходе; native
  `getDisplayMedia` всегда показывает системный выбор окна/экрана.
- `personalRealtime.ts` создаёт account WS с одноразовым билетом, восстановлением
  истории при reconnect, ограниченным окном дедупликации и очисткой при logout.
  Контракты: [PERSONAL_MESSAGES.md](PERSONAL_MESSAGES.md).
- `DirectCalls` обрабатывает `call.updated`, перечитывает `/calls/active` и
  управляет звонком через REST. После принятия `DirectCallMedia` подключает
  закрытую conference/SFU с тем же `clientId`; account WS не передаёт медиабайты.
  Подробности: [direct-calls.md](direct-calls.md).
- `useNotificationStream` создаёт одну SSE-подписку оболочки зарегистрированного
  аккаунта. Данные записи загружаются для выбранной встречи и записи, с отдельными
  user-scoped query keys.

## Авторизация, возможности и безопасность

Вход использует email/password; календарный OAuth не авторизует в приложении.
JWT живёт один час, хранится в памяти и `sessionStorage` вкладки. Постоянная
HttpOnly cookie-сессия восстанавливает аккаунт после открытия браузера и позволяет
обновить JWT. Отказ сети сохраняет профиль и открытую конференцию. Явный выход
сразу открывает публичную главную, очищает кэши и соединения; локальная отметка
выхода позволяет повторить отзыв серверной сессии после восстановления сети.
Гость получает JWT одной встречи на час, без постоянной сессии.

`GET /api/v1/auth/me` возвращает профиль; `PATCH /api/v1/auth/me` принимает только
`{displayName}` — 1–100 Unicode code points после trim, без управляющих символов.
Профиль обновляется без замены JWT; очистка имени не поддерживается.
Смена аккаунта отменяет запросы, очищает Query cache, Object URLs и соединения
прежней личности. Refresh передаёт выбранный `userId`.

`GET /api/v1/capabilities` сообщает `liveCaptions`, `transcription`, `aiSummary`,
`semanticSearch`, `meetingAnalytics`, `recordingModes` и `buildVersion` без
адресов провайдеров и ключей. Неизвестный или ошибочный ответ не разрешает функцию.
Панели учитывают также статусы и ошибки операций. Email, push, STT, ИИ и embeddings
по умолчанию выключены; `mock` не выполняет реальную доставку или обработку.

`/admin` читает `GET /api/v1/admin/summary`: агрегаты встреч, присутствия, записей,
заданий и отказов, readiness API/media-worker и состояние инфраструктуры.
Право `users.is_admin` проверяется каждым запросом и не совпадает с ролью owner
конференции. Интерфейс не показывает чат, расшифровки, секреты или персональные
строки. Назначение права описано в [admin runbook](operations/admin.md).
Owner/co_host конференции также различаются: назначение ролей и camera control —
owner-only. Клиентские ограничения не заменяют серверные ACL.

Пользовательский текст выводится средствами React, без `dangerouslySetInnerHTML`.
Вложения ограничены серверным allowlist и 10 МиБ на файл. Group-файлы/аватары
читаются Bearer fetch в Object URL; direct/conference файлы — по подписанным
ссылкам. Токены и ссылки не должны попадать в логи, а секреты — в `VITE_*`.
JWT в `sessionStorage` доступен JavaScript и не защищён от XSS как HttpOnly cookie.

Production Nginx отправляет CSP без `unsafe-eval` и `unsafe-inline`,
`X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`.
Точный origin `MINIO_PUBLIC_ENDPOINT` добавляется в `img-src`/`media-src`,
WSS origin берётся из `PUBLIC_FRONTEND_URL`. Путь, query, userinfo и wildcard
в этих переменных отклоняются при запуске. Локальное HTTP-хранилище допускается
только на loopback (`localhost` или `127.0.0.1`); HTTPS-странице нужен HTTPS endpoint.

## Поведение экранов

После входа открывается `/` либо проверенный внутренний адрес `next`.
Перенаправление `/app` сохраняет query и hash. Восстановление сессии сначала
показывает загрузку, при временной ошибке — повторную попытку.

Prejoin показывает название встречи, имя, preview, уровень микрофона и выбор
устройств. Предпочтения хранятся локально для аккаунта; исчезнувшее устройство
заменяется системным. Имя аккаунта меняется в профиле, гостевое — на приглашении.
Действующий invite допускает в открытую встречу сразу, включая прежний waiting;
rejected/kicked сохраняются. Без invite прежнее ожидание не снимается.
После reconnect аппаратный захват не включается автоматически.

Комната показывает WebSocket/PeerConnection/ICE состояния. Диагностика включает
RTT, потери входящих RTP-пакетов, bitrate и direct/relay, если браузер их выдаёт.
Копируемый отчёт исключает SDP, IP, токены и идентификаторы устройств.

В «Записях» выбирается встреча, затем запись. Просмотр содержит плеер и файлы;
чат открывается в самой конференции и после завершения становится доступным
только для чтения. Удалить завершённую запись могут owner или её зарегистрированный
инициатор при сохранённом доступе к истории и отсутствии активной аренды worker.
Уведомления открывают ресурсы по внутренним ссылкам, сохраняя серверную проверку прав.

Настройки открываются модальным окном без смены URL; прямые `/settings` и
`/app/settings` показывают его над главной, закрытие ведёт на `/`.
Аккаунту доступны профиль, аккаунты, аудио, видео, уведомления, оформление;
гостю — аудио, видео, оформление. Можно подключить до пяти аккаунтов браузера.
Тема и размер текста хранятся в `localStorage` отдельно для аккаунта; публичные
страницы и гость используют светлую тему и 100%. Устройства сохраняются только
в этом браузере. Управления календарными подключениями в окне нет, серверные
подключения и OAuth callback продолжают работать.

Личные/group переписки используют общий `MessageThread`, server search и
user-scoped query keys. Уведомления можно отключить для себя в обоих типах;
очистка истории и скрытие — только direct. В groups до 100 участников и роли
owner/admin/member. Общие закрепления возвращаются в `pinned`; конференционные
личные отметки — в `important`. Папки связывают доступные встречи и переписки,
не предоставляя новых прав. Сценарии: [USER_GUIDE.md](USER_GUIDE.md).

## Дизайн и адаптивность

`src/brand.ts` — единственный источник имени продукта; `VITE_PRODUCT_NAME` меняет
его во время сборки. По умолчанию используется MeetSpace; прежние значения
Meetrix/Meet также отображаются как MeetSpace. `public/brand-mark.svg` — кодовый знак.
`tokens.css` содержит semantic colors, поверхности комнаты, spacing/radius/shadow,
типографику и z-index. Старые `--blue`, `--line`, `--soft` — aliases для совместимости.
`workspace.css` задаёт оболочку/auth, остальные стили ограничены страницами.

Desktop: светлый кабинет с sidebar/topbar, тёмная активная комната без внешней
навигации. Tablet ≤1100px — компактная сетка; mobile ≤760px — drawer, нижняя
навигация и список календаря. Панели комнаты открываются поверх сцены на узком
экране. Формы и таблицы не расширяют страницу за viewport; широкая техническая
таблица аналитики прокручивается внутри своего контейнера.

Есть skip-link, labels, aria-live состояния, keyboard tabs, Escape/focus return,
focus trap диалогов/drawer и reduced motion. Горячие клавиши M/V/C
переключают микрофон, камеру и чат; они не действуют при наборе текста или в диалоге.
Основные пользовательские строки русские, полной локализации нет.

## Запуск, HTTPS и HMR

Сначала выполните подготовку `.env` и инфраструктуры из
[README](../README.md#локальный-запуск): `JWT_SECRET` минимум 32 байта,
собственные пароли PostgreSQL/RabbitMQ/MinIO и внешняя сеть `gateway_default`
(либо имя из `GATEWAY_NETWORK`). Для локального HTTP задайте:

```dotenv
APP_ENV=local
PUBLIC_FRONTEND_URL=http://localhost:5173
MINIO_PUBLIC_ENDPOINT=http://localhost:9000
WORKER_INTERNAL_URL=http://worker:8090
```

В `MEDIA_NAT_IPS` нужен IP хоста Docker, доступный браузеру; для Docker Desktop
и Firefox используйте LAN IP. Адрес страницы должен точно совпадать с
`PUBLIC_FRONTEND_URL`, включая host и порт, иначе login/refresh вернут `403`.
Если задан `WS_ALLOWED_ORIGINS`, он должен включать используемый origin.

Контейнерная сборка интерфейса из корня репозитория:

```sh
docker compose up -d --build api media-worker worker product-worker frontend
```

Откройте `http://localhost:5173`; API опубликован на `http://localhost:8085`.
Зависимости запускаются Compose. В локальной конфигурации `AUTO_MIGRATE=true`
применяет up-миграции перед работой API; в production миграция — отдельный шаг.
Контейнер frontend не получает корневой `.env` или серверные ключи.
Nginx направляет `/api/` и WebSocket к API; SPA fallback не подменяет JSON-ошибки.

Для HMR нужен Node **22.22.2–22.x, 24.15.0–24.x или 26+** — точное условие
`^22.22.2 || ^24.15.0 || >=26.0.0` из `package.json`. Docker frontend использует Node 24.
Освободите порт контейнерного frontend и запустите Vite из корня проекта:

```sh
docker compose stop frontend
docker compose up -d --build api media-worker worker product-worker
npm --prefix frontend ci
npm --prefix frontend run dev
```

Vite слушает `127.0.0.1:5173` (`strictPort=true`), проксирует `/api` и WebSocket
на `http://127.0.0.1:8085`, сохраняя Origin. Другой backend задаётся только для dev:

```sh
API_PROXY_TARGET=http://127.0.0.1:28085 npm --prefix frontend run dev
```

`npm --prefix frontend run build` делает TypeScript-проверку и Vite build;
`npm --prefix frontend run preview` предназначен для просмотра сборки и не заменяет
production Nginx с API/WS/TLS/CSP. У preview другой origin/порт; наличие страницы
само по себе не подтверждает работоспособность auth/API.

Для локального HTTPS сначала создайте сертификаты:

```sh
HTTPS_CERT_IP=127.0.0.1 sh scripts/generate-https-certs.sh
```

Доверяйте CA `dockers/https/certs/go-recorder-local-ca.pem` в системе/браузере.
Повторная генерация заменяет сертификаты. Установите оба
`PUBLIC_FRONTEND_URL` и `MINIO_PUBLIC_ENDPOINT` в `https://localhost:18482`, затем
`docker compose up -d --build`. Для LAN используйте доступный адрес, включённый
в сертификат. `make up` также запускает HTTPS и требует подготовленных сертификатов.
HTTP- и HTTPS-настройки auth нельзя смешивать. Камера/микрофон требуют secure
context: HTTPS либо разрешённый браузером localhost.

Новая production-установка: [INSTALLATION.md](INSTALLATION.md).
Локальный Compose публикует инфраструктуру и не является production-конфигурацией.

Публичные настройки сборки:

| Настройка | Назначение |
| --- | --- |
| `VITE_PRODUCT_NAME` | Имя; образ Docker принимает `--build-arg PRODUCT_NAME=MeetSpace` |
| `VITE_BUILD_VERSION`, `VITE_BUILD_COMMIT`, `VITE_BUILD_TIME` | Публичные release metadata |
| `VITE_CLIENT_TELEMETRY_ENABLED` | Переключатель безопасной телеметрии |
| `API_PROXY_TARGET` | Только proxy Vite в разработке, не секрет и не production API URL |

## Версия артефакта и ошибки браузера

Каждая сборка включает публичный `/version.json` с тремя полями: `version`,
`commit`, `buildTime`. Эти же значения встраиваются в JavaScript; источник —
`VITE_BUILD_VERSION`, `VITE_BUILD_COMMIT`, `VITE_BUILD_TIME` при сборке.
В Docker им соответствуют аргументы `BUILD_VERSION`, `BUILD_COMMIT`, `BUILD_TIME`.
Для релиза передаются версия `vX.Y.Z`, SHA коммита и UTC-время в RFC3339. Значения `dev`/`unknown` означают локальную сборку,
а не подтверждённый release. Серверные секреты и остальные переменные окружения
в JSON не включаются. Nginx запрещает использование сохранённого ответа без
повторной проверки. Этот адрес описывает frontend; версия API проверяется отдельно.

Телеметрия по умолчанию выключена. Для включения нужен frontend build argument
`CLIENT_TELEMETRY_ENABLED=true` (`VITE_CLIENT_TELEMETRY_ENABLED=true` при прямой
сборке) и серверный `CLIENT_TELEMETRY_ENABLED=true`. Смена frontend-флага требует
нового артефакта. Ошибки JavaScript, необработанные Promise и отказы рендера React
отправляются только на same-origin `POST /api/v1/client-errors`. React также
показывает экран восстановления с кнопкой обновления страницы.

Перед отправкой остаются только версия, шаблон маршрута, фиксированный код,
семейство браузера и максимум пять координат в JS-артефактах текущего origin.
Query, hash, UUID, код приглашения, исходное сообщение, имена функций, внешний
URL, содержимое чата/расшифровок, SDP, Authorization, cookies и Referer не
отправляются. Неизвестный путь превращается в `unknown`; подписанные URL и кадры
из исходных файлов разработки отбрасываются. Отправка ограничена пятью событиями
в минуту, двадцатью за страницу и подавлением одинаковых событий на десять секунд.
Адаптер не хранит очередь на диске, не повторяет неудачные запросы и не использует
сторонний SDK. Это ограниченная диагностика ошибок, а не аналитика действий.

## Проверка сборки и интерфейса

Из корня проекта:

```sh
npm --prefix frontend run check
```

`check` запускает Prettier и TypeScript/Vite build. Для отдельной проверки типов
есть `npm --prefix frontend run typecheck`. В package.json нет test/e2e-команд.
Build target — Chrome 120, Firefox 120, Safari 17; поддержку реальных устройств
и сетей проверяют отдельно.

Перед выпуском проверьте изменённые сценарии на desktop/mobile, keyboard/focus,
тему и масштаб текста, переключение аккаунтов, API/WS, устройства и доступ к
материалам. CSP проверяйте без ослабления политики. Сверьте frontend `/version.json`
и API `/version`; при включённой телеметрии проверьте отсутствие credentials и
пользовательского текста в запросах. Общая приёмка и откат:
[RELEASE_PROCESS.md](RELEASE_PROCESS.md).

## Точные HTTP-пути клиента

`frontend/src/api.ts` использует same-origin `/api/v1`. История встречи —
`/conferences/:id/history`, списки — `/me/conferences` с `view`, уведомления —
`/notifications` и `/notifications/:notificationId/read`, SSE — `/notifications/events`
с Bearer в заголовке. Поиск переписки — `/conversations/:id/messages/search`,
конференционного чата — `/conferences/:id/chat/messages/search`.
Не переносите frontend redirects `/history/*` или `/search` в адреса API.
