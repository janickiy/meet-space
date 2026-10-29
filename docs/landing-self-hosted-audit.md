# Аудит Self-Hosted для публичного лендинга

> Исторический отчёт: отражает состояние проекта на дату проверки. После удаления распознавания речи и переименования сервиса записи его перечень компонентов не является описанием текущей версии. Актуальные сведения: [архитектура](ARCHITECTURE.md).

Дата: 18 октября 2026. Проверяемый исходный commit: `893566bb90f07bdcec4542eec5e00116c423e852`.

Область: фактические Compose, env-примеры, release-скрипты и эксплуатационные документы. Аудит выполнен чтением исходного дерева; Docker-окружения не запускались, удалённые серверы не изменялись, production readiness не подтверждалась.

## Итог

В проекте существует single-host Docker Compose процесс выпуска с отдельной миграцией, неизменяемыми image references, backup/restore helpers и блокирующей проверкой production evidence. Это не однокомандный инсталлятор для произвольного VPS. Текущий исходный commit не равен автоматически подготовленному серверному релизу.

Страница `/docs/self-hosted` объясняет необходимую подготовку, даёт команды SSH, закреплённого checkout и реальных release-скриптов для отдельного staging, фиксирует отсутствие гарантированной удалённой проверки. Ссылки на исходники закреплены на полный SHA.

## Доказательства и матрица

| Область                        | Статус                                  | Фактическое подтверждение / предел                                                                                                                             |
| ------------------------------ | --------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Серверный Compose              | Реализовано                             | `docker-compose.production.yml`; `scripts/release/common.sh` формирует список файлов и образов из manifest                                                     |
| Локальный Compose              | Реализовано для разработки              | `docker-compose.yml`: локальные build, defaults, публикация PG/Redis/MinIO и внешняя gateway network; непригоден как рекомендация для публичного VPS           |
| Выпуск неизменяемых артефактов | Реализовано                             | `scripts/release/build.sh`, `package.sh`, `release.sh`, `common.sh`; registry digests и archive image IDs, clean provenance, checksum/SBOM/security validation |
| Production текущего commit     | Не подтверждено                         | В этой работе не собран release package, не выполнены серверные, WAN, restore/capacity проверки и version-bound приёмка                                        |
| Миграции                       | Реализовано                             | `release.sh migrate`, service `migrate`, API image entrypoint `/app/recorder-migrate migrate`; `AUTO_MIGRATE=false`                                            |
| Backup / restore               | Частично по покрытию                    | PostgreSQL dump + текущие объекты S3; отдельный restore-check; не полная резервная копия инфраструктуры                                                        |
| Application rollback           | Реализовано с условиями                 | Прежние образы на совместимой текущей схеме; автоматического down SQL нет                                                                                      |
| TLS provisioning               | Не реализован как автоматическая выдача | Proxy читает подготовленные `fullchain.pem` / `privkey.pem`; внешний TLS нужен для shared-host                                                                 |
| Системные минимумы VPS         | Не подтверждены                         | Нет универсальной утверждённой матрицы CPU/RAM/диска/capacity; лимиты Compose не являются минимумами                                                           |
| Android SSH wizard             | Не подтверждён / недоступен             | В исходном дереве не найден Android/desktop/mobile проект или SSH wizard; страница не изображает его готовым                                                   |
| HA / живая миграция комнат     | Не реализованы в этой схеме             | `docs/ARCHITECTURE.md`, `DEPLOYMENT.md`: один Compose-хост, управляемое последовательное обновление с возможным перерывом                                      |

## Два поддерживаемых layout

### `classic`

`common.sh` выбирает `docker-compose.production.yml` и `docker-compose.grafana.yml`. Сервисы приложений: `api`, `media-worker`, `worker`, `product-worker`, `live-worker`, `frontend`. Зависимости: PostgreSQL, Redis, RabbitMQ, MinIO, Coturn, proxy; observability: Prometheus/Grafana.

- Nginx публикует `443/TCP`, читает сертификаты из `TLS_CERT_DIR`.
- SFU: `MEDIA_UDP_PORT` / `MEDIA_TCP_PORT`, по умолчанию `50010`.
- Coturn: `3478/UDP+TCP`, `5349/TCP`, relay `49160–49259/UDP`.
- БД, broker, Redis, MinIO console, внутренние HTTP endpoints не опубликованы.
- Prometheus `127.0.0.1:19090`, Grafana `127.0.0.1:13001`.
- Том PostgreSQL `postgres`, RabbitMQ `rabbitmq`, MinIO `minio`; отдельный bind для recorder spool.
- Redis в production не использует persist AOF/RDB: состояние краткоживущее.

