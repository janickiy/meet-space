# Развёртывание MeetSpace

Для первой установки на новый сервер: [скачивание и запуск `install.sh`](INSTALLATION.md).
Ниже разделены сопровождение установки мастером и общий release workflow.

MeetSpace разворачивается на одном Docker Compose-хосте. Перед приёмкой
проверьте DNS/TLS/TURN, firewall, связь с реальных устройств и нагрузку.
Состав сервисов и ограничения: [ARCHITECTURE.md](ARCHITECTURE.md).

## Выбор процедуры по типу установки

| Тип | Фактическая конфигурация | Как сопровождать |
| --- | --- | --- |
| Новый standalone через `install.sh` | Production + hardened + сгенерированные `config/compose.json` и `config/turn.yml`; свой proxy на 443, TURN host network, SeaweedFS | `manage.sh` и Compose-функция ниже; автоматического обновления пока нет |
| Общий release, `deploymentLayout=classic` | Production + Grafana overlay; MinIO | `scripts/release/release.sh`, отдельные project/env/manifest |
| Общий release, `deploymentLayout=hardened-shared-host` | Production + hardened + shared-host overlay; существующий внешний TLS proxy, SeaweedFS | Release helper только после сверки внешнего proxy, подсети, портов и томов |
| Development | `docker-compose.yml`, `.env`, локальные bind mounts | [README](../README.md); не шаблон для production |

В development-конфигурации API, служебный HTTP worker, отладчики, PostgreSQL,
Redis и MinIO опубликованы только на `127.0.0.1`. Для удалённой отладки
используйте SSH-туннель. HTTPS и медиапорты остаются доступными для проверки
звонков с других устройств. Не открывайте отладчик или Redis в интернет;
production использует отдельный Compose-шаблон с приватными зависимостями.

Мастер выпускает архив с `deploymentLayout=hardened-shared-host`, но при установке
**заменяет host-specific overlay** своей конфигурацией. Поэтому одного manifest
недостаточно для выбора Compose-файлов. `release.sh`, `backup.sh` и
`restore-check.sh` используют `scripts/release/common.sh`: для hardened они
подключают `docker-compose.shared-host.yml`, не `config/compose.json` и `turn.yml`.
Не запускайте эти helpers над установкой мастера, подставив только её env/manifest:
это может выбрать другие сеть, proxy и пути томов. Продолжение через
`scripts/install/wizard.py --resume` также не обновляет завершённую установку;
`install.sh` принимает только `--ref` и `--help`.

### Compose-контекст установки мастером

Для стандартных каталога `/opt/meetspace` и проекта `meetspace` в root-shell
(`sudo -i`) задайте функцию. При других значениях сверяйте `installation.json`:

```bash
cd /opt/meetspace
compose() {
  docker compose --project-name meetspace \
    --env-file /opt/meetspace/config/meetspace.env \
    -f /opt/meetspace/bundle/deployment/docker-compose.production.yml \
    -f /opt/meetspace/bundle/deployment/docker-compose.hardened.yml \
    -f /opt/meetspace/config/compose.json \
    -f /opt/meetspace/config/turn.yml "$@"
}
compose --profile observability --profile migration config --quiet
compose --profile observability ps -a
compose exec -T api wget -qO- http://127.0.0.1:8085/health/ready
```

Эта функция далее используется в runbook и инструкции резервного копирования.
`manage.sh status`, `logs [service]`, `create-account`, `certificates`, `reload-tls`
используют тот же установленный контекст. Изменения env требуют пересоздания
потребителя; `docker restart` не перечитывает env-file. Перед заменой образов нужны
backup, совместимость миграций, готовый старый пакет и план с **теми же** volumes,
сетью, TLS, proxy и image IDs. Переключение MinIO ↔ SeaweedFS — перенос данных,
а не обычное обновление образа.

## Окружения общего release workflow

| Среда | Конфигурация / проект | Назначение и границы |
| --- | --- | --- |
| Development | `docker-compose.yml`, локальный `.env` | Разработка, локальная сборка и отладка; тестовые пароли/публикации портов не переносятся в production |
| Local release rehearsal | Production Compose + `docker-compose.release-local.yml`; `recorder-release-rehearsal` или такой префикс с суффиксом | Отдельные тома/порты, `APP_ENV=test`, loopback, временный сертификат; допускаются локальные image ID и помеченный dirty source |
| Staging | Production Compose; `recorder-staging` или суффикс | Реальные HTTPS/WSS/TURNS origins, отдельные секреты/данные, immutable registry/archive images, `APP_ENV=production`; состояние стенда проверяется для каждого выпуска |
| Production | Production Compose; `recorder-production` или суффикс | Собственные DNS/TLS/хранилища, проверенный манифест, evidence и явное решение оператора |

