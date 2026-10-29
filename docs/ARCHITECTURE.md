# Архитектура MeetSpace

Актуализировано 29 октября 2026 года: запись, встречи, сообщения и интеграции.

Документ описывает текущую модель одного хоста Docker Compose. Базовая конфигурация выпуска —
[`docker-compose.production.yml`](../docker-compose.production.yml);
локальный [`docker-compose.yml`](../docker-compose.yml) имеет дополнительные
публикации портов и не является staging. Процедуры выпуска и восстановления:
[развёртывание](DEPLOYMENT.md), [релизы](RELEASE_PROCESS.md), [откат](ROLLBACK.md).

```mermaid
flowchart LR
  Browser[Браузер] -->|HTTPS / WSS| Proxy[Публичный Nginx]
  Proxy --> Frontend[Frontend: статика Nginx]
  Proxy --> API[API: REST / WS / SSE]
  Proxy -->|Подписанное скачивание| Objects[(S3: MinIO или SeaweedFS)]
  Browser <-->|WebRTC| SFU[media-worker: SFU]
  Browser <-->|TURN / TURNS| TURN[Coturn]
  TURN <--> SFU
  API --> PG[(PostgreSQL)]
  API <--> Redis[(Redis)]
  API -->|record.start / record.stop через outbox| Rabbit[(RabbitMQ)]
  API -->|Внутренние команды сигнализации| SFU
  SFU <--> Redis
  Rabbit --> Recorder[record-worker: запись + compositor + FFmpeg]
  SFU -->|HTTP NDJSON: metadata + base64 RTP| Recorder
  Recorder --> Spool[(Локальный spool /storage)]
  Recorder --> PG
  Recorder <--> Redis
  Recorder --> Objects
  Product[product-worker: задания и scheduler] <--> PG
  Product --> Providers[Настроенные внешние провайдеры]
  Prometheus[Prometheus] -->|Закрытые metrics| API
  Prometheus --> Recorder
  Prometheus --> SFU
  Prometheus --> Product
  Grafana[Grafana: отдельный вариант поставки] --> Prometheus
```

## Процессы приложения

Порты в таблицах — внутри контейнеров, кроме явно указанных публикаций хоста.
Readiness проверяет готовность зависимостей, а не только наличие HTTP-процесса.

| Сервис | Ответственность | Порты / зависимости | Постоянные данные | Health | Масштабирование |
| --- | --- | --- | --- | --- | --- |
| `frontend` | React/TypeScript/Vite, статические файлы и same-origin API proxy | 8080/TCP; API | В образе, записываемых бизнес-данных нет | `/healthz`, `/version.json` | Несколько экземпляров возможны за балансировщиком с согласованной версией; текущий Compose содержит один |
| `api` | Авторизация, встречи, REST, WS/SSE, права, постановка команд и заданий | 8085/TCP; PostgreSQL, Redis, RabbitMQ, S3, media-worker | Устойчивые данные в PostgreSQL/S3; активные сокеты в памяти | `/health/live`, `/health/ready`, `/version` | Общие хранилища позволяют несколько процессов; текущий proxy и Compose не реализуют бесшовный rolling pool |
| `media-worker` | SFU: маршрутизация RTP, политики устройств, egress для записи | 8091/TCP только внутри сети; публичные 50010/UDP и TCP по Compose; Redis | Активные комнаты и PeerConnection в памяти; аренды в Redis | `/health/live`, `/health/ready`, `/version` | Уникальные worker ID, адреса и сетевые порты для каждого экземпляра; живая миграция комнат отсутствует |
| `record-worker` (Go record-worker) | Захват записи, compositor, FFmpeg, проверка и публикация результатов | 8090/TCP внутри; PostgreSQL, Redis, RabbitMQ, S3, media-worker | Spool `/storage` на выделенном томе; результаты в S3, метаданные в PostgreSQL | `/health/live`, `/health/ready`, `/version`, проверка диска и consumer | Ограниченные параллельные задачи, аренды с поколениями; каждому экземпляру нужны собственная идентичность и корректное владение spool |
| `product-worker` | Постоянная очередь заданий, напоминания, email/push и календари | 8092/TCP внутри; PostgreSQL и настроенные провайдеры | `background_jobs` и результаты доставки в PostgreSQL | `/health/live`, `/health/ready`, `/version` | Ограниченные пулы, аренды, `SKIP LOCKED` и идемпотентность внешних запросов |

Compositor встроен в `record-worker`; scheduler выполняется внутри существующих
процессов обработки. Отдельных сервисов compositor, scheduler, service mesh
или Kubernetes в текущем выпуске нет. `migrate` — одноразовая служебная задача
из API-образа, а не постоянно работающий сервис.

