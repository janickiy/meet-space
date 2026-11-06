# MeetSpace: исходная точка комплексной оптимизации

Дата: 6 ноября 2026 года. Задание: `CODEX_PROMPT_MEETSPACE_FULL_AUDIT_OPTIMIZATION.md`.
Аудит охватывает сервер, SFU, запись, хранилища, очереди, пользовательский frontend,
существующую админку, установщик и CI. Приоритет — подтверждённые P0/P1;
непроверенные гипотезы перечислены в [находках](meetspace-findings.md).

## Версия и сохранность рабочей копии

- Исходный commit: `1e27a55b881ac64c2fa5ad15f09a96e825b24328`.
- Исходная ветка: `codex/admin-console`, каталог `/Users/aleksandranickij/htdocs/go-recorder`.
- До начала работы уже были staged `docs/NEXT_RELEASE_PLAN_RU.md` и `docs/README.md`.
  Их содержимое и индекс не изменялись. Исходный diff сохранён вне репозитория.
- Во время аудита внешний процесс переключил общую папку на `main`, `0bd1248a2411842515af4e02f1e95821616748fb`.
  Аудит перенесён в отдельный worktree `/Users/aleksandranickij/htdocs/meetspace-audit-20261106`,
  ветка `codex/full-optimization-20261106`, от исходного `1e27a55`.
  Перенесены только изменения аудита; различия веток в SFU отдельно исключены из diff.
- В исходной папке сохранены переключение пользователя, staged-документы и ранее
  существовавшие артефакты `admin/`. На момент завершения локального аудита
  публикация, push и merge ещё не выполнялись.
- Инвентаризация: 997 отслеживаемых файлов, 344 Go-файла; после новых тестов
  `go list ./...` содержит 79 пакетов; 67 файлов миграций. Поиск регистраций HTTP
  дал 156 строк, это индекс мест регистрации, а не число всех развёрнутых маршрутов.

## Реальные компоненты

| Компонент | Процесс и зависимости | Состояние/ресурсы |
|---|---|---|
| REST, WS/SSE | `cmd/api`, `internal/app/api.go`; Gin, PostgreSQL, Redis, RabbitMQ, S3, internal media HTTP | Сессии и ACL в БД; активные сокеты/ограниченные очереди в памяти |
| SFU | `cmd/media-worker`, Pion WebRTC, Redis | PeerConnection, ICE UDP/TCP mux, комнаты, RTP/RTCP, egress записи |
| Запись | `cmd/record-worker`, PostgreSQL, RabbitMQ, SFU HTTP stream, FFmpeg, S3 | Аренды с поколениями, локальный spool, процессы FFmpeg, метаданные артефактов |
| Продуктовые задачи | `cmd/product-worker`, PostgreSQL, почта/push/календарные провайдеры | Bounded pools, background jobs, leases, retries, scheduler |
| Миграции/CLI | `cmd/main`; `cmd/admin` для административных команд | Явные операции установки и восстановления |
| Пользовательский интерфейс | `frontend/`, React 19, TypeScript 7, Vite 8, TanStack Query | Аккаунты, media tracks, preview, realtime subscriptions, object URLs |
| Админка | `admin/`; отдельный Go admin router/listener и Nginx | Собственные administrator sessions и username/password; HTTPS порт по умолчанию 8081, internal HTTP 8082 |
| Инфраструктура | PostgreSQL 16, Redis, RabbitMQ, MinIO/SeaweedFS, Coturn | Бизнес-данные, S3, transient presence/leases; TURN для медиатрафика |
| Доставка | Compose, `install.sh`, `scripts/install`, `scripts/release`, `.gitlab-ci.yml` | Существующие профили, сертификаты, обновление/откат, проверка релизов |

Путь записи: HTTP/ACL → транзакция record/outbox → fenced claim → AMQP publisher
confirm → record-worker → SFU egress → FFmpeg → S3 → фиксация статуса/артефактов.
Путь уведомлений: транзакционные события → jobs/notifications → проверка текущего
доступа → Redis/WS/SSE и, при настройке, внешние провайдеры.

Изучены README, ARCHITECTURE, DATA_FLOWS, ADMIN_AUDIT_RU, ADMIN_DEPLOYMENT_RU,
операционные инструкции, Makefile, manifests зависимостей, Docker/Compose,
установщик и существующие тесты. Список важных исходников и воспроизведений — в findings.