### `hardened-shared-host`

`common.sh` выбирает production + hardened + shared-host. Используется SeaweedFS, но service DNS `minio` и `MINIO_*` сохранены для совместимого S3 клиента. Новый том `objects` несовместим со старым томом MinIO; автоматический перенос не происходит.

Proxy публикуется только на `127.0.0.1:${REVERSE_PROXY_PORT:-19080}`. Требуется существующий внешний TLS reverse proxy, например описанный в истории Apache. Coturn использует `network_mode: host`, привязывается к публичному IP, ограничения firewall остаются ответственностью оператора. Подсеть `172.31.242.0/24` необходимо проверять на конфликты. Grafana исключена из данного layout. Ресурсные лимиты существенно ниже classic и рассчитаны на ограниченный стенд, не универсальную нагрузку.

## Конфигурация и безопасность

Источники: `.env.production.example`, `internal/config/operations.go`, `internal/config/product.go`, `dockers/turn/entrypoint.sh`, production Compose.

- `JWT_SECRET`, `MEDIA_TICKET_SECRET`, `MEDIA_INTERNAL_SECRET`, `WORKER_INTERNAL_SECRET`, `METRICS_SECRET`, `TURN_SHARED_SECRET` должны быть независимыми, не менее 32 байт; production отклоняет повторные/default значения.
- `POSTGRES_PASSWORD`, `RABBIT_MQ_PASSWORD`, `MINIO_ROOT_PASSWORD`, `REDIS_PASSWORD` требуют non-default значения не короче 16 байт.
- `PUBLIC_FRONTEND_URL`, `MINIO_PUBLIC_ENDPOINT`, `WS_ALLOWED_ORIGINS` — реальные HTTPS origins; trusted proxies задаются точно.
- `MEDIA_NAT_IPS`, `TURN_PUBLIC_IP`, `TURN_REALM`, `TURN_URLS` должны соответствовать публичной сети и DNS.
- Сертификаты готовятся отдельно; автоматических ACME-клиента или гарантии сертификата для IP нет.
- `RECORDER_STORAGE_DIR` должен принадлежать `1000:1000`, права `0750`; запас диска определяется измерением нагрузки и политикой записи.
- Env и secrets вне Git; полный `docker compose config` может раскрывать секреты, поэтому в инструкции он не предлагается. `release.sh validate` использует quiet validation.
- `METRICS_TOKEN_FILE` соответствует `METRICS_SECRET`; classic требует `GRAFANA_PASSWORD_FILE`.
- Все операторы используют один абсолютный `RELEASE_LOCK_DIR` на Docker-хосте.
- Email/push/calendar/STT/AI/live/embeddings по умолчанию noop/disabled, не обещаются включёнными. Provider encryption key имеет отдельный формат base64 AES-256 и нужен для согласованного восстановления токенов. Embeddings требует совместимый pgvector image/extension.

## Процесс выпуска

`release.sh` принимает `validate|pull|migrate|drain|resume|deploy|rollback`, явную среду, соответствующее имя проекта, абсолютные env/manifest пути. Production mutating actions проверяют `RELEASE_PRODUCTION_APPROVAL`, `--evidence`, совпадение version/commit/manifest SHA-256 и действительные утверждения совместимости, backup, rollback, staging, media, security. Gate проверяет структуру утверждений, а не истинность отчёта оператора.

`migrate` отдельно поднимает зависимости и применяет SQL; `deploy` не собирает и не мигрирует. Существующие инфраструктурные image IDs защищены от незаметного изменения. Порядок приложений: media-worker → worker → product-worker → live-worker → api → frontend, затем proxy/observability. Drain timeout оставляет процесс draining, не делает force restart. Для frontend/proxy без Docker healthcheck `wait_healthy` видит running, поэтому нужны реальные HTTP-проверки.

`package.sh` поддерживает registry/archive/local. Archive production допустим при проверенном чистом происхождении, соответствующих image IDs и scan/SBOM. Обобщение «production только registry» было бы неверным. `scripts/ci-deploy.sh` — legacy dev sync/build/up, не production promotion; на лендинге не предлагается.