## Инфраструктура и хранение

| Сервис | Назначение / зависимости | Порты и публикация | Данные | Health и ограничение |
| --- | --- | --- | --- | --- |
| `proxy` | TLS, HTTPS/WSS, маршруты frontend/API/скачивания | Публичный 443/TCP | Конфигурация и сертификаты read-only | Проверка публичного HTTPS и выпуска через smoke; отдельный Compose healthcheck не задан |
| `coturn` | Релей WebRTC при невозможности прямого соединения | 3478/UDP+TCP, 5349/TCP; relay 49160–49259/UDP по умолчанию | Конфигурация, внешний secret и TLS-сертификат; живые allocation в памяти | HTTP health нет; требуется реальный forced-relay smoke, открытый порт этого не доказывает |
| `postgres` | Пользователи, права, встречи, переписка, уведомления и фоновые задания | 5432/TCP, только private network | Том `postgres` | `pg_isready` плюс прикладные запросы; одиночный экземпляр, автоматического failover нет |
| `redis` | Presence, Pub/Sub, leases, билеты, rate limits, временная доставка реакций | 6379/TCP, только private network | В production Compose persistence выключен | `PING`; потеря Redis требует восстановления сессий и аренды, не является потерей PostgreSQL-истории |
| `rabbitmq` | Доставка команд записи с подтверждениями | 5672/TCP; management 15672 не опубликован | Том `rabbitmq` | `rabbitmq-diagnostics ping` и consumer readiness; одиночный брокер |
| `minio` | S3-совместимое приватное хранилище записей, превью, архивов и вложений | 9000/TCP; у классического MinIO console 9001 закрыта | Классический том `minio`; hardened — `objects:/data` | Healthcheck зависит от overlay; S3-проверки нужны в обоих вариантах |
| `prometheus` | Метрики служб и правила alert | 9090/TCP, хост `127.0.0.1:19090` | Том `prometheus`, retention 7 дней в примере | `/-/ready`, состояние targets; профиль `observability`, без настроенной доставки alert сам по себе не уведомляет дежурного |
| `grafana` | Предоставленные dashboards | 3000/TCP, хост `127.0.0.1:13001` | Том `grafana`, provisioning, внешний пароль | `/api/health`; профиль `observability`, доступ через контролируемый канал; не включена в текущий hardened-комплект |

Однохостовая схема имеет общие точки отказа: сам хост/диск, proxy, БД, Redis,
брокер, object storage и единственный media-worker. Имена томов не означают
резервирование или репликацию. Горизонтальное увеличение API само по себе не
устраняет эти зависимости и не увеличивает сетевую ёмкость SFU.

## Варианты поставки и локальные файлы

`install.sh` по умолчанию запускает [`from-images.py`](../scripts/install/from-images.py):
скачивает совместимый проверенный комплект готовых образов из GitHub Releases.
Отдельный режим `--mode source --ref main` запускает [`from-source.py`](../scripts/install/from-source.py)
и собирает комплект `hardened-shared-host` на сервере. Его S3-сервис по-прежнему называется
`minio`, но образ [`dockers/object-storage/Dockerfile`](../dockers/object-storage/Dockerfile)
содержит **SeaweedFS**. Имена `MINIO_*` и S3 SDK сохранены ради совместимости.
Установщик объединяет production Compose, `docker-compose.hardened.yml` и
сгенерированные `config/compose.json`, `config/turn.yml`; последний включает
host network для Coturn. Название layout не означает автоматического подключения
`docker-compose.shared-host.yml`. Классический Compose с MinIO также остаётся в
репозитории; содержимое его тома не переносится автоматически в `objects`.
В текущем hardened-комплекте есть Prometheus; Grafana исключена из этого комплекта.

В production `record-worker` работает как UID/GID 1000 и получает bind mount
`${RECORDER_STORAGE_DIR}:/storage`. `/storage/records/{recordId}` — локальный spool
сырьевых IVF/Ogg, manifests, сегментов и готовящихся файлов. Это отдельное
хранилище от S3. После успеха оно очищается при `RECORDING_KEEP_LOCAL=false`;
после ошибки сохраняется для разбора/восстановления финализации. Автоматический
семидневный retention S3 не чистит failed spools. Legacy RTP-порты record-worker в
production отключены (`WEBRTC_UDP_PORT=0`, `WEBRTC_TCP_PORT=0`); браузер работает с SFU.

## Границы ответственности и завершение