## Контракты, которые сохраняются

- `/api/v1` и JSON camelCase; Bearer пользователя/гостя; cookie refresh/account switching.
- Независимый `/api/v1/admin`, opaque cookie, точный Origin и CSRF для мутаций;
  пользовательский JWT не становится административной сессией.
- Членство, admission, guest scope, владельцы записей, видимость очищенной/удалённой
  переписки, muted/left состояния. UUID сам по себе не предоставляет доступ.
- Существующие WS/SSE события, WebRTC offer/answer/ICE, RTP/RTCP и порядок команд.
- `record.start`/`record.stop`, at-least-once delivery, lease fencing, outbox и storage keys.
- Личные/групповые сообщения, вложения, поиск, звонки, календарь, уведомления и папки.
- Страницы лендинга/документации, их текст, заголовки, canonical и JSON-LD.
- Миграции, переменные конфигурации, схемы БД, пути обновления и Docker-профили.

Изменений публичных маршрутов, JSON-полей, схемы БД или сигнализации в diff нет.

## Среда и исходные проверки

macOS arm64, Apple M5; Go `1.26.10`; Docker client/server `29.4.2`.
Node `24.22.1` из bundled runtime соответствует `engines`. Системный Node `23.1.0`
не поддерживается: три frontend-suite не запускались из-за отсутствия `registerHooks`;
проверки повторены на поддерживаемой версии без изменения кода ради старого Node.

| Проверка до исправлений | Фактический результат |
|---|---|
| `go test ./...` | PASS; часть результатов из Go cache; DB integration без DSN пропускается |
| `go test -race -count=1 ./...` | PASS; интеграции хранилищ отдельно |
| `go vet ./...`, `go build ./...` | PASS |
| `go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...` | FAIL: S1038 в `library_test.go`, исправлено |
| `govulncheck@v1.8.0 ./...` | 0 вызываемых уязвимостей; 2 в импортируемых пакетах и 2 на уровне модулей без найденных вызовов |
| PostgreSQL `go test -race -count=1 -v ./internal/infrastructure/postgres` | 45 верхнеуровневых тестов PASS на отдельной пустой БД, реальные миграции |
| Frontend tests / lint / build (включает typecheck) | 51 тест PASS, Prettier PASS, build PASS |
| Admin tests / build (включает typecheck) | 34 теста PASS, build PASS; отдельного lint script нет |
| Install/release Python tests | 38 install tests: 36 PASS, 2 opt-in proxy проверки пропущены; 4 release tests PASS. Proxy затем проверен отдельно |
| Compose | 11 корректных сочетаний профилей проходят `config --quiet` с синтетическими переменными |
| Shell | `bash -n` всех 23 исходных shell scripts PASS; shellcheck не установлен |

Первый пробный Compose-прогон обнаружил неполные параметры тестового окружения
и неверное сочетание optional Grafana override. После задания синтетических
секретов, `.env` из примера и базового Grafana profile все 11 сочетаний валидны;
это не дефект production Compose. Реальные `.env` не читались для этих проверок.

## Baseline производительности и границы

- Уведомления: PostgreSQL 16, disposable tmpfs, 2 CPU/1 GiB, 30 000 сообщений,
  100 уведомлений, запрос страницы 50+1 и счётчика. Пять `EXPLAIN (ANALYZE, BUFFERS)`
  для каждого запроса. До исправления — повторные полные сканирования сообщений.
- Realtime: microbenchmark выборки 2 адресатов при 100/1 000/10 000 сокетов.
- SFU: `recordPacket`, RTP payload 1 200 B, включённая/выключенная запись,
  пять прогонов по 500 ms; TCP lifecycle на реальных loopback sockets.
- Браузер: production build, фактически загруженные JS chunks `/login`, gzip;
  отдельные admin navigation/memory cycles и короткий 180-секундный soak.

Точные значения и повторение — [before/after](meetspace-performance-before-after.md).
End-to-end API p50/p95/p99/RPS, рабочие CPU/RSS/fd, полноценный AV/TURN/FFmpeg/S3 soak
не измерены: локальные unit fixtures не подменяют эти результаты.