## Backup и откат

Источники: `docs/operations/backup-restore.md`, `docs/ROLLBACK.md`, `scripts/release/backup.sh`, `restore-check.sh`.

Backup включает custom PostgreSQL dump и текущие S3 key/bytes/hash/ContentType. Он не атомарен между хранилищами; нужен глобальный write freeze, включая presigned uploads, cron и прямых писателей. `RELEASE_BACKUP_QUIESCED=1` — утверждение оператора, а не автоматическое доказательство.

Не включены история S3 versions/delete markers/tags/произвольная metadata, broker snapshot, незавершённый recorder spool, env, ключи, TLS, IAM/ACL/policies и Grafana config. Restore-check работает на отдельной local/staging цели, создаёт отдельные БД/bucket и не является destructive restore production. Hash и signed access не заменяют проверку бизнес-связей данных.

Application rollback не откатывает SQL и не восстанавливает данные. Требуются прежний готовый пакет и доказанная совместимость со схемой. Смена инфраструктуры требует собственного окна обслуживания.

## Противоречия и устаревшие сведения в исходной документации

1. `DEPLOYMENT.md`, `ROLLBACK.md`, `PRODUCTION_LAUNCH_CHECKLIST.md` говорят, что настоящий staging ещё не предоставлен. При этом `docs/operations/meeting-host-deployment.md` содержит подробную историю удалённого пилота и прямо отличает его от production-приёмки. Нельзя ни объявлять отсутствие любого реального размещения, ни приписывать этому commit production readiness.
2. `DEPLOYMENT.md` требует synthetic smoke credentials; более актуальный `RELEASE_PROCESS.md` говорит об удалении synthetic smoke и ручных проверках существующим согласованным аккаунтом. Страница следует фактическому текущему процессу и не предлагает удалённые scripts, отсутствующие в коде.
3. Исторический `launch-readiness-report.md` и checklist перечисляют старые automated tests. Текущий `RELEASE_PROCESS.md` документирует удаление Go/frontend/e2e тестов по решению владельца. Старые результаты не переносились на создаваемый лендинг.
4. В удалённой истории встречаются разные версии отдельных сервисов и последующие frontend patches. Их нельзя считать согласованным пакетом нынешнего HEAD.
5. Исторические сведения о CVE/scan относятся к конкретным пакетам и датам; новый scan в этом аудите не выполнялся. Страница не обещает чистый security scan текущего выпуска.

## Реализация публичной документации

- `frontend/src/pages/SelfHostedPage.tsx`: именованный export `SelfHostedPage`.
- `frontend/src/pages/self-hosted.css`: стили со scope `.self-page`, светлая палитра, синий/фиолетовый акценты.
- Использует существующий `Brand` и `react-router` ссылки; маршрут и SEO интегрирует основной агент.
- 9 разделов, desktop sticky оглавление, мобильное оглавление, семантические таблицы, native details troubleshooting, команды с горизонтальной прокруткой и кнопками копирования.
- Копирование через Clipboard API; понятная ошибка при недоступном буфере. Нет автоматического выполнения команд.
- Видимый focus, skip link, aria labels/status, кнопки и оглавление с высотой 44px, reduced-motion, адаптивность для узких экранов.
- Все GitHub source links закреплены на исходный commit, в коде страницы нет секретов и готовых паролей.
- Внешние ссылки открываются с `noopener noreferrer`.
- `App.tsx`, `main`, package manifest, Go/Pion и deploy scripts не изменены этой подзадачей.

## Проверки этой подзадачи

- `frontend/node_modules/.bin/prettier --write src/pages/SelfHostedPage.tsx src/pages/self-hosted.css`: выполнено.
- `frontend/node_modules/.bin/tsc --noEmit`: PASS.
- Текст команд сопоставлен с реальными `parse_release_args`, `validate_release`, `release.sh` actions.
- Docker install/Compose сведения сверены по официальным страницам: https://docs.docker.com/engine/install/ и https://docs.docker.com/reference/compose-file/merge/ (Compose `!override` — 2.24.4+).
- Удалённый deploy, production сборка серверного пакета, Docker startup, WAN/TURN, restore и нагрузочные проверки не выполнялись.
- Browser/layout QA объединённой страницы относится к финальной проверке основного агента; самостоятельно в этой подзадаче не заявляется.