У каждой среды свои PostgreSQL, Redis namespaces, RabbitMQ, S3 bucket/данные,
provider credentials и worker identities. Нельзя направлять локальные тесты в
production-зависимости. Внешние email/push/calendar/STT/AI/embeddings по умолчанию
выключены; mock запрещён для staging/production. Матрица флагов и лимитов —
[API.md](API.md), шаблон — [`.env.production.example`](../.env.production.example).

Семантический поиск требует pgvector. Текущий package использует обычный
PostgreSQL 16 image, поэтому одного переключения `EMBEDDINGS_ENABLED` недостаточно:
нужен отдельно согласованный совместимый image/extension и проверка миграции.
Не подменяйте закреплённый образ БД во время application deploy ради этой функции.

## Подготовка хоста

Для общего workflow нужны Docker Engine и Compose 2.24.4+ с поддержкой `!override`, Bash, `jq`,
`openssl`, `ripgrep`, утилита SHA-256; для сборки операторских бинарников —
поддерживаемая версия Go из `go.mod`. Скрипты вызываются через `bash`, права
исполнения у файла не предполагаются. Команды ниже выполняются из корня репозитория.

Для staging/production заранее подготовить:

- DNS, сертификаты HTTPS и TURNS, публичный SFU IP и совпадающие firewall/NAT
  порты из [архитектуры](ARCHITECTURE.md); один открытый порт не доказывает relay.
- Абсолютный env-файл вне Git, права `0600`, секреты из внешнего защищённого
  источника; не выводить `docker compose config` без `--quiet` и не включать `set -x`.
- `TLS_CERT_DIR` и `TURN_TLS_CERT_DIR` с `fullchain.pem`/`privkey.pem`.
  Ключ Coturn должен читаться его пользователем, но не быть общедоступным.
- `RECORDER_STORAGE_DIR` с владельцем `1000:1000` и правами `0750`, запасом места;
  директории объектов и БД на диске с проверенным резервированием.
- `METRICS_TOKEN_FILE` вне Git с правами чтения контейнера; значение совпадает
  с `METRICS_SECRET`. Для classic также нужен `GRAFANA_PASSWORD_FILE`.
- Доступ к registry, чистый проверенный release manifest и предыдущий manifest.
  Все операторы одного Docker-хоста используют общий абсолютный `RELEASE_LOCK_DIR`.
  Локальный lock-каталог одного ноутбука не защищает от другого оператора.
- Обычные аккаунты для ручной проверки входа, прав и пользовательских сценариев.

Production Compose не публикует БД, Redis, RabbitMQ и S3. Prometheus доступен
на loopback 19090. Grafana доступна на loopback 13001 только в classic-варианте;
hardened и текущая установка мастером её не запускают. Организуйте закрытый
доступ оператора. Внутренние HTTP/AMQP/Redis предполагают доверенную сеть одного хоста; перенос на разные
хосты требует отдельного решения TLS/mTLS/VPN.

## Локальная репетиция без изменения существующих проектов

Выберите новый абсолютный каталог; init создаёт отдельные конфигурацию,
секреты, сертификат и storage для репетиции.

```bash
LOCAL_RELEASE_DIR=/absolute/private/meet-rehearsal
bash scripts/release/init-local.sh --directory "$LOCAL_RELEASE_DIR"
```

Получатся `.env.local`, временный TLS-сертификат, секреты и storage. HTTP API
публикуется только на `127.0.0.1:28085`, HTTPS UI — `https://localhost:25482`,
SFU — `50220`, TURN — `23478` и relay `50300–50340`. Это локальная схема с
разрешённым loopback и без TURNS; её параметры нельзя считать production-шаблоном.
Init использует доступную контейнеру папку spool; для production обязательны
ограниченные права, указанные выше.

Для GET-проверки локального HTTPS явно укажите созданный CA, например
`curl --cacert "$LOCAL_RELEASE_DIR/certs/fullchain.pem" https://localhost:25482/version`.
Для ручной работы браузера настройте доверие сертификату только в выделенном
профиле стенда. На staging/production проверяйте действительную цепочку
и имя сертификата.

После [сборки и упаковки](RELEASE_PROCESS.md) укажите существующий манифест:

```bash
LOCAL_MANIFEST=/absolute/artifacts/package/release.json
bash scripts/release/release.sh validate --environment local --project recorder-release-rehearsal --env-file "$LOCAL_RELEASE_DIR/.env.local" --manifest "$LOCAL_MANIFEST"
bash scripts/release/release.sh migrate --environment local --project recorder-release-rehearsal --env-file "$LOCAL_RELEASE_DIR/.env.local" --manifest "$LOCAL_MANIFEST"
bash scripts/release/release.sh deploy --environment local --project recorder-release-rehearsal --env-file "$LOCAL_RELEASE_DIR/.env.local" --manifest "$LOCAL_MANIFEST"
```

Скрипты принимают только разрешённый проект для выбранной среды. `migrate`
поднимает зависимости и запускает явную migration-задачу. `deploy` не выполняет
DDL и не собирает образы. Если зависимости уже существуют, смена их image
запрещается: обновление PostgreSQL/Redis/RabbitMQ/S3/Coturn — отдельное окно.

