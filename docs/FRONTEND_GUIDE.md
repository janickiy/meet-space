# MeetSpace: устройство и запуск frontend

## Архитектура

Frontend находится в `frontend/`. Стек: React, TypeScript, Vite, React Router,
TanStack Query, Lucide и локальные шрифты Inter. Новых runtime-зависимостей ради
макета не добавлено. Точные версии закреплены в `frontend/package-lock.json`.

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
  cache и соединения прежней личности. Старый ключ `meet.session.v1` сохранён.
  Гостевая сессия ограничена одной встречей и сроком JWT. Контракты:
  [постоянная авторизация](PERSISTENT_AUTH.md), [аккаунты](account-management.md).
- `queries.ts` — user-scoped query keys, серверные фильтры, cursor/offset pagination.
  Главная содержит быстрые действия; списки встреч загружаются в своих разделах.
  Поиск названий в списке встреч применяется к загруженным страницам.
- `realtime.ts` (включая `useRealtime`) — существующие ticket, WS, snapshot/reconnect,
  reconciliation и cleanup. События обновляют локальное состояние комнаты;
  snapshot и reconciliation восстанавливают его по данным сервера.
- `media.ts`/`useMedia.ts` — существующий SFU/WebRTC client: admission → signaling
  → publication; SDP/ICE, server policy, exclusive screen sharing и reconnect не
  заменены новой реализацией ради дизайна. MediaStream не сохраняется в Query cache.
- `MediaTile` меняет `srcObject` при изменении самого потока, а не каждого события
  чата/участника; при удалении видео отвязывает поток. Управление tracks остаётся
  у владельца media lifecycle.
- Prejoin получает устройства только после явного действия, освобождает preview
  при выходе. Native `getDisplayMedia` показывает системный выбор окна/экрана.
- `personalRealtime.ts` создаёт account WS с одноразовым билетом, восстановлением
  истории при reconnect, ограниченным окном дедупликации и очисткой при logout.
  Контракты: [PERSONAL_MESSAGES.md](PERSONAL_MESSAGES.md).
- `useNotificationStream` создаёт одну SSE-подписку оболочки зарегистрированного
  аккаунта. Данные записи загружаются для выбранной встречи и записи, с отдельными
  user-scoped query keys.

## Возможности и безопасность

`/capabilities` сообщает доступность обработки материалов, субтитров, аналитики
и режимов записи; наличие серверного флага не означает отдельный маршрут UI.
Активные панели учитывают capability-флаги, серверные статусы и ошибки.
Окно аккаунта показывает настройки уведомлений; управление календарными
подключениями из него убрано, серверные интеграции и callback сохранены.
Клиентские проверки не заменяют ACL сервера. Owner/co-host
различаются: роль и выключение чужой камеры остаются owner-only.

Форма входа использует email/password. Календарный OAuth не используется
для авторизации в приложении.

Текст чата и AI выводится средствами React, без нового `dangerouslySetInnerHTML`.
Загрузка вложений сохраняет серверный allowlist и лимит 10 МБ на файл; приватные
медиа скачиваются по выданным сервером ссылкам. Секреты никогда не помещаются
в `VITE_*`, артефакты сборки или console logging. Серверная телеметрия допускает
новые статические пути `/calendar` и `/analytics`, но не идентификаторы встреч.

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
focus trap диалогов/drawer и reduced motion. Горячие клавиши комнаты не действуют
при наборе текста или открытом диалоге. Контраст, фокус и доступность нужно
проверять в браузере; наличие этих механизмов не подтверждает результат аудита.

## Настройка, запуск и сборка

Требуется версия Node из `package.json.engines` (проверено Node 24.19.0).
Из каталога `frontend`:

```sh
npm ci
npm run dev
npm run build
npm run preview
```

Dev-сервер слушает только `127.0.0.1:5173`; API proxy по умолчанию направлен на
`http://127.0.0.1:8085`. Изменение порта API: `API_PROXY_TARGET=http://127.0.0.1:28085
npm run dev`. Серверный `WS_ALLOWED_ORIGINS` должен разрешать фактический origin
UI; подменять Origin для обхода защиты нельзя. Камера/микрофон требуют secure
context (HTTPS или разрешённый браузером localhost), а удалённый стенд — TLS/TURN.

Публичные настройки сборки:

| Настройка | Назначение |
| --- | --- |
| `VITE_PRODUCT_NAME` | Имя; образ Docker принимает `--build-arg PRODUCT_NAME=MeetSpace` |
| `VITE_BUILD_VERSION`, `VITE_BUILD_COMMIT`, `VITE_BUILD_TIME` | Публичные release metadata |
| `VITE_CLIENT_TELEMETRY_ENABLED` | Существующий явный переключатель безопасной телеметрии |
| `API_PROXY_TARGET` | Только proxy Vite в разработке, не секрет и не production API URL |

`npm run preview` не заменяет production reverse proxy и его API/WS/TLS/CSP.
Для production используются существующий Dockerfile, Nginx, CSP validation и
release automation; редизайн не разворачивает новый образ автоматически.

## Проверки

```sh
npm run lint
npm run typecheck
npm run build
```

Автоматические unit/a11y/e2e-сценарии, их фикстуры и CSP test script удалены.
Сборка и lint не заменяют ручную проверку интерфейса: пройти изменённые страницы
на desktop/mobile, проверить keyboard/focus, светлую/тёмную тему и масштаб текста.
В согласованном окружении с существующими аккаунтами проверить API/WS, устройства,
медиа и доступ к материалам. Новые данные создавать только с явным согласованием,
а не автоматически при публикации. CSP сверить с фактической конфигурацией proxy;
проверить блокировки в браузере без ослабления политики безопасности.

Проверки health/version, безопасность и откат описаны в
[процессе выпуска](RELEASE_PROCESS.md). Исторические результаты и браузерные
ограничения прежней проверки сохранены в [отчёте](FRONTEND_INTEGRATION_REPORT.md)
и не подтверждают новый артефакт. Build target Chrome120 /
Firefox120 / Safari17 — цель компиляции, а не доказательство тестирования устройств.
