# Эксплуатация MeetSpace

> При обновлении старой установки сначала выполните [переход на record-worker и миграцию 000037](speech-removal-upgrade.md). Миграция необратимо удаляет данные функций речи; требуются проверенная резервная копия и остановка старых процессов. Простой откат образов после неё не поддерживается.

Runbook для одного Docker Compose-хоста: проверка служб, сеть, управление
нагрузкой, диагностика, резервное копирование и восстановление.

## Выпуск и навигация

- [Развёртывание и окружения](../DEPLOYMENT.md): хост, конфигурация, сеть, secrets и local rehearsal.
- [Процесс выпуска](../RELEASE_PROCESS.md): immutable package, CI gates, ledger и production approval.
- [Откат](../ROLLBACK.md): предыдущие образы и совместимость с текущей схемой.
- [Backup/restore](backup-restore.md): write freeze, проверка копии и границы восстановления.
- [Production checklist](../PRODUCTION_LAUNCH_CHECKLIST.md): актуальное решение go/no-go.
- [Проверки frontend](frontend-release-checks.md): сборка, браузеры и пользовательские сценарии.
- [Отказные проверки](FAILURE_TESTS.md): сценарии для изолированного стенда.
- [Архитектура](../ARCHITECTURE.md), [потоки данных](../DATA_FLOWS.md), [API](../API.md), [пользовательское руководство](../USER_GUIDE.md).