## Staging и production общего release workflow

Перед выполнением команд замените все пути и адреса на проверенные реальные
значения. `validate` проверяет формат/Compose, но не доказывает доступность
DNS, сертификата, TURN, registry и восстановимость данных.

```bash
STAGING_CONFIG=/srv/meet/config/staging.env
RELEASE_MANIFEST=/srv/meet/releases/v1.2.3/release.json
bash scripts/release/release.sh validate --environment staging --project recorder-staging --env-file "$STAGING_CONFIG" --manifest "$RELEASE_MANIFEST"
bash scripts/release/release.sh pull --environment staging --project recorder-staging --env-file "$STAGING_CONFIG" --manifest "$RELEASE_MANIFEST"
bash scripts/release/release.sh migrate --environment staging --project recorder-staging --env-file "$STAGING_CONFIG" --manifest "$RELEASE_MANIFEST"
bash scripts/release/release.sh deploy --environment staging --project recorder-staging --env-file "$STAGING_CONFIG" --manifest "$RELEASE_MANIFEST"
```

Для обновления существующего окружения перед `migrate` обязателен согласованный
[backup с write freeze](operations/backup-restore.md) и проверка миграций на копии.
Production использует те же операции с `--environment production`, проектом
`recorder-production`, собственным env и дополнительными approval/evidence
из [RELEASE_PROCESS.md](RELEASE_PROCESS.md). Нельзя подменять `production` на
`local` ради обхода gate.

Обновление приложений идёт последовательно:
перед ним ограничить новые старты записей, дождаться уже ожидающих `record.start`
и штатного завершения активных записей. `active=0` у процесса не означает пустую
очередь команд. Ограничение requeue и порядок проверки recorder drain описаны в
[runbook](operations/README.md#health-и-завершение).

Порядок замены:
`media-worker → worker → product-worker → [live-worker] → api → frontend → proxy`,
затем observability. Сначала выполняется drain, ожидается нулевая активность,
после запуска проверяется health. При тайм-ауте процесс остаётся draining,
принудительного restart нет. Это управляемая замена single-host сервисов с
возможным перерывом, а не обещание zero-downtime или canary traffic splitting.

`live-worker` — необязательный профиль `live`: release-helper включает его
только при `LIVE_STT_ENABLED=true` или `MEETING_ANALYTICS_ENABLED=true` в проверенном
env-файле. По умолчанию оба флага `false`; готовность live-worker не требуется,
его цель исключена из Prometheus. Полный пакет всё ещё содержит проверенный образ
для последующего включения; проверки его происхождения и SBOM не ослаблены.
При выключении ранее запущенного воркера release-helper сначала выполняет drain,
ждёт `active=0`, затем останавливает контейнер без удаления. Если работа не
завершена за тайм-аут, обновление останавливается без принудительного kill.

Для возобновления после общего drain/backup существует `release.sh resume`
с тем же установленным manifest: он пересоздаёт процессы с теми же образами.
Если выпуск остановился посередине и образовалась смесь версий, сначала
сверьте фактические image ID каждого сервиса и процедуру [отката](ROLLBACK.md).
Не используйте `resume` с произвольным манифестом.

## Ротация секретов

Записывайте изменения в защищённый env/secret store, не в образ или Git.
Планируйте перезапуск потребителей и проверку `/version`, readiness, auth,
WS/медиа и доставки после каждой группы изменений.

| Секрет | Последствия и порядок |
| --- | --- |
| `JWT_SECRET` | Новый ключ отвергает старые JWT; потребители должны согласованно получить его. Logout отдельного пользователя не заменяет эту операцию |
| Media ticket/internal, recorder internal | Обновить API и соответствующие workers согласованно; старые tickets/вызовы могут перестать работать, понадобится reconnect |
| `METRICS_SECRET` | Согласованно обновить приложения и `METRICS_TOKEN_FILE`, затем проверить scrape; drain также использует этот secret |
| `TURN_SHARED_SECRET` | Согласовать Coturn и выдачу credentials в приложении, учесть TTL старых credentials и проверить новые relay-соединения |
| Пароли PostgreSQL/Redis/RabbitMQ/MinIO | Сначала запланировать изменение учётной записи самой зависимости, затем всех клиентов; изменение env не меняет пароль в существующем persistent storage автоматически |
| `PROVIDER_TOKEN_ENCRYPTION_KEY` | Не заменять вслепую: существующий ciphertext станет нечитаемым. Автоматического re-encryption/key ring нет; нужен отдельный план миграции токенов либо отключения и повторного подключения, с сохранением ключа для разрешённого восстановления backup |
| Provider/OAuth credentials | Ротация у провайдера, затем в потребителях; учесть повторы jobs, refresh/revoke и готовность внешнего gateway |

Наличие инструкции не означает выполненную ротацию или проверенный production
release. Итоговое решение принимается по [чеклисту запуска](PRODUCTION_LAUNCH_CHECKLIST.md).