API управляет правами и сигнализацией; RTP между участниками передаёт SFU.
FFmpeg не работает в обработчике API и не декодирует медиа внутри SFU.
Отдельные ограниченные очереди отделяют запись от живого звонка.
Go record-worker получает `application/x-ndjson` от SFU: `hello`, `track`,
`rtp` (байты RTP как base64 в JSON), `track.end`, `ping`, `error`. RabbitMQ доставляет
только команды записи. API фиксирует запись и `recording_outbox` атомарно; dispatcher
публикует команды с подтверждением. События состояния идут через Redis и conference WS.
`product-worker` использует другую постоянную очередь — `background_jobs` в PostgreSQL.
Подробнее:
[запись](conference-recording.md), [SFU](media-sfu.md), [интеграции](content-integrations.md).

Секреты имеют разные назначения: `MEDIA_TICKET_SECRET` подписывает внутренние
билеты, `MEDIA_INTERNAL_SECRET` защищает media control/egress, `WORKER_INTERNAL_SECRET`
защищает legacy API record-worker, `METRICS_SECRET` — metrics/drain. В production они
задаются отдельно; `TURN_SHARED_SECRET` остаётся серверным секретом, браузер получает
только временные TURN credentials. JWT/билеты/SDP/полные provider payload не должны
попадать в журналы.

Внутренний `POST /operations/drain` с `METRICS_SECRET` снимает readiness и
останавливает приём новых работ там, где подключён обработчик сервиса.
`GET /operations/drain` показывает `draining`, `active`, `ready` без идентификаторов
задач. Возврата этого процесса в ready после drain нет. `active` учитывает
незавершённые прикладные HTTP-запросы и подключённый счётчик работ сервиса;
у API к нему относятся локальные WebSocket-подключения. Это проверка наличия
работы в конкретном процессе, а не общая метрика пользователей всего кластера.

Перед заменой процесса оператор ждёт завершения активной работы. SIGTERM запускает
ограниченное по времени завершение; стандартный `SHUTDOWN_TIMEOUT` — 20 секунд,
Compose grace period — 45 секунд. Длинная встреча или запись может не успеть
закончиться. Принудительная замена SFU потребует переподключения клиентов, а
прерванный активный capture нельзя объявлять непрерывной успешной записью.
Готовые закрытые сегменты и аренды позволяют восстанавливать предусмотренные
этапы финализации, но не обещают бесшовное продолжение захвата.

Публичный proxy закрывает `/health`, `/metrics`, `/internal`, `/operations` и
`/debug`. `/version` и `/version.json` раскрывают только проверенные метки артефакта.
Разрешения пользователя и admin проверяются сервером; интерфейс не является
границей безопасности. Состав передаваемых данных — [DATA_FLOWS.md](DATA_FLOWS.md).

## Контракты и пределы

Точки входа и текущие ограничения собраны в [API.md](API.md). Фактические значения
задают [`internal/config/`](../internal/config), проверенный env-файл выпуска
и router/usecase ограничения; увеличение лимита не подтверждает ёмкость оборудования.

Сводные лимиты и правила подбора ресурсов — [CAPACITY_BASELINE.md](CAPACITY_BASELINE.md).
Число участников, параллельных записей и размер очередей ограничивают работу
процессов, но сами по себе не определяют пропускную способность хоста.

## Личные сообщения

Conference chat и личные беседы используют общий прикладной адаптер
с раздельными таблицами и проверками доступа. `domain/chat`,
`usecase/chat`, SQL-операции ChatRepository, HTTP handler и React MessageThread
общие. Различаются таблицы, проверка членства и publisher/transport.
Для личной беседы заданы уникальная пара пользователей и отложенная проверка
ровно двух участников. Account WS доставляет committed
изменения через персональные каналы Redis и имеет отдельное присутствие
аккаунта; Pion/recorder в этом пути отсутствуют. PostgreSQL — источник истории,
Pub/Sub не гарантирует доставку. Модель, lifecycle, API и ограничения описаны
в [PERSONAL_MESSAGES.md](PERSONAL_MESSAGES.md).

## Точки входа и пакеты

`cmd/api`, `cmd/media-worker`, `cmd/record-worker`, `cmd/product-worker`
запускают соответствующие функции в `internal/app`. Сигнализация находится в
`internal/usecase/media` и `internal/transport/mediaworker`; SFU —
`internal/infrastructure/sfu`; запись — `internal/usecase/recordings`,
`internal/usecase/recorder`, `internal/infrastructure/composite` и
`internal/infrastructure/ffmpeg`.
Продуктовые очереди и интеграции подключает `internal/app/product.go`.