Для установки через `install.sh` используйте `manage.sh` и её полный
[Compose-контекст](../DEPLOYMENT.md#compose-контекст-установки-мастером). Общие
release helpers выбирают classic/shared-host и автоматически не читают
`config/compose.json` и `config/turn.yml` мастера. Обновление standalone требует
плана с сохранением её томов, сети, proxy/TURN и образов; `--resume` не updater.

Dev-сборка описана в [корневом README](../../README.md). На release-хосте используются закреплённые
образы, `AUTO_MIGRATE=false` и явная migration job до deploy, не DDL при старте.
Применённые SQL-файлы и их комментарии не редактируют: ledger сверяет байты.
В частности, индексы `000021_admin_capability.up.sql` требуют замера времени и
блокировок на репрезентативной копии, а не предположения по локальному запуску.

Внутренний транспорт на одном доверенном Compose-хосте — HTTP/AMQP/Redis без
TLS, не опубликованный наружу. Между отдельными хостами нужны TLS/mTLS/VPN и
отдельный security review. Значения CPU/RAM в Compose — начальные защитные
ограничения, а не результат production capacity planning.

## Порты и границы сети

| Назначение | Порт | Доступ |
|---|---|---|
| HTTPS/WSS | 443/TCP | Браузеры → Nginx |
| API | 8085/TCP | Только Compose/LB внутри доверенной сети |
| media-worker control | 8091/TCP | Только API/recorder, Bearer service secret |
| recorder control | 8090/TCP | Внутренняя сеть, Bearer service secret |
| product-worker | 8092/TCP | Health/operations только внутри сети |
| SFU ICE mux | 50010/UDP и TCP по умолчанию | Публичное назначение `MEDIA_NAT_IPS`, прямой RTP |
| TURN | 3478/UDP + TCP | Браузеры → Coturn |
| TURNS | 5349/TCP | TLS fallback, отдельный действительный сертификат |
| TURN relay | 49160–49259/UDP по умолчанию | Coturn → SFU; диапазон должен совпасть с firewall |
| PostgreSQL / Redis / Rabbit / S3 | 5432 / 6379 / 5672 / 9000 | Не опубликованы в production |
| ACME | 80/TCP | Выпуск/продление Let’s Encrypt в установке мастером; постоянного HTTP redirect нет |
| Metrics / pprof | `/metrics` / loopback `PPROF_PORT` | Метрики требуют Bearer; pprof выключен по умолчанию |

RTP не проходит через HTTP reverse proxy. TCP fallback здесь означает TCP между
браузером и TURN; TURN→SFU обычно остаётся UDP. Не закрывать этот внутренний UDP
маршрут. Перенаправить SFU mux 1:1 через NAT, без случайной смены внешнего порта.
Для `MEDIA_UDP_MIN_PORT/MAX_PORT` вместо mux нужен явный соответствующий диапазон
в Compose/firewall; production пример использует именно один mux.

Coturn production запрещает private/loopback/link-local/multicast peer addresses,
чтобы не стать прокси к внутренним сервисам. Если TURN и SFU на одном хосте и
hairpin NAT недоступен, разрешить **только конкретный** адрес SFU через
`allowed-peer-ip`, а не весь private CIDR. Тестовый стенд имеет отдельные
послабления для loopback и никогда не используется как production конфигурация.

`HTTP_TRUSTED_PROXIES` пустой по умолчанию: forwarded IP не доверяются. Для
production задать только фактический IP/CIDR Nginx/LB, иначе IP rate limits могут
считать весь трафик одним proxy-клиентом. При внешнем LB нужно согласовать цепочку
доверия; Nginx пример сам устанавливает `X-Forwarded-For=$remote_addr`.

## TURN и ICE

`GET /api/v1/webrtc/config` доступен только с пользовательским JWT. Ответ содержит
`iceServers`, `iceTransportPolicy` и Unix `expiresAt`, без shared secret. Username
— `expiry:random UUID`; password — Base64(HMAC-SHA1(shared secret, username)),
совместимый с [Coturn REST](https://github.com/coturn/coturn/blob/master/README.turnserver).
Это не алгоритм хеширования паролей пользователей. TTL по умолчанию 10 минут,
допустимый диапазон 1–60 минут. Часы API/media-worker/Coturn должны быть
синхронизированы. Для нового/recreated PeerConnection браузер получает новые
credentials через `media.joined`. Автоматического ICE restart/обновления TURN
credentials внутри уже живого PeerConnection пока нет; долгие встречи нужно
проверить через полный TURN allocation refresh на целевой сети.

Candidate `host` — локальный адрес, `srflx` — адрес после STUN/NAT, `relay` —
выделенный Coturn адрес. При диагностике смотреть **выбранную** пару, а не только
наличие relay кандидата. `TURN_FORCE_RELAY=true` применяется к новым соединениям.
Shared secret никогда не передаётся браузеру. Для ротации ключа согласовать
перекрытие старого/нового секрета на Coturn и TTL на API, иначе новые подключения
или refresh могут прекратиться. Синхронная ротация без перекрытия — операция с
возможным разрывом соединения, не бесшовная.

При `401/438` проверить часы, общий ключ, realm, TTL и transport URI. При timeout
проверить DNS, 3478/5349, relay диапазон, public address, Docker NAT и разрешённый
маршрут relay→SFU. TURNS требует доверенную цепочку и соответствие имени
сертификата. Принудительный relay проверен локально по UDP/TCP; WAN, мобильная и
корпоративная сети, реальный TLS сертификат проверяются оператором отдельно.

## Health и завершение

`/health/live` отвечает 200, пока процесс жив; сбой БД не делает liveness ложным.
`/health/ready` отвечает 200 или 503. Пробы выполняются каждые 2 секунды с общим
дедлайном 1 секунда, а не по каждому запросу health. Тексты ошибок/адреса/ключи
не публикуются. Readiness API: PG, Redis, Rabbit, MinIO; media: Redis и lease
ownership; recorder/compositor: PG, Redis, Rabbit consumer, MinIO и локальный диск.
Compositor встроен в recorder, отдельного процесса/health порта у него нет.
Product проверяет PostgreSQL.
Readiness внешнего provider нельзя считать доказанным только по этим probes.

Для установки мастером сначала задайте функцию `compose` из
[DEPLOYMENT.md](../DEPLOYMENT.md#compose-контекст-установки-мастером), затем
выполняйте только строки `compose ps`/`compose exec` ниже.

Следующий полный блок предназначен для общего release workflow: отдельный Bash
из корня checkout после выбора **установленного** manifest. Helper читает env как данные Compose,
не исполняет его shell-код и не выводит конфигурацию с секретами:

```bash
bash
source scripts/release/common.sh
parse_release_args --environment production --project recorder-production --env-file /srv/meet/config/production.env --manifest /srv/meet/releases/v1.2.3/release.json
validate_release
compose ps
compose exec -T api wget -qO- http://127.0.0.1:8085/health/ready
compose exec -T media-worker wget -qO- http://127.0.0.1:8091/health/ready
compose exec -T record-worker wget -qO- http://127.0.0.1:8090/health/ready
compose exec -T product-worker wget -qO- http://127.0.0.1:8092/health/ready
```

Замените пути/версию на фактические. Для local/staging выберите их отдельный
project/env/manifest. После диагностики `exit` закрывает этот Bash. Эти read-only
команды не являются обходом approval для deploy/migrate/drain/resume.

Перед заменой процесса release helper вызывает закрытый `/operations/drain`,
запрещает новую работу и ждёт `active=0`. В счётчик входят прикладные HTTP и
работа роли: WS, комнаты, записи или jobs. Timeout оставляет процесс draining,
не перезапускает его силой. Возврат в ready — только пересоздание через `resume`
с тем же manifest. Во внешнем LB/admission также ограничить новую нагрузку.
SIGTERM устанавливает draining/readiness=false, API перестаёт принимать
новые запросы и закрывает hijacked WS, media освобождает комнаты/PC/leases,
recorder завершает обработку в ограниченное время. `SHUTDOWN_TIMEOUT=20s`, Compose
grace period 45s. После дедлайна процесс может завершиться без полной финализации,
закрытые фрагменты сохраняются. Live migration комнат между SFU не реализована:
при смене media-worker требуется reconnect, повторное получение ticket и новые PC.
Redis PubSub ephemeral; клиент после reconnect получает HTTP snapshot/history.

### Возврат standalone после drain

В установке мастером используйте её функцию `compose`, не `release.sh resume`.
Перед перезапуском завершите встречи/записи и фоновые работы, включите drain и
проверьте `draining=true`, `active=0` у всех заменяемых процессов. Управляющий
POST и GET для API выполняются внутри контейнера без вывода секрета:

```bash
compose exec -T api sh -c 'wget -qO- --post-data="" --header="Authorization: Bearer $METRICS_SECRET" http://127.0.0.1:8085/operations/drain'
compose exec -T api sh -c 'wget -qO- --header="Authorization: Bearer $METRICS_SECRET" http://127.0.0.1:8085/operations/drain'
```

Повторите с service/port `media-worker:8091`, `record-worker:8090`,
`product-worker:8092`. Только после нулевой активности,
проверки очередей и завершения backup/обслуживания верните те же image IDs:

```bash
compose up -d --no-build --no-deps --force-recreate --wait --wait-timeout 240 \
  media-worker record-worker product-worker api frontend
```

Этот блок возобновляет установленную версию: он не собирает новый выпуск и не
обновляет инфраструктуру. Если env/образы уже менялись, сначала сверяйте план
обновления. Повторно проверьте readiness/version, очереди и пользовательские
сценарии, затем снимайте внешнее ограничение нагрузки. Перезапуск SFU прерывает
соединения, поэтому его выполняют после завершения встреч.

Для recorder перед drain сначала прекратить появление новых `record.start` и
дождаться обработки уже ожидающих стартов, сопоставляя очередь/outbox со
статусами записей. Уже активные записи остановить штатно и дождаться финализации;
сделать это до общего drain API, который отклоняет новые прикладные запросы.
`active=0` — счётчик процесса, не доказательство пустой Rabbit-очереди.
Во время drain consumer возвращает новые старты через `Nack(requeue=true)` с
паузой 250ms. Start/stop используют одну очередь: повторяемый start способен
задерживать stop; отдельной retry-топологии или приоритета stop пока нет.
У composite остановка сохраняется в SQL и замечается владельцем при опросе,
но это не заменяет проверку завершения. Если активность не исчезла, timeout
безопасно прекращает deploy, оставляя процесс draining; не обходить его force restart.

## Диагностика и метрики

JSON логи имеют `service`, `instance_id`, `event_type`; HTTP получает UUID
`X-Request-ID`, внутренние media команды и Rabbit сообщения сохраняют correlation
где применимо. WS media использует UUID конверта как correlation ID; это не
тождественно ID HTTP handshake. Entity IDs допустимы в ограниченных логах,
**не** в metric labels. SQL query logging выключен, чтобы не писать текст чата.
Не включать dump SDP, JWT, tickets, upload/download query или тела чата.

`/metrics` без `Authorization: Bearer METRICS_SECRET` возвращает 404. Публичный
production Nginx закрывает metrics/internal/debug/health и drain. `release.sh
deploy` поднимает observability-профиль из того же immutable manifest; отдельно
пересобирать или запускать mutable образы метрик при выпуске не нужно.

Предварительно положить token в `METRICS_TOKEN_FILE`, совпадающий с METRICS_SECRET.
Для classic-варианта нужен отдельный пароль Grafana в `GRAFANA_PASSWORD_FILE`.
Установка мастером создаёт token автоматически; hardened не запускает Grafana. Файлы вне Git, доступны
пользователю контейнера; не сохранять значения в dashboard или Prometheus YAML.
Prometheus доступен только на loopback 19090, Grafana classic — 13001; используйте SSH tunnel.
`dockers/observability/` содержит starter dashboard и alert rules. Порог DB 18
соответствует pool 20; при изменении пула скорректировать alert. Общие CPU/RAM
пороги требуют настройки под роль службы. Доставка alert через Alertmanager
не настроена; необходимо подключить свой канал уведомлений.

Основные серии: `recorder_http_requests_total{route,method,status}` (401/403 auth,
429 limits), `recorder_http_duration_seconds`, `recorder_ws_active`,
`recorder_ws_messages_total{class}`, `recorder_dependency_up{dependency}`,
`recorder_events_total{event}`, `recorder_work_duration_seconds{operation}`,
`recorder_state{resource}`, Go/process collectors. Route — шаблон, не URL с ID.
State содержит DB pool, media rooms/peers/tracks/subscriptions/RTP, активные
recordings/FFmpeg, disk free и последнюю queue latency. Счётчики SFU представлены
монотонными snapshot gauges, при рестарте обнуляются; использовать `increase`
с пониманием reset. Probe failure count — число неудачных проб, не число всех
ошибок запросов зависимости. `media_relay_total` / `media_direct_total` учитывают
первую выбранную пару физического подключения: TURN на любой стороне — relay,
host/srflx/prflx — direct. Это не статистика переключений пары внутри одного PC.
Waiting/online глобально и Rabbit queue depth пока не экспортируются; queue depth
смотреть в брокере.

Продуктовые jobs имеют `recorder_product_queue`, `recorder_product_jobs_total`
и provider-метрики. Это не Rabbit depth. Опциональная frontend-телеметрия добавляет
`recorder_client_errors_total{code,browser,route}` с ограниченными значениями;
она не содержит пользовательский контент и не измеряет всех пользователей:
disabled-флаг, rate limits и ошибки доставки уменьшают наблюдаемую выборку.

Pprof: `PPROF_PORT=0` по умолчанию; ненулевой порт слушает `127.0.0.1` **внутри**
контейнера. Не добавлять port mapping или публичный proxy маршрут. Временный
admin доступ через `docker compose exec`/внутренний namespace. Сервер отклоняет
`seconds` вне (0,60], включая NaN/Inf; запросы ограничивать 30–60 секундами.
Выключить порт после диагностики.

## Лимиты и хранилище

WS physical limit 1000/процесс, queue/message/SDP/ICE limits — `WS_*`. SFU:
`MEDIA_MAX_PEERS/ROOMS/PUBLISHED_TRACKS/AUDIO_TRACKS/VIDEO_TRACKS/SCREEN_SHARERS`,
очереди RTP и deadlines — `MEDIA_*`. Recorder: `RECORDING_MAX_ACTIVE`,
`RECORDING_FFMPEG_CONCURRENCY`, `MAX_DURATION`, `MAX_MIB`, `DISK_RESERVE_MIB`.
Legacy ingest также ограничен MaxActive. JSON max 1 МиБ, API read timeout 60s;
upload route имеет отдельный лимит 10 МиБ и собственный deadline. SSE/WS имеют
отдельные heartbeat/write deadlines, поэтому общий HTTP WriteTimeout не используется.

Значения шаблона PG pool — 20 open/10 idle/30m lifetime; текущий мастер
задаёт 10 open/2 idle. Сверяйте фактические DB_MAX_OPEN/DB_MAX_IDLE перед
настройкой оповещений. Подключение 5s, query timeout 10s,
lock timeout 5s. Migration job имеет отдельные transaction/advisory lock,
`lock_timeout=15s` и deadline 5 минут. Не увеличивать лимит всех пользовательских
запросов ради DDL; тяжёлую миграцию проектировать и проверять отдельно.
Redis dial/read/write/pool 3s, context cancellation; Rabbit heartbeat 5s,
connect bounded, publisher confirms/reconnect, consumer QoS=1/reconnect.
Команда: одна повторная доставка, затем durable `.failed` queue (7 суток,
10000 сообщений и 64 МиБ). Повторный старт/стоп должен оставаться идемпотентным.
Общие business ошибки тоже попадают в retry/quarantine; не переотправлять DLQ
вслепую. Сначала исправить причину и проверить состояние record в PG.

Для backlog alert подключить собственный Rabbit management/exporter в закрытой
сети: следить за messages_ready/unacknowledged, consumers=0 и возрастом команды.
Начальный рекомендационный порог messages_ready>100 в течение 5m требует
пересмотра по длительности записей и нагрузке; он не включён как «работающий»
Prometheus rule без соответствующей серии/exporter.

В development/classic работает MinIO; в hardened и текущей установке мастером
— SeaweedFS под тем же service name `minio` и `MINIO_*`. S3-клиент общий,
но формат томов разный: подключать MinIO data volume к SeaweedFS нельзя.
`S3_MAX_VOLUMES=16` — default SeaweedFS, размер volume задаётся entrypoint как
128 МиБ; ёмкость планируется отдельно от размера диска хоста.

S3-операции имеют сетевые deadlines, bounded SDK retry, upload deadline 10m,
проверку размера сохранённого объекта. При частичной composite финализации
незакоммиченный token-prefix удаляется best effort (5s). При outage очистка может
не пройти; сверять такие orphan objects с PG перед удалением. Повторный upload
одного object key идемпотентен, ready ставится только после подтверждения артефактов.
Бакет приватный; не включать anonymous access. Ссылки имеют TTL и проверку доступа.

FFmpeg вызывается напрямую, без shell. Новые bounded wrappers сохраняют до
64 КиБ вывода. Cancel: SIGTERM, через 3s SIGKILL; legacy normal stop сначала
отправляет `q`, затем TERM/KILL с ограниченным ожиданием. Контейнер `init: true`
помогает reap дочерних процессов. Codecs: VP8/Opus ingest, H.264/AAC MP4 для composite/screen_focus и JPEG
preview; audio_only формирует AAC в MP4 без preview, individual_tracks —
аудио MP4 и ZIP исходных IVF/OGG с manifest. Runtime FFmpeg должен содержать
libvpx/libx264/libopus/AAC. Конференционный путь: SFU → HTTP NDJSON egress →
recorder → FFmpeg → S3; RabbitMQ передаёт команды, а не RTP.
Low disk снимает readiness, но не является полноценной квотой записи в процессе:
следить за диском и заранее прекращать admission. Приложение не удаляет чужие
или неопознанные каталоги при запуске; завершённые chunks нужны для восстановления.
Composite очищает spool после upload и commit ready, если
`RECORDING_KEEP_LOCAL=false`. Артефакты сохраняются под
`recordings/{conferenceId}/{recordId}/artifacts/{token}/`; устаревшие keys
`records/{recordId}/final.mp4` не описывают текущий конференционный путь.

## Аварии и восстановление

[Матрица отказных проверок](FAILURE_TESTS.md) описывает, как проверить
восстановление на изолированном стенде.

При recording failed не подменять статус success. Проверить record-worker readiness,
lease/token, свободный диск, retained chunks, FFmpeg stderr, MinIO и outbox.
Перезапуск может финализировать завершённые фрагменты, но не восстановит потерянный
RTP или незакрытый хвостовой chunk. Не копировать файлы между record/token
каталогами без проверки ownership. Для Rabbit backlog:

```bash
compose exec -T rabbitmq rabbitmqctl list_queues name messages_ready messages_unacknowledged consumers
```

PG saturation: посмотреть pool metrics/lock waits, `pg_stat_activity` в закрытом
admin-сеансе без вывода SQL с приватным контентом, active long transactions;
не начинать с массового увеличения max_connections. Redis outage: восстановить
Redis, дождаться новых leases, клиенты reconnect+snapshot; PubSub не replay.
MinIO outage: восстановить storage/диск/credentials, сверить PG status и objects.

## Backup, restore и rollback

Актуальные команды и границы — [backup-restore.md](backup-restore.md) и
[ROLLBACK.md](../ROLLBACK.md). Общий backup helper привязан к release workflow;
для standalone используйте отдельный контекст и инструкции backup. Сохраняются
PG и текущие S3 objects
с `ContentType`, не историю версий, произвольную metadata или весь broker/spool.
Глобальный write freeze обязателен: это не атомарный cross-store snapshot.
Secrets/config/TLS и ключ provider-токенов сохраняются отдельно защищённо.
Redis presence/ownership/PubSub не восстанавливают как живые старые leases.

Restore drill создаёт отдельные БД/bucket и сохраняет их для осмотра. Hash/privacy
и signed access проверяются, но бизнес-связи и playback требуют дополнительной
приёмки. Локальные длительности не задают production RPO/RTO. Application rollback
запускает предыдущие immutable images на совместимой текущей схеме, без DB down
и без пересборки старого commit. Несовместимость схемы требует отдельного решения.

## Ручная проверка после выпуска

Сверить `/version` и `/version.json` с выбранным манифестом; через закрытый доступ
проверить health/readiness, drain и очереди каждого изменённого сервиса.
С согласованными существующими аккаунтами проверить вход, WS/reconnect и доступ
к материалам. Медиа и TURN/TURNS проверить отдельно на целевых сетях с согласия
участников. Новые встречи/записи не создаются автоматически при публикации.
Сохранить реальные результаты и ограничения; health/version не доказывают
успешность RTP, FFmpeg или playback. Пользовательские данные и volumes не удалять.

Актуальные CI jobs, security/backup/rollback gates и ручные проверки описаны в
[RELEASE_PROCESS](../RELEASE_PROCESS.md) и [чеклисте](RELEASE_CHECKLIST.md).

## Предлагаемые SLI/SLO и окно наблюдения

Выберите SLI и измерьте их при ожидаемой нагрузке целевого окружения. На основе
этих данных определяются SLO и допустимое число одновременных встреч.

| Область SLI | Что измерять перед назначением SLO |
| --- | --- |
| HTTP/auth | Число реальных запросов и 5xx, p95 по маршрутам без long-lived WS/SSE; отдельно ожидаемые 401/403/429 |
| WS/media | Попытки join/reconnect и успешные подключения, время до выбранной ICE pair и приёма медиа; denominator фиксировать в smoke/browser harness, не выводить из одного gauge |
| Записи | Start/stop/finalization, failed/ready, время до доступного проверенного MP4/preview, backlog и число реально завершённых записей |
| Фоновые функции | Jobs queued/failed/retries, возраст работ, provider errors/rate limits, live audio drops; отделять отключённую функцию от неисправной |
| Ресурсы и зависимости | Readiness, DB pool/locks, CPU/RSS/goroutines/FD, диск/spool, PG/MinIO/Rabbit и выбранные relay/direct подключения |
| Frontend | Ограниченные error codes и версии с фактическим числом browser-сценариев; отсутствие событий не доказывает отсутствие ошибок |

До запуска задайте окно наблюдения, минимальный объём сценариев и условия
остановки. Сохраните фактическое число HTTP/login/WS/media/recording операций
за период. При недостаточном трафике продлите наблюдение или выполните
контролируемые проверки на стенде.

Стартовые alert rules находятся в `dockers/observability/alerts.yml`:

- Критичные: стойкая недоступность зависимостей/metrics, опасно низкий диск.
  Оператор также немедленно разбирает системную недоступность auth/WS/media и потерю данных.
- Предупреждения: HTTP 5xx/p95, saturation/рост ресурсов, SFU/FFmpeg failures,
  повторные jobs/DLQ/provider errors. Одиночный FFmpeg failure сейчас warning,
  а не автоматическая paging-эскалация или доказательство общего outage.
- Информационные: всплеск frontend errors — сигнал к сверке версии и сценария,
  не точное число пострадавших пользователей.

Порог HTTP 5xx >5%/p95 >1s, DB in-use ≥18 и другие числа в starter rules —
начальные защитные настройки, не обещанный SLO; пересмотреть под реальную выборку,
лимиты и роль сервиса. Настроить доставку, группировку, ответственных и проверку
уведомлений. До этого dashboard/alert rule не считается работающим on-call процессом.
