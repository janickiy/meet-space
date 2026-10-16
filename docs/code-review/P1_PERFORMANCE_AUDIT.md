# P1: производительность и эффективность

## Повторный аудит — 16 октября 2026

### 1. Executive summary

Проведён новый измеримый P1-аудит текущего кода: RTP/ownership, locks/channels, Redis, RabbitMQ, PostgreSQL, temporary storage и FFmpeg. Принята **одна production-оптимизация**: пакетная вставка участников при создании группы. Для 20 участников latency benchmark снизилась на **58.64%**, для 100 — на **70.44%** (по 6 samples, p=.002). SQL на создание группы: 23→4 и 103→4 соответственно. Роли, валидация аккаунтов, транзакция и идемпотентность сохранены.

Аудит завершён с явно указанными ограничениями. P0 lifecycle smoke и race проходят. Общий staticcheck остаётся красным из-за двух **существовавших до правки** U1000; это не полный зелёный quality gate. Результаты старых аудитов ниже — история, не baseline этого прохода. Деплой не выполнялся; P2 не реализовывался.

### 2. Baseline environment

- Исходный commit `a604c0d49ce5651f481fe0c0a144e5a049759ff4`, working tree был чистым. Go 1.26.9 darwin/arm64, macOS 27.0.1, Apple M5, 10 logical CPU, 24 GiB RAM. GOMAXPROCS=10, GOGC не менялся.
- Docker 29.4.2, Linux arm64 VM: 10 CPU, 16,748,032,000 B RAM, kernel6.12.76-linuxkit. PostgreSQL 17.10, Redis 7-alpine, RabbitMQ 4.2.9-management; сохранены точные image IDs. FFmpeg 6.1.2 в существующем образе `go-recorder-worker`, ID `sha256:d37e4a93b60ce872d856eb2accfcf9626a082a14a28e36eb80881a70ff54660d`.
- Собственные disposable PostgreSQL/Redis/RabbitMQ: отдельная сеть, случайные credentials, только loopback ports. Ни данные, ни настройки production не использовались. 11 исходных контейнеров пользователя продолжали работать; это shared host, а не изолированная лаборатория.
- До изменений: uncached `go test`, `go test -race`, vet, frontend Prettier — PASS. Постоянных Go tests в дереве нет: пустой зелёный `go test` **не заменяет** реальные opt-in проверки ниже. Staticcheck 0.7.0 — два U1000; govulncheck 1.8.0 — 0 symbol/package findings, одна module-only `GO-2026-5932` для неимпортируемого OpenPGP в x/crypto. Это не утверждение об отсутствии уязвимостей всех транзитивных зависимостей.

Ignored evidence: `tmp/p1-audit-20261016/` (profiles, SQL/plans, JSON metrics, logs, before source snapshot, overlays, source hashes). Проверка переносимых инструментов: `tmp/p1-repro-20261016/`. Точные команды и безопасное удаление стенда: [tools/performance-audit/README.md](../../tools/performance-audit/README.md). Бинарники baseline/after используют один harness, исходная группа подставляется через Go overlay без отката working tree.

### 3. CPU/heap/alloc profiles

Сняты CPU, heap, allocs, mutex, block и goroutine checkpoints для idle; 2/5/10 peers; двух комнат по5/10; screen+recording/audio egress; DB/read+JSON, Redis reconciliation и двух concurrent composite finalizers. `profile-summary/` содержит разобранные pprof tops. Отдельный pprof установлен во временную папку: текущий Go toolchain не включает `go tool pprof`.

Media — настоящие Pion ICE/DTLS/SRTP/UDP клиенты и сервер в одном процессе, payload160/1200 B, 50RTP/source/s; 22 s warmup, около5 s sample. Это encoded synthetic packets, не browser camera encode/WAN и не server-only capacity. Pion NACK history прогревается до ограниченного размера1024.

В 10×2 rooms: CPU8.1529 s за5.0004 s wall; runtime mallocs396,522/s,52.316 MB/s. CPU samples7.76 s: raw syscall60.44%, pthread wait8.76%; alloc-space delta283.98 MB, Pion ReadRTP78.44% cumulative. Часть расходов принадлежит тестовым receiving clients. Active heap384.02 MB; sampled heap93.80% в bounded retransmission packet factory. После cleanup live heap1.661 MB, G3/FD6, все maps пусты. RSS774.34 MB сохраняет allocator high-water и сам по себе не доказывает утечку.

DB profile3.24 s содержит290 ms Go CPU samples, alloc-space delta74.75 MB: `relatedByRecordIDs`42% cumulative, JSON9.44 MB cumulative. Серверный PostgreSQL CPU сюда не входит. Recording parent CPU profile за0.51 s имеет0 samples; CPU дочерних FFmpeg/ffprobe измерен отдельно через RUSAGE_CHILDREN — отсутствие Go samples не означает отсутствия работы.

### 4. RTP allocation/copy findings

Путь: `TrackRemote.ReadRTP → Manager.receive → bounded subscription queue → subscription.forward → TrackLocalStaticRTP.WriteRTP → NACK/SRTP/UDP`. Собственного N-кратного payload clone перед subscribers нет. `receive` создаёт snapshot указателей на subscriptions, не payload. `recordPacket` snapshot-ит указатели на sinks и один раз marshal-ит RTP для всех recording/audio consumers.

Targeted egress benchmark: без sinks0 alloc/op; при1 и2 sinks одинаково1 alloc/op —176 B для payload160,1280 B для payload1200. Для1200 B median около218ns/op с одним sink и251ns/op с двумя. Шесть samples по300 ms; это цена enqueue/marshal, **не полный forwarding latency**. SDK receive allocations доминируют; новый sync.Pool, reusable shared packet или custom RTP stack не обоснованы.

Expensive debug formatting и SQL/Redis на каждый пакет в inspected hot path не обнаружены. Per-packet counters — атомарные; high-cardinality packet labels не добавлялись. Сжатие уже кодированного RTP не включалось.

### 5. Buffer ownership и Audio/STT

| Участок | Ownership / основание |
| --- | --- |
| Pion ReadRTP | Packet-owned allocation, payload живёт дольше receive iteration |
| Header extensions | Producer изменяет до fan-out; дальше packet immutable |
| Subscriber queue | Shared immutable packet; async consumers исключают немедленный reuse |
| Pion WriteRTP | Pooled shallow header copy меняет SSRC/PT, payload не переписывается |
| NACK/RTX | Bounded copies per subscriber нужны для повторной передачи |
| Recording + audio egress | Один immutable marshaled buffer разделён consumers; нельзя вернуть его в pool до завершения обоих |
| PCM decoder | Отдельный FFmpeg/worker, не RTP goroutine; reusable PCM buffer допустим только на синхронном callback |

Subscriber queues default128/max4096, recording2048/max8192, audio taps≤256. Overflow неблокирующий; drop/cancel guards сохранены. Opus/STT decode не возвращён в SFU. Provider STT throughput/worker saturation **not measured**: synthetic egress вычитывается без внешнего STT, реальные decoders проверены отдельно на lifecycle.

### 6. Lock contention и channels

10×2 media delta: aggregate mutex delay230.01 ms, из него readRTCP179.67 ms, forward18.07 ms; room/registry snapshot не доминирует. Block delta7908.96 aggregate goroutine-seconds,99.99% в select. Сумма concurrent goroutine waits может включать завершившиеся ожидания, начавшиеся во время warmup; это **не** latency запроса и не CPU. Per-lock hold/frequency **not measured**: pprof не даёт этого распределения, per-packet instrumentation ради недоказанного hotspot не вносилась.

Registry/room snapshot locks отпускаются до fan-out/marshal; блокирующий channel send не добавлялся. Peer SDP barriers, Pion channel ownership и WS delivery authorization fences нельзя снимать механически. Rabbit publisher mutex упорядочивает общий channel и confirm. В DB `GroupMembers` использует `lockGroup`/FOR UPDATE даже на read:32×5 readers дают201.6 ms wall/2.022 aggregate pool-wait seconds; after196.2 ms/1.896 s. Этот path не менялся, ускорение не заявляется; безопасное ослабление требует отдельных revocation/concurrency контрактов. Mutex→RWMutex, sharding и замена channels не делались.

### 7. Redis reconnect/efficiency

Go-redis 9.19.0 владеет pool/command retries: max retries2, timeout3 s, randomized SDK backoff8–512 ms. Hub использует один acknowledged Pub/Sub recovery loop100 ms→2 s, дублирующего worker нет. Hub jitter отсутствует; prolonged outage/multi-replica herd **not measured**.

Пять CLIENT KILL своей subscription при здоровом Redis: recovery readiness+доставка104.122–107.421 ms. Missing500:1 pipeline request/500 commands,≈0.946–0.988 ms/op,167256 B/op,2521 alloc/op. Online1/20/100:1 request/1Lua command, single observations303.8/310.3/449.1 µs; чтение не продлевает TTL. Reconciliation использует real Redis + synthetic Stale repository; SQL этого сценария не включён. Никаких per-RTP keys/TTL writes. Production Redis не менялся.

### 8. RabbitMQ reconnect/prefetch/throughput

Confirmed serial publish345.944–431.742 µs/op,≈2874 commands/s по median; parallel341.393–358.875 µs/op. Go allocations65/op,≈2875 B/op. Publisher reconnect4.983–5.786 ms; consumer3 fault recovery1.008–1.016 s до первой наблюдаемой доставки (возможна redelivery). Это не FFmpeg jobs/s и не длительный outage SLA.

Connection/channel переиспользуются, confirms сохранены, manual ack/nack и quarantine≤65536 B. QoS/prefetch1 заново применяется после reconnect и соответствует sequential FFmpeg handler. Queue содержит команды/ссылки, не media/transcript blobs. Consumer retry фиксированный1 s без jitter; sustained outage/multi-worker herd остаётся отдельной P1-проверкой. Новых broker optimizations нет.

### 9. PostgreSQL pool

Текущие MaxOpen20/MaxIdle10, lifetime30min, MaxIdleTime unset; connect timeout5 s, statement timeout10 s (DB_QUERY_TIMEOUT1 s..1min), lock timeout5 s. На disposable PG max_connections100. Конкурентный group-chat read32 clients×5: peak20 open/20 in-use; final10 open/0 in-use/10 idle,0 errors. До133 waits/1.089 aggregate s/111 ms wall, после131 waits/1.214 s/122 ms; p95 47.498→55.963 ms. Read path не изменён; не объявляется выигрыш или доказанная регрессия.

Production compose содержит4 DB-процесса: API, worker, product-worker, live-worker. Media-worker использует Redis без PostgreSQL. После Connect четыре DB-процесса применяют Operations config (default DB_MAX_OPEN=20, DB_MAX_IDLE=10, DB_CONNECTION_LIFETIME=30m). Сумма default pools4×20=80 оставляет20 из PG100 до учёта миграций/диагностики и других клиентов; shared-host override PG60 уже ниже этой верхней суммы. Это статически подтверждённый конфигурационный риск, **не воспроизведённый production outage**. Одновременная saturation всех реплик/ролей **not measured**. Увеличивать pool до100 либо менять лимиты без этого эксперимента не обосновано.

### 10. N+1/query findings

Read page query counts не растут per row в изученных batch loaders: recordings1/20=7/7, history6, participants20=4, conference chat20=8, personal list20=2, personal unread group20=2, group/direct chat20=10, group members20=3, notifications20=2, transcript20=5, summary4, analytics4, admin3. Полные p50/p95/p99 и SQL находятся в [P1_BEFORE_AFTER.md](P1_BEFORE_AFTER.md) и evidence. Для replies/attachments fixture пустой: сохранённые10/8 query counts не являются измерением heavy attachments/reply fan-out; source audit показывает batched loads.

Обнаружен write N+1: group CreateGroup2/20/100 участников =5/23/103 SQL. Все аккаунты уже проверены одной блокирующей выборкой, но membership INSERT выполнялся отдельно на каждого. Исправлено одним bounded INSERT, теперь4 SQL для каждого размера. Read exact unread count и ACL predicates не менялись.

Recordings20 всё ещё загружает full segment/event metadata:≈1.253 MB/op repository и≈1.984 MB/op Gin+JSON. Более лёгкая projection требует проверки API-клиентов и DTO-контракта, поэтому поля не удалялись ради bench. Постоянный round-trip count не означает постоянный PostgreSQL CPU: personal list сканирует50,950 messages для latest/unread aggregation.

### 11. EXPLAIN/index changes

Сохранены exact emitted SQL + EXPLAIN(ANALYZE,BUFFERS) всех17 read workloads. Recording20 использует существующие cursor/relation indexes: largest measured relation plan≈0.068 ms; in-memory sorts25–65KiB, disk spill не наблюдался. Group/direct chat применяют существующий cursor index; scans маленьких/пустых fixtures не повод для нового индекса. Personal list top-N sort36KiB и seq scan50,950 messages: first plan8.527 ms, unread aggregate12.214 ms. Personal unread group EXISTS уже был реализован до этого аудита; не приписывается новой правке.

Наиболее медленный read — notifications20: p50≈118 ms, p95≈131 ms. Unread-count EXPLAIN128.368 ms, JIT127.906 ms,53 functions; дорогие estimates из `id::text = payload`/OR приводят к JIT даже при fixture non-chat types и never-executed subplans. Это измеренная проблема; chat.message-heavy distributions/invalid payload/ACL revocation regressions **not measured**. Без них безопасная indexable UUID/rewrite стратегия пока не доказана. Глобально JIT не выключался; индексы и миграции не добавлялись. Следующий P1 эксперимент должен сравнить это точечно, сохраняя mute/leave/clear/delete правила.

### 12. Temp-file/disk I/O

| Поток | Create/write/consume/cleanup |
| --- | --- |
| Composite sources/chunks | Per-record directory, RTP source files + JSON manifests, durable fsync, compose consumes; retained on recoverable failure, cleanup после успешной публикации |
| Final MP4/preview | Per-record intermediates, validate/preview/checksum, streamed upload; recovery artifacts не удаляются для экономии I/O |
| Individual tracks ZIP | Per-record archive, io.Copy/LimitReader+context; нет whole-media ReadAll |
| STT extraction | Private job tempdir, MinIO stream → bounded io.Copy/LimitReader file → FFmpeg decode; once cleanup, нет media→fullRAM→disk |
| Checksums/uploads | SHA256 io.Copy и потоковый storage API |

Manifest ReadFile читает metadata, не video. Hard byte/count bound для recovery manifest Glob/ReadFile не найден: при очень долгой/повреждённой записи возможен отдельный memory/work spike, это оставшаяся проверка. Большие avatar/attachment ReadAll ограничены соответствующими ingress limits; этот аудит их не меняет.

Два concurrent finalizers,6 rounds/12 samples: existing4s640×36010fps H264/AAC,2ssegments; fixture encoding исключён. FinalizeComposite p50 **84.723 ms**, max88.874 ms; wall0.510 s. FFmpeg+ffprobe child CPU1.122311 s. Temp before/peak/after **1,964,672 B /8 files**, стабильно; durable recovery segments ожидаемо остаются. Go heap послеGC1.482→1.485 MB, total allocated8.621 MB. `/proc/self/io`: logical rchar delta33,855,916 B, wchar delta11,793,607 B; file cache/tmpfs/children делают эти числа непригодными для физической дисковой пропускной способности. Physical I/O wait **not measured**: shared Docker VM, контейнерный tmpfs, нет attributing block-device instrumentation.

Preview извлекает один кадр; duplicate final-file probe уже устранён до baseline. Segment duration/codecs/passes не менялись. Полный SFU→capture→finalize→MinIO→DB concurrent pipeline **not measured**; этот fixture измеряет метод финализации и process lifecycle, не end-to-end recording capacity.

### 13. Optimizations implemented

**Bottleneck → evidence:** sequential membership INSERT, линейные5/23/103queries; baseline snapshot и6pairedsamples. **Change:** normalize/validate и account locks прежние; подготовить≤100 rows и один `tx.Table("conversation_members").Create(&members)` в прежней транзакции. **Before→after:**20members6.893→2.851 ms;100members29.945→8.852 ms. **CPU/memory/latency:** меньше round trips/Go objects;100 members allocs3667→1845, B/op282.7→293.7KiB (+3.89%). Separate group CPU savings **not measured** — нет изолированного CPU profile этого короткого метода. **Risk:** bounded temporary batch и GORM generation; ограничения/owner/rollback проверены. **Regression:**100 members, ровно1owner, retry, invalid user/oversize, injected DB failure leaves no partial group/members, всё под race.

Для2 members time2.010→1.899 ms,p=.065 — значимый выигрыш не подтверждён; B/op+7.12%,allocs+2%. Небольшой bounded memory tradeoff принят ради измеренного крупного выигрыша на20/100 members и простой реализации. Никакого sharding/pooling/cache/serialization/compression redesign.

### 14. Before/After metrics

См. [обязательную таблицу](P1_BEFORE_AFTER.md). Единственное causal before/after — membership batch. SFU/Redis/Rabbit/FFmpeg/pools/read SQL не изменялись: их результаты — baseline текущего кода, не новая оптимизация. Старые audit numbers не используются. Expanded DB run, пересёкшийся с lifecycle load, сохранён как `data-overlapped-*` и исключён из таблиц; окончательные `data-before/after` выполнены последовательно. Shared host noise сохраняется, поэтому unchanged read p95 колебания не названы выигрышем.

### 15. Bench/load results

- Media7scenarios PASS; internal queues dropped0. Forwarded packets за sample:10peers45,000;10×2=90,000. Отдельный expected-sender/sequence/network-loss audit **not measured**, zero queue drops не равен zero loss.
- DB17 read paths×100 samples после5warmups,32×5 concurrency, exact query plans;1000 meetings,20 users/20kparticipants,20krecordings,40kfiles,400ksegments,200kevents,50kconference messages,20knotifications/transcripts/summaries,400ktranscriptsegments;1000 groups×20 members+19 direct,50,950 personal messages. Requests local, noTLS/auth/provider/upload.
- Group benchmark6alternatingbefore/after ×30ops ×2/20/100 members; fresh DB per sub-benchmark, benchstat p=.002 on20/100. Redis3benchmark samples,500-route reconciliation; Rabbit3 samples+3 faults; egress6 samples. Не оценивается production SLA/p99 по этим коротким samples.
- Portable `prepare`, actual WS/WebRTC/AMQP lifecycle и Linux FFmpeg wrapper проверены на тех же собственных fixtures. `.go.txt` templates не входят в обычный Go build; runner создаёт временный overlay.

### 16. P0 regression и gates

| Проверка | Результат текущего прохода |
| --- | --- |
| go test -count=1 ./... / go test -race -count=1 ./... | PASS, постоянных Go tests нет; обязательные actual regressions ниже |
| go vet / go build ./... / go mod verify | PASS |
| frontend Prettier / gofmt / diff whitespace | PASS |
| staticcheck | FAIL: прежние U1000 `composite/capture.go:490 newSourceWriter`, `sfu/peer.go:716 validateOffer`; новых diagnostics нет |
| govulncheck |0 symbol/package findings,1 module-only advisory; зависимости не менялись |
| Real SQL+Redis+2Hub WebSocket, race | PASS:20 warm+4×250=1020 lifecycles, subscription recovery и transient Prune; final14 goroutines,14 FD,0 localWS/namespacekeys/connectedSQLsessions |
| Real Pion lifecycle+source churn, race | PASS:25 fullcycles+25 replacements; after cleanupG2/FD6,0 room/peer/track/subscription; no race report |
| Group contracts, race | PASS: roles/bounds/idempotency/rejection/transaction rollback |
| AMQP blocked writes/RPC/handshake/Close, race ×3 | PASS; contexts/Close unblock sockets |
| Actual FFmpeg processes, Linux | PASS:17 segmentstarts,14 finalizations,15 cancellations/timeouts,6 expectedinput/disk failures,6 livedecoders; finalG2/FD7,0 child/0 tempfile/0 tempbyte; CGO0, не race binary |

WebRTC race-log содержит Pion teardown EOF/unhandled SSRC сообщения после остановки source; assertions/cleanup и race проходят. Они сохранены, не скрыты. Многодневная стабильность, WAN/browser/Android timestamp interoperability и production load **not measured** в этом P1. Новая UI/recording startup задача не подменяется синтетическим аудитом.

Финальный group contract исправлен, чтобы invalid-user fixture не изменял исходный список аккаунтов. Проверяется текст именно injected INSERT-trigger error, затем отсутствие частично созданных metadata/members и роли всех 100 участников. Три повторения под race: PASS (`tmp/p1-audit-20261016/final-group-contract-race.log`). Предыдущая версия могла завершиться до INSERT; её старый PASS сам по себе не используется как доказательство rollback. На latency benchmark это не влияет: он не запускает этот контракт и использует собственные fresh fixtures.

### 17. Remaining P1 bottlenecks

1. **Notifications/JIT:** измеренные≈118 ms median, JIT127.9 ms в plan; отдельный indexable predicate experiment + invalid/muted/left/cleared/deleted fixtures, без global JIT toggle.
2. **Pool budget:**4×20 potentialconnections против PG100/override60; media-worker не имеет DB pool; нужна simultaneousrole/replica load и измеренный budget, затем configuration tuning.
3. **Personal read cost:** latest/unread aggregate по50,950 messages и GroupMembers FOR UPDATE serialization; current race-safe ACL fences сохранить, сравнивать только с security/regression contracts.
4. **Recording list:**≈2 MB/request Go allocations с full nested metadata; lightweight projection после consumer-contract verification, не удаление полей молча.
5. **Reconnect herd:** fixed Rabbit retry1 s/Hub no jitter; multi-replica prolonged outage не измерен. Текущие singlefault recovery timings нельзя расширять до такого SLA.
6. **Long recovery/temp growth:** metadata Glob/ReadFile без hard cardinality/byte cap; failed durable artifact retention/storage pressure требует operational test. Recovery chunks не удалять только ради красивого heap/disk графика.
7. **Static gate:** два старых U1000 оставлены, поскольку naming/dead-code cleanup явно вне P1 scope. Это открытый gate, не «всё зелёное».

### 18. P2 follow-up

Отдельно, без реализации: договориться о compact recording DTO/пагинации nested artifacts; собрать end-to-end multi-room capture/MinIO stop→ready capacity; production topology connection budget; длительный multi-worker outage/backlog и bounded recovery retention. Sharding, custom RTP stack, новый cache или замена брокера не предлагаются без следующего измерения. Автоматического перехода к P2 нет.

### 19. Files changed и cleanup

- `internal/infrastructure/postgres/group_repository.go` — один batch INSERT вместо N inserts.
- `docs/code-review/P1_PERFORMANCE_AUDIT.md`, `P1_BEFORE_AFTER.md` — текущие evidence/limitations; предыдущие аудиты сохранены ниже как история.
- `tools/performance-audit/run.py`, `README.md`, `harness/**/*.go.txt` — opt-in load/bench/query/profile/lifecycle reproduction, без permanent Go tests и без обращения к пользовательским сервисам.

Cleanup receipt сохранён в `tmp/p1-audit-20261016/cleanup.json`; собственные контейнеры, их anonymous volumes и отдельная сеть удалены, состояние11 preexistingcontainers проверено. Приложение не публиковалось на удалённый хост.

---

## Повторный аудит — 10 октября 2026

Этот раздел описывает **текущий** P1 после P0. Отчёт 6 октября ниже сохранён как история: его другое оборудование, Go и прежние оптимизации не используются в качестве сегодняшнего baseline. Сводные числа и ограничения сравнения находятся в [P1_BEFORE_AFTER.md](P1_BEFORE_AFTER.md).

### 1. Executive summary и границы

Проверены RTP, ownership буферов, contention, Redis, RabbitMQ, PostgreSQL, личные/групповые чаты и две одновременные записи. Выбраны две минимальные правки: проверка наличия непрочитанного сообщения через EXISTS вместо полного COUNT и переиспользование уже проверенной длительности MP4 вместо второго ffprobe. Точные счётчики, проверки доступа, publisher confirms, recovery-файлы и P0-ограничения очередей сохранены.

SFU, размеры пулов, кодеки, сегментация, сериализация и reconnect не переписаны: измерения не обосновали такой риск. Новых индексов/миграций, UI и P2 нет. На удалённый хост ничего не публиковалось; существующие сервисы Docker не обновлялись и не перезапускались.

### 2. Baseline environment

- HEAD: `a0781257debba3f911ee180067cc0fad5bb06204` плюс уже имевшиеся P0/security/UI изменения. Они не откатывались и не считаются P1. Начальное состояние: `tmp/p1-audit-20261010/initial-status.txt`, `preexisting-working-tree.diff`.
- Apple M5 Pro, 18 logical CPU, 48 GiB RAM; macOS 27.0.1 arm64; Go 1.26.9. Docker 29.4.2, Linux arm64 VM: 10 CPU, 16 748 032 000 B RAM. GOGC/GOMAXPROCS не настраивались.
- Pion WebRTC 4.2.12/interceptor 0.1.45/RTP 1.10.2. DB-нагрузка: отдельные PostgreSQL 17.10 и Redis 8.8.1. Recording pod: PostgreSQL 17, Redis 7, RabbitMQ 4.2.9, MinIO, FFmpeg 6.1.2; точные image IDs в source manifests.
- До production-правок: uncached tests, race, vet, staticcheck v0.7.0, govulncheck v1.8.0, frontend lint, gofmt, shell syntax и diff whitespace — PASS. Govulncheck: 0 вызываемых/импортированных уязвимых пакетов; одна module-only OpenPGP advisory вне импортируемого пути, не утверждение об отсутствии всех уязвимостей.

Доказательства находятся в `tmp/p1-audit-20261010/{media,data,recording}`; это локальные ignored artifacts, не опубликованные данные пользователя. Репродуцируемые harness/tests находятся в репозитории. Нагрузки выполнялись в согласованных последовательных окнах, но приложения пользователя и прежние контейнеры продолжали работать. Первоначальные media microbench/recording warmup, пересекавшиеся с проверками, не включены в парное сравнение.

### 3. CPU/heap/alloc profiles и методика

Сняты CPU, heap, allocs, mutex, block, goroutine checkpoints для idle, 2/5/10 peers, двух комнат по 5/10 peers, screen + recording/audio taps, Redis reconciliation, DB/service/JSON, concurrent recording. Накопительные profiles сравниваются с исходным checkpoint; profiler/flate overhead не считается сжатием RTP. CPU sampling 100 Hz: отсутствие samples не означает нулевого CPU.

Media — реальные Pion ICE/DTLS/SRTP/UDP в одном процессе с клиентами, synthetic encoded payload160/1200 B, 50 RTP/s/source, 22 s прогрева и около 5 s измерения. Taps вычитываются без FFmpeg/STT; настоящий recording измерен отдельно. Это не браузерный/WAN/server-only capacity benchmark.

DB: 1000 встреч, 20k записей, 400k recording segments, 200k events, 50k conference messages, 400k transcript segments. Добавлены 1000 групп по 20 участников, 19 direct chats, по 50 сообщений, реальные fixture replies/attachments. По 100 samples, 5 warmups, ANALYZE; SQL count без BEGIN/COMMIT. Gin/JSON использует готовую identity без TLS/auth verification/presigning. Отдельный matched unread microbenchmark фиксирует два состояния: VACUUM ANALYZE до таймера и freshly written pages с autovacuum-off только на собственной disposable table. Это отделяет изменение SQL от возможного перехода heap scan → index-only scan;6samples по30операций в каждой ветке.

В 10×2 media измерено 14.54k forwarded RTP/s против идеальных 18k/s. Internal queue drops=0, но delivered tick count/sequence gaps и сетевые потери не измерены. Это ограничение стенда/результата, а не доказательство loss-free capacity. Крупнейший CPU profile: raw syscalls 58.58%, pthread wait 14.78%; application receive cum 0.98%. Retained heap в активной нагрузке на 92.58% связан с bounded Pion NACK history. После каждого сценария room/peer/track/subscription maps пусты, G3/FD6; live heap 0.49–2.24 MB, RSS не приравнивается к утечке.

DB recording-list profile:520ms CPU samples за3.22s, в основном kernel/scheduler; alloc-space delta168.36MB, relatedByRecordIDs38.63% cumulative, JSON marshal≈24.34MB cumulative. Это профиль metadata списка записей, не personal SQL CPU. Уменьшение DTO потребует API-контракта. Mutex≈2.86ms преимущественно runtime; block≈712ms в основном Tx.awaitDone/profiler stop. Recording profile содержит640MiB fixture Argon2 hashing (67.67% allocations), не медиа leak; security hashing не ослаблялся.

### 4. RTP allocations/copies

Путь: `TrackRemote.ReadRTP → Manager.receive → subscriber queue → subscription.forward → TrackLocalStaticRTP.WriteRTP → NACK/SRTP/UDP`. Собственного N-кратного payload clone перед подписчиками нет. Receive snapshot копирует указатели на subscribers, а не payload; sampled allocation share 0.38% при 10 peers и 0.22% при 10×2. Pool/reusable snapshots не оправданы.

`recordPacket` маршалит один раз для recording и audio tap: 0 alloc/op без sinks; 1 alloc/op, 176 B для payload160 или 1280 B для payload1200 и при одном, и при двух sinks. Шесть samples текущего кода, не новая оптимизация. В microbenchmark оба размера используют KindAudio; это не передача видео в production STT. Per-RTP SQL/Redis, debug formatting и high-cardinality labels не обнаружены; atomic counters сохранены.

### 5. Buffer ownership и Audio/STT

| Участок | Ownership / почему нельзя убрать copy |
| --- | --- |
| Pion receive buffer/Packet | Packet-owned; payload ссылается на buffer и переживает receive iteration |
| Negotiated extensions | Producer изменяет до fan-out; далее packet immutable |
| Очереди подписчиков | Shared immutable packet, bounded async lifetime; немедленный reuse небезопасен |
| Pion WriteRTP | SDK pooled shallow header copy для SSRC/PT, payload не меняется |
| NACK/RTX | Retained per-subscriber copies нужны для retransmission; bounded history1024 |
| Recording/audio egress | Один immutable marshaled buffer разделяют consumers до завершения чтения |
| Live PCM | Отдельный worker/FFmpeg вне SFU, reusable640 B только при синхронном callback |

Subscriber queue default128/max4096, recording2048/max8192, audio tap≤256. Nonblocking overflow/drop/cancel сохранены. STT HTTP/NDJSON/decoder CPU не входит в SFU benchmark; отдельного provider capacity test не проводилось.

### 6. Locks и channels

Registry/room snapshots освобождают locks перед fan-out и marshal. Largest media aggregate mutex delay225.66 ms: readRTCP82.84 ms, forwarding7.48 ms. Block7551.96 aggregate goroutine-seconds на 99.86% select wait — не latency одного запроса и не CPU. Per-peer SDP/forwarding barriers упорядочивают Pion операции; доказательств безопасного полезного удаления нет. Mutex→RWMutex, sharding и замена channels не выполнены.

Точный hold-time/frequency **not measured**: pprof показывает cumulative contention, не распределение каждого захвата; дополнительный per-packet timing не внесён ради субпроцентного snapshot. В personal delivery DB `FOR SHARE` удерживается на время bounded WS write намеренно: revocation/clear должны сериализоваться с доставкой. Снятие lock или общий cached JSON между recipients нарушит security contract (membership, cutoff, mute, reply stripping); generic «unlock before I/O» здесь не применён механически.

### 7. Redis reconnect/efficiency

go-redis v9.19.0 владеет pool/command retries: max retries2, timeout3 s, SDK randomized backoff8–512 ms. Hub имеет один receive recovery loop100 ms→2 s с acknowledged Subscribe; дополнительного reconnect worker нет. Hub-level jitter отсутствует, многорепличный prolonged outage/herd **not measured**.

Missing уже использует pipeline EXISTS по ≤500 routes, ошибки отбрасывают partial results. 500 routes — 1 request batch/500 commands, не 1 команда. Account Online1/20/100 users — 1 batch/1 Lua command после прогрева; TTL чтением не продлевается. Присутствие — control plane, не запись на каждый RTP. Пять CLIENT KILL собственной subscription дали readiness + новую доставку за 105.888–109.492 ms; Redis оставался здоровым, это не restart/partition SLA. Production Redis не менялся.

### 8. RabbitMQ

Confirmed serial commands249.53–258.91 µs/op (median≈3983 commands/s), parallel243.59–279.29 µs/op; 65 Go alloc/op. Publisher reconnect2.169–2.436 ms; девять consumer faults — первая наблюдаемая delivery1.005–1.014 s. Возможна redelivery; это не p95 отдельной новой команды. Confirms и connection/channel reuse сохранены; payload — команды/ссылки, не blobs. QoS/prefetch1 соответствует sequential heavy handler.

Нет измеренного основания для confirm pipelining/prefetch tuning. Fixed consumer retry без jitter и отсутствие общего queue backlog limit требуют отдельного multi-worker/outage/capacity решения, а не выключения надёжности. Новых Rabbit production-правок нет.

### 9. PostgreSQL pool

MaxOpen20/MaxIdle10, lifetime30m, connect timeout5s, statement timeout10s, lock timeout5s; MaxIdleTime не задан. 32 clients×5 pages: peak20/20, final10/0/10, 0 errors, 917 waits / 1.455 aggregate waiter-seconds за166 ms wall. Повтор после правок:897waits/2.007aggregate s за184ms, p95 pool workload51.830→58.298ms; этот recordings path не менялся, gain не заявляется. Увеличение пула не обосновано.

Четыре DB-роли с default20 дают потенциальные80 соединений, без резерва для миграций/диагностики. Тестовый PG max_connections100; shared-host override60 конфликтует с потенциальным budget80. Это конфигурационный риск, не воспроизведённый production outage; удалённые настройки не читались/не менялись. Общая saturation всех ролей и реплик необходима перед tuning.

### 10. N+1/query findings

Записи1/20 — по6 SQL; meetings20=1; history=6; participants20=4; conference chat20=8; notifications20=2; transcript20=5; summary4; analytics3; admin3. Personal list1/20=2, Gin personal20=3; group chat1/20=10 с replies/attachments/security, direct chat20=9, read state6, group members20=3. Число сетевых SQL не растёт на каждый элемент этих страниц. COUNT в correlated projection всё ещё может быть дорогим: постоянные round trips не означают постоянный DB CPU.

History backend и routes остаются в текущем checkout, несмотря на ранее удалённый пункт меню; поэтому они проверены. Recording list по прежнему контракту содержит segments/events metadata, но не медиа bytes/full transcript. Удалять эти поля ради ускорения без изменения API не стали.

### 11. EXPLAIN и unread optimization

EXPLAIN(ANALYZE,BUFFERS) сохранены для фактических SQL. Исходный unread filter выполнял correlated COUNT>0 для1019 conversations: примерно48 подходящих сообщений/loop,54 133 buffer hits в фильтрующем subplan. Первый SQL целиком:58 654buffer hits/19.339ms; после EXISTS5972/8.306ms, Hash Semi Join. Это не1019измеренных early-stop index probes. Для булева ответа полный count избыточен.

Заменён только predicate на EXISTS с общей строкой условий conversation, max(read,clear) cutoff, sender≠self, deleted_at IS NULL. Numeric unread projection/общий badge остаются COUNT; membership/hidden/deleted/filter/cursor условия не меняются. Существующие индексы используются без миграции. Чистый matched fresh-page benchmark6samples:25.74→14.88ms (−42.18%,p=.002), Go allocations без значимого изменения. На vacuumed pages эффект статистически не подтверждён (p=.065). Парные планы/benchstat и различие состояний fixture приведены во втором отчёте. Тест сверяет DTO, exact counts, порядок и все страницы с авторитетным numeric projection, включая clear/read/self/deleted/mute/hidden случаи.

### 12. Temporary files и disk I/O

Sources/chunk manifests создаются в `records/<UUID>/sources` с atomic manifest rename; completed chunks сохраняются для recovery. Concat `-c copy`, final MP4 и seek+one-frame preview читаются потоковыми checksum/upload. Staging upload scope включает lease token; при неудачном commit удаляется только собственная generation. После ready локальная запись удаляется при KEEP_LOCAL=false. STT использует private job dir, bounded io.Copy и cleanup; individual tracks — streaming ZIP Store, без повторного сжатия media.

Full-media ReadAll/MinIO→full RAM→disk в проверенных production путях не найден. Disk chunks и отдельные stages нужны для восстановления; pipes/слияние FFmpeg passes/изменение segment duration не внедрены. Full-stack fixture использует KeepLocal=true для проверки sources до cleanup, поэтому peak temp не равен production retention.

Дублирующий **ffprobe**, а не повторное transcoding, удалён в composite/audio finalization. `validateOutput` уже проверяет finite positive format.duration/container/codecs; файл затем только читается. Локальное reuse сохраняет rounding `int(d+0.5)` и `ctx.Err()` checkpoint audio после checksum. Legacy Finalize не менялся. Тесты invalid0/NaN/text и rounding2.49/2.50, probe count, cancellation защищают контракт; negative controls воспроизводят прежний второй probe и потерю cancellation при удалении checkpoint.

### 13–16. Оптимизации, before/after и регрессии

Итоговые парные числа, таблица обязательных метрик и доказательства приведены в [P1_BEFORE_AFTER.md](P1_BEFORE_AFTER.md). Два принятых изменения: matched fresh unread25.74→14.88ms (−42.18%); composite finalization102.41→83.70ms (−18.27%), audio63.95→52.41ms (−18.05%), по6samples,p=.002. На vacuumed unread pages gain не подтверждён; весь stop→ready≈411ms не ускорился. Неизменённые подсистемы помечены baseline, а не фиктивным before/after.

Окончательные uncached whole tests/race, vet, staticcheck, govulncheck, frontend lint, gofmt/shell/diff — PASS. Реальные opt-in P0 smoke: SFU25 lifecycle+25source churn,2/3/5 peers/stalled tap — PASS92.639s; recording17starts/14finalizations/15cancel-timeouts/6live decoders — PASS; account1020+conference1020 WebSocket cycles и Redis faults — PASS. Scoped DB/Redis security race PASS, включая unread projection/query budget, clear/mute/hide/revoke/TTL. Browser/MinIO-only cases в отдельном security наборе SKIP, не выдаются за coverage; real MinIO recording проверен отдельным full-stack workload.

Удалены только новые disposable audit containers/databases/volumes/networks. Receipt `data/existing-services-after-cleanup.json`: все30 прежних контейнеров running с прежними ID/name/StartedAt, собственные PG/Redis отсутствуют; recording receipts показывают0 own leftovers. Synthetic workload data пересоздаётся harness, пользовательские записи/сервисы не затронуты.

### 17. Remaining P1 / ограничения измерений

1. 10×2 loopback media не достигает идеального offered rate; нужны separate Linux server/client, source tick и downstream sequence-gap counters, WAN/TURN/real-browser workload. Текущий PASS не заменяет capacity assessment.
2. Общий unread total и group member counts остаются пропорциональны данным; EXISTS ускоряет filter, но не превращает точный badge в O(1). Кэш/денормализация без invalidation contract не добавлены.
3. Shared-host budget80/60 и суммарные DB role/replica pools требуют операторского sizing. No arbitrary pool tuning.
4. Prolonged Redis/Rabbit outage, multi-worker herd, длительные composite/individual/STT jobs, full disk I/O totals и production capacity не измерены. `/proc` child samples — lower bounds, shared VM iowait нельзя приписать одной записи.
5. Personal-delivery row lock защищает revoke ordering. Если он станет hotspot, нужна измеренная security-preserving redesign, а не снятие барьера.
6. Из P0 остаётся отдельная диагностика macOS long account-WS run: прежний TCP dial timeout около2495 cycles при Linux3020 PASS не объявляется исправленным этим P1; свежий smoke не подменяет long-run investigation.

### 18. P2 отдельно

Не реализованы: unread counter cache/denormalization, lightweight recording-list DTO, очередь с capacity/admission policy, multi-replica reconnect jitter tuning, temp recovery TTL policy, redesign delivery revocation locking. Это отдельные решения по контрактам/ёмкости, не автоматическое продолжение P1. Не менялись dependency major versions, UI и архитектура.

### 19. Изменённые файлы текущего P1 и воспроизводимость

Production: `internal/infrastructure/postgres/personal_repository.go`, `internal/infrastructure/ffmpeg/postprocessor.go`, `internal/infrastructure/ffmpeg/audio_finalize.go`.

Tests/tools: `tests/integration/p1_db_performance_test.go`, `p1_redis_performance_test.go`, новый `p1_personal_performance_test.go`; общий fixture `auth_conferences_test.go` принимает testing.TB для benchmark. Новые `internal/infrastructure/ffmpeg/finalization_test.go` и `finalization_performance_test.go`. Документы: этот файл и `P1_BEFORE_AFTER.md`. Остальные dirty/staged файлы существовали до P1.

Команды и env для повторения:

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go test ./internal/infrastructure/sfu -run '^$' -bench '^BenchmarkP1RecordPacket$' -benchmem -count=6 -benchtime=1s
RECORDER_P1_MEDIA=true RECORDER_P1_PROFILE_DIR="$PWD/tmp/p1-new/media" go test -v ./internal/infrastructure/sfu -run '^TestP1MediaPerformance$' -count=1 -timeout=10m -memprofilerate=65536
RECORDER_P1_DB_PERF=true RECORDER_P1_DB_CONFERENCES=1000 RECORDER_P1_DB_PROFILE_DIR="$PWD/tmp/p1-new/db" go test -v ./tests/integration -run '^TestP1DBWorkloads$' -count=1 -timeout=10m
go test ./tests/integration -run '^$' -bench '^BenchmarkP1PersonalUnread(Fresh)?Page$' -benchmem -count=6 -benchtime=30x
RECORDER_P1_REDIS_LOAD=true RECORDER_P1_PROFILE_DIR="$PWD/tmp/p1-new/redis" go test -v ./tests/integration -run '^TestP1Redis(ReconciliationLoad|RecoveryLatency)$' -count=1 -timeout=2m
go test ./tests/integration -run '^$' -bench '^BenchmarkP1(PresencePage|UserPresencePage)$' -benchmem -count=6 -benchtime=200ms
RECORDER_P1_FINALIZATION_BENCH=true go test ./internal/infrastructure/ffmpeg -run '^$' -bench '^BenchmarkPostProcessorFinalization$' -benchmem -count=6 -benchtime=1s
python3 tools/p1_audit/recording_pod.py --phase after --output tmp/p1-new/recording --rabbit
```

DB/Redis требуют **выделенных тестовых** RECORDER_STAGE1_TEST_POSTGRES_DSN, RECORDER_P1_REDIS_ADDR/RECORDER_STAGE2_TEST_REDIS_ADDR, не production credentials. Fixture создаёт/удаляет собственные случайные DB/namespace. Finalization microbenchmark запускался compiled test binary в Linux FFmpeg image. `recording_pod.py --phase after` означает current source, даже для исходного baseline этого аудита; `--phase before` предназначен для старого capture.go overlay и не воспроизводит новые две правки. Их исходники/overlay сохранены отдельно в текущих evidence. Новые output directories обязательны. Benchstat версия `golang.org/x/perf v0.0.0-20261009192801-be2c69fb417e`, только local evidence binary, без изменения go.mod.

---

## Исторический отчёт — 6 октября 2026 (не текущий baseline)

Дата: 6 октября 2026. Базовый commit: `86bded31814db9eb748be0c04d7798a6b2212a85`. Задание: `CODE_REVIEW_AND_OPTIMIZATION_P1.md`. Аудит и нагрузки выполнены локально; развёртывание на удалённом хосте не выполнялось. P2 не реализовывался.

## 1. Результат

Приняты четыре изменения, основанные на измерениях: пакетная проверка присутствия Redis, пакетное чтение карточек записей, индекс для страниц записей и истории конференции, устранение повторного сканирования RTP-буфера при Flush записи. Состав ответов, проверки доступа, срок хранения, publisher confirms, очереди и штатное окно компенсации джиттера сохранены.

SFU, его буферы и блокировки оставлены без оптимизаций: измеренные собственные аллокации не обосновали дополнительную сложность. Числа, разделение эффектов и ограничения сравнения: [P1_BEFORE_AFTER.md](P1_BEFORE_AFTER.md).

## 2. Окружение и исходные проверки

Apple M5, 10 CPU, 24 GiB RAM; macOS 27.0.1 arm64; Go 1.26.6. Docker 29.4.2, Linux VM: 10 CPU и 16 748 032 000 байт памяти. PostgreSQL 16.14, Redis 7; версии FFmpeg/RabbitMQ и параметры контейнеров зафиксированы в доказательствах записи. GOMAXPROCS/GOGC не менялись.

До production-правок выполнены:

| Проверка | Результат |
| --- | --- |
| `go test -count=1 ./...` | PASS |
| `go test -race -count=1 ./...` | PASS |
| `go vet ./...` | PASS |
| gofmt, синтаксис release shell scripts | PASS |
| frontend `npm run lint` — проверка Prettier | PASS |
| staticcheck v0.7.0 | Только исходный S1024 в `persistent_auth_sessions_test.go:103` |
| govulncheck v1.8.0 | 0 уязвимых вызываемых путей; одна уязвимость зависимости вне вызываемого кода |

Локальные доказательства: `tmp/p1-performance-20261006/evidence/`. Приватные параметры подключений находятся отдельно, в файле с правами 0600, и не входят в отчёт. Профили/логи в `tmp` не отслеживаются Git; ключевые результаты перенесены в эти два документа. Воспроизводимые тесты включены в репозиторий.

## 3. Профили и методика

Сняты CPU, heap, allocs, mutex, block и goroutine profiles: idle; 2/5/10 участников; 5 и 10 участников в двух комнатах; экран с recording/audio tap; Redis reconciliation; DB/service/JSON; две параллельные записи. Дополнительно профилирован повторный WebSocket lifecycle.

SFU использует реальные Pion/ICE/DTLS/SRTP/UDP, но синтетические payload, без браузерного декодирования. Клиенты и сервер находятся в одном процессе, поэтому суммарный CPU/heap не является расходом только сервера. Прогрев 22 s заполняет bounded NACK history; затем 5 s измерения. RTP: audio 160/video 1200 байт, 50 пакетов/s на источник. В screen+recording+audio-tap сценарии encoded egress вычитывают тестовые goroutines; FFmpeg, Opus decode и STT здесь не запускаются. Их расходы не входят в эти SFU показатели; запись измерена отдельно. Профили heap sampling 65 536 байт; mutex fraction 1, block rate 1. Накопительные mutex/block/alloc profiles анализируются с исходным checkpoint, а не как независимые суммы разных subtests.

DB: случайные отдельные базы, миграции и ANALYZE, прогрев 5, 100 запросов на сценарий. Большой набор: 1000 встреч, 20 000 записей, 400 000 сегментов записи, 200 000 событий, 50 000 сообщений и 400 000 фрагментов транскриптов. Счётчик SQL включает Query/Row/Raw, исключает BEGIN/COMMIT. Gin-проверка включает handler/JSON с готовой identity, исключает TLS/сеть/auth verification/MinIO presigning.

Redis измеряется отдельно от SQL на реальных ключах и синтетическом Stale repository: 0/20/500 длительных сессий, 20 проходов за 5 s. Hook считает клиентские request batches; внутренние команды Lua и Pub/Sub handshake не входят в этот счётчик. Запись выполняется в собственном Docker pod на внутренней сети, с настоящими зависимостями и FFmpeg.

Окна тяжёлых нагрузок согласованы, но обычные процессы пользователя и существующие контейнеры продолжали работать. Малые изменения задержки не объявляются эффектом оптимизации. CPU sampling: 100 Hz: отсутствие samples означает недостаточное разрешение, а не нулевой CPU. Нагрузки короткие и не подтверждают максимальную вместимость production.

## 4. RTP allocations и copies

Путь: `TrackRemote.ReadRTP → Manager.receive → bounded subscriber queue → subscription.forward → TrackLocalStaticRTP.WriteRTP → NACK/SRTP/UDP`.

В SFU нет payload clone на каждого подписчика: один неизменяемый packet передаётся всем очередям. Pion создаёт receive buffer/Packet, сохраняет нужные копии для NACK и сериализации/шифрования. Перезаписывание SSRC/PT выполняется на поверхностной копии заголовка Pion. `recordPacket` маршалит RTP один раз и разделяет bytes между записью и audio tap.

Бенчмарк показал 0 alloc/op без egress, 1 alloc/op при одном **или двух** egress: 176 B для payload160, 1280 B для payload1200. Microbenchmark использует KindAudio и синтетические sinks для обоих размеров; второй sink не означает передачу video в production STT. Предполагаемый snapshot allocation в этом пути не подтвердился на Go1.26.6. В receive при 10 участниках snapshot составляет примерно 0,63% sampled allocations; при 2/5 не наблюдался в heap. Пул и reusable snapshot не добавлены.

Per-RTP JSON/Redis/SQL, дорогое debug formatting и payload compression в forwarding отсутствуют. Атомарные packet/byte counters сохранены. Собственный RTP stack, sharding и новые cache не добавлялись.

## 5. Buffer ownership и STT

| Участок | Владелец / срок жизни |
| --- | --- |
| Receive buffer/Packet Pion | Принадлежит packet; payload ссылается на этот buffer и может пережить текущий receive |
| Подготовка RTP в SFU | Producer меняет negotiated extensions до fan-out |
| Очереди подписчиков | Разделяют immutable packet до отправки/отмены; payload не меняется |
| Pion WriteRTP | Собственный временный header/SSRC/PT; shared immutable payload |
| NACK history | Собственные retained copies, до 1024 пакетов на подписчика |
| Recording/audio egress | Один immutable marshaled buffer; асинхронные consumers держат ссылку |
| PCM live STT | Отдельный captions worker/FFmpeg decoder; синхронный callback с ограниченным chunk |

Немедленное переиспользование receive/payload buffer нарушило бы срок жизни данных. STT остаётся вне RTP goroutine. Subscriber queue default128/max4096; recording default2048/max8192; audio tap max256. Overflow не блокирует producer и изолируется на соответствующем downstream. Decoder slots/LiveQueue ограничены; декодирование Opus выполняется отдельным процессом.

## 6. Блокировки и каналы

Изучены manager/room/peer/subscriptions, negotiation, forward barrier, WS registry, routing и recording state. Профили не подтвердили значимого SFU/WS registry contention. В WS replay суммарная mutex delay7,47 ms, около 97% — runtime locks; пользовательский Mutex Unlock около 0,04 ms. Ожидания select/network/timer в block profiles являются ожидаемыми состояниями горутин, не автоматически CPU bottleneck.

Negotiation/WriteRTP barriers обеспечивают согласованность SDP и готовности подписки. Их изменение требует отдельного доказательства выгоды. Длительность каждого lock hold и частота acquisition отдельно не инструментировались: pprof даёт суммарное ожидание, а наблюдавшаяся доля не обосновала production instrumentation/refactor. Mutex→RWMutex, новые shards и замена channels не выполнялись. Queues сохранили P0 bounds/drop/cancellation.

Rabbit publish mutex сериализует публикацию с confirm; measurable parallel wait объясняется этим контрактом. Throughput около 4,2k команд/s не обосновывает усложнение подтверждений.

## 7. Redis

Подтверждённый bottleneck: SQL Session намеренно обновляется при открытии/закрытии, поэтому давно работающие соединения попадают в Stale; Hub каждые 250 ms последовательно GET/decode проверял до 500 маршрутов. При 500 сессиях:10 022 request batches/5 s и 14 127 688 allocated bytes.

`RealtimeStore.Missing` выполняет pipeline EXISTS по тем же route keys, до 500 команд в batch; Hub закрывает только подтверждённо отсутствующие маршруты. Redis/context error отбрасывает частичные ответы. Истечение, LastSeenAt, SQL Close и TTL не изменены. После:42 batches/5 s,4 574 880 allocated bytes. Серверных команд всё ещё 10 023: сокращены round trips и JSON, не количество EXISTS.

Benchmark500:84,976→0,932 ms,9500→2521 alloc/op. Benchstat5 samples, p=.008 для 20/500; пяти samples недостаточно для 95%CI. При 2 строках timing gain статистически не подтверждён. CPU до:300 ms samples/5,02 s; после:0 samples, без заявления о 100% CPU saving.

go-redisv9.19.0 владеет pool/command retries с SDK jitter; app задаёт timeout3 s, MaxRetries2. Hub recovery выполняет один существующий receive worker с acknowledged Subscribe,100 ms…2 s. SDK может выполнить одно немедленное переподключение перед возвратом Receive error; Hub затем закрывает эту subscription. Дополнительного параллельного recovery worker нет. Hub-level jitter и многорепличный prolonged-outage herd не измерены и не изменены.

5 faults CLIENT KILL только собственной Pub/Sub:103,385–105,767 ms до readiness; replacement действительно получает событие. Это post-change observation при здоровом Redis, не restart/partition benchmark. Register/Touch/Unregister/Active/Prune — существующий Lua,1 warm round trip; cold NOSCRIPT может добавить EVAL. Rate limit/ownership также остаются атомарными. Heartbeat control plane default5 s; Redis per RTP нет. Pool hard cap не добавлен без измеренной saturation.

## 8. RabbitMQ

Connection/channel reuse, один consumer loop, publisher confirms и QoS1 сохранены. Payload — маленькие команды/ссылки, без медиа или transcript blobs. Serial confirmed publish229–242 µs/op; parallel298–356 µs/op. Publisher reconnect с topology/confirms2,42–2,51 ms; consumer disconnect→первая наблюдаемая delivery 1,004–1,011 s при существующем фиксированном backoff. Девять faults; наблюдение может включать redelivery, latency отдельной новой команды/p95 этим тестом не измеряется.

Prefetch1 соответствует последовательному handler; повышение только перенесло бы ожидание jobs в процесс. Consumer retry не имеет jitter; длительный multi-worker outage/thundering herd не измерен. Для текущего workload изменение recovery/prefetch/confirm pipeline не оправдано. P0 bounded close/cancel не ослаблены.

## 9. PostgreSQL pool

MaxOpen 20, MaxIdle 10, lifetime 30m; connect timeout5 s, statement_timeout10 s, lock_timeout5 s. Runtime Operations config задаёт MaxOpen/MaxIdle/lifetime из env; DB_QUERY_TIMEOUT отдельно управляет statement_timeout. Connect/lock timeout фиксированы. MaxIdleTime не задан. Чтение передаёт context; тестовые запросы имеют deadline10 s. Pool не увеличен.

32 клиента×5 страниц: WaitCount12 908→889, суммарный WaitDuration8,774→2,609 s, elapsed757→245 ms; peak Open/InUse20/20, final10/0/10, ошибок 0. WaitDuration суммирует ожидания разных горутин и не является wall time.

Четыре DB-процесса API/recorder/product/live с cap20 дают потенциальные 80 соединений. Реальный локальный PG, проверенный SHOW, допускает 100. Конфигурационный `docker-compose.shared-host.yml` задаёт 60: при его применении default budget80 превышает лимит. Это несогласованность комбинации конфигураций, не воспроизведённый отказ локального runtime. Дополнительные реплики увеличивают budget. Remote settings и общий 80-connection saturation не проверялись. Перед capacity tuning требуется суммарная нагрузка всех ролей и резерв для миграций/диагностики; произвольные новые pool limits не установлены.

## 10. N+1 и состав запросов

`recordings.List` выполнял `2+4N` SQL:6 для 1 записи,82 для 20. Новый batch после membership/page query повторно проверяет availability, загружает record/files/segments/events общими запросами и восстанавливает порядок UUID. Тот же DTO и public status, nullable fields, presigning builder; legacy adapters сохраняют fallback. Страница 1/20 теперь 6 запросов.

Остальные пути имеют постоянный budget: meetings20=1; history=6; participants20=4; chat20=6 или 7 с replies/attachments; notifications20=2; transcript20=5; summary=4; analytics=3; admin=3. Проверка чата с непустыми вложениями/ответами даёт 7 SQL и при 1, и при 20 сообщениях. Полный object-storage/UI chat E2E не запускался; DB decoration проверен напрямую.

Список записей сохраняет существующие segments/events metadata: fixture20 карточек содержит 400 сегментов и 200 событий. Двоичные медиа не читаются; transcript page ограничена. Удаление этих коллекций потребует отдельного API contract и не выполнено в этой минимальной правке.

## 11. EXPLAIN и индекс

После ANALYZE сохранён каждый фактически выполненный SQL и EXPLAIN(ANALYZE,BUFFERS). На 20k записей page/history выполняли Seq Scan, отбрасывая 19 980 строк. Candidate по `platform_conference_id, created_at DESC, id DESC`, partial `deleted_at IS NULL` и четырём recording modes, дал Bitmap Index/Heap Scan:588→3 shared buffers, page1,478→0,040 ms, history1,220→0,018 ms.

Миграция `000027_recording_page_index.up.sql` добавляет только этот индекс; старые checksums не менялись. Локальный build34 ms, размер 1 007 616 байт; small in-memory sort без temp spill. Retention deadline остаётся условием запроса. Общие COUNT с низкой selectivity не получили необоснованные индексы.

Миграционный runner транзакционный: обычный CREATE INDEX блокирует запись в таблицу на время построения/транзакции. Локальные 34 ms не определяют production окно. Write/WAL overhead не измерен. Текущий loader читает только up.sql, как миграции 23–26; автоматического down rollback нет. Развёртывание и операторский rollback в этом аудите не выполнялись.

## 12. Временные файлы и disk I/O

Проверен жизненный цикл файлов, включая успешное завершение и recovery:

| Данные | Создание / потребление | Очистка / сохранение |
| --- | --- | --- |
| Encoded IVF/H.264/Ogg sources и chunk manifests | `records/<UUID>/sources`, manifest .partial→atomic rename; composer читает завершённый chunk | Сохраняются при interruption/failure для recovery |
| Rendered MP4 chunks | Отдельный FFmpeg на chunk, фиксированные thread/concurrency limits | Повторно используются recovery/concat; после защищённого ready удаляется каталог, если KEEP_LOCAL=false |
| `concat.txt`, final MP4 и preview JPG | Composite concat использует `-c copy`; preview seek + один frame, SHA256 читает файл потоком | После upload и lease-guarded metadata commit очищается локальная запись |
| MinIO upload staging | FPutObject читает файл, затем StatObject сверяет размер; ключ включает recording и lease token | Неуспешный commit удаляет только generation prefix своего token; общий retention сохранён |
| STT input MP4 / WAV | Отдельный приватный случайный job directory; bounded object reader→io.Copy→disk; FFmpeg имеет byte/duration limits | Cleanup закрывает reader и один раз удаляет job directory; error path также удаляет каталог |
| Individual-tracks archive | CreateTemp в recording directory; `zip.Store`, streaming io.Copy + checksum, без повторного сжатия encoded media | Частичная ошибка удаляет ZIP; после upload defer удаляет staging archive |
| Legacy recorder intermediates | Каталоги `records/<UUID>` и `tmp/<UUID>` | Штатная cleanup удаляет только каталоги соответствующей записи |

Full-media ReadAll / MinIO→full RAM→disk в этих production путях не найден. ReadFile в composer касается manifests. Дисковые chunks нужны для recovery; заменять их pipes без измеренной выгоды и нового failure contract не стали. Segment duration в fixture 2 s; изменение duration, FFmpeg passes или codec settings не обосновано измерениями. Offline/live STT/provider CPU и полные длительные записи отдельно **not measured**: workload записи не запускает STT provider.

Парные две pipelines: Linux arm64, PostgreSQL17, Redis7, RabbitMQ4.2.9, MinIO, FFmpeg6.1.2; 4→6 участников в двух комнатах, screen, H.264/AAC640×360, около 11 s и 1 MB на каждый итоговый MP4. Fixture KeepLocal=true сохраняет sources для проверки; temp peak 8 013 114→7 806 045 B; stop→ready 364,962/414,852→363,403/364,156 ms. Этот timer включает Rabbit/control, FFmpeg, upload/DB и polling. FFmpeg-only `/proc` CPU lower bounds 0,40→0,35 s; all-child CPU 5,397→5,740 s включает другие subprocesses. Shared VM iowait и полные disk totals не измерены, причина и process counters приведены в [P1_BEFORE_AFTER.md](P1_BEFORE_AFTER.md).

Исходный профиль показал дорогой SampleBuilder.tooOld при forced Flush. Изолированный loss case воспроизведён через публичные Push/Pop: VP8 tail без partition head вызывает повторное сканирование пустых RTP slots. Linux6 repeats median697,140 ms→298,791 µs, одинаковые output/drop/timing и 0 B/0 alloc. Guard снимает age limit только внутри Flush, затем восстанавливает 200 ms; normal capture не меняется. Реальные outputs, wrap/reorder/loss и reuse проверены, включая mutation-test восстановления окна джиттера.

Matched normal-case CPU и allocations не показывают общего выигрыша: Go parent 3,964→4,373 CPU s, all-child5,397→5,740 s. Принята только доказанная правка дорогого loss case. Initial profile, парное сравнение и неприменимые попытки сохранены раздельно. Первый after-профиль остановила существующая MinIO bootstrap race BucketExists→MakeBucket; одинаковое предварительное создание disposable bucket в обеих measurement fixtures устраняет этот шум без production-правки. Последний after temp sample не захватил финальную очистку; measured EOFtemp0 не утверждается. Все собственные pod containers/network/anonymous volumes после запусков удалены.

## 13. Принятые оптимизации

| Находка | Доказательство | Минимальная правка | Основной риск / проверка |
| --- | --- | --- | --- |
| Sequential presence GET | 500 RTT/page, JSON allocation/profile | bounded EXISTS pipeline | Partial failure не должен закрыть живую Session; expiry/cancel/error tests |
| Recording page N+1 | 82 SQL для 20, рост пропорционален N | Batch records и existing related loaders | DTO/order/NULL/access/expiry equivalence |
| Full scan platform recordings | EXPLAIN588 buffers/19 980 лишних строк | Один query-specific partial index | DDL lock/write cost; реальные migration ledger/lock tests |
| RTP builder final Flush scan | Профиль tooOld/forced purge | Отключить только age scan во время Flush, восстановить 200 ms | RTP tail/holes/reorder/wrap/quiet reuse equivalence и реальная запись |

Ни одна правка не переносит STT в SFU, не отключает confirms, не увеличивает очереди и не меняет формат записанного медиа. JSON/compression/cache не оптимизировались: профили не обосновали отдельное изменение.

## 14. Before/after

Полная таблица обязательных метрик, отдельные batch/index эффекты и не измеренные показатели — в [P1_BEFORE_AFTER.md](P1_BEFORE_AFTER.md). Нельзя складывать показатели разных процессов или трактовать throughput-normalized workload как одинаковый объём работы.

## 15. Нагрузочные сценарии

Медиа-матрица idle/2/5/10/две комнаты/screen+egress прошла. Счётчики SFU/recording/audio-tap drops равны 0. Forwarded RTP/s считает успешные WriteRTP; downstream sequence gaps и полные сетевые потери отдельно не измерялись. При 10 участниках в двух комнатах измерено ≈17 995 forwarded RTP/s. API/DB: 100 samples на каждый путь и 32 concurrent clients; Redis: 0/20/500 sessions; recording две независимые pipelines. Подробные таблицы и ограничения представлены во втором отчёте.

## 16. P0 regression

Финальные `go test -count=1 ./...` — PASS, 18,208 s, `go test -race -count=1 ./...` — PASS, 22,694 s, 49 пакетов с тестами. `go vet`, gofmt, release shell syntax, frontend Prettier и govulncheck — PASS. Staticcheck: только исходный S1024, новых замечаний нет. Govulncheck: 0 вызываемых уязвимых путей.

Opt-in regression: SFU: 25 lifecycle +25 source churn + smoke2/3/5 + stalled audio tap под race — PASS, 57,317 s; после shutdown/churn maps0, G2/FD6. DB: 10 выбранных tests под race — PASS, 8,763 s: DTO/order/NULL/modes, access, expiry/deletion, cursor/history, retention/retry, content/analytics и migrations. Redis проверяет chunking1001 IDs, expiry/cancel/error и Pub/Sub recovery; реальный WS lifecycle повторён после DB/Redis изменений. Recording Flush equivalence и реальные FFmpeg output tests прошли 6 Linux повторов.

Первый final gate выявил тестовый adapter без нового Missing и временные .go overlay sources внутри module. Исправлены adapter и расширения snapshots; повторные gates успешны. Собственные Docker pods/Redis/network/anonymous volumes удалены; исходные 11 контейнеров продолжают работать.

Повторный WS replay:1020 lifecycles, горутины 15 во всех checkpoints, warmFD14/final15, final localWS/Redis keys/connectedSQL=0. Первый запуск совместил CLI CPU profiler с внутренним profiler старого теста и завершился ошибкой наблюдателя; исправлен режим запуска и выполнен успешный повтор. SFU fixture сначала зарезервировал лишнюю SDP-пару; corrected baseline использует штатные limits. Эти попытки сохранены отдельно, не выдаются за дефекты production.

## 17. Оставшиеся P1

1. Production capacity: длительная одновременная API/media/recording/product/live нагрузка, отдельный server-only SFU process, браузерные кодеки, TURN/TCP/NAT и remote configuration не измерены.
2. При shared-host overlay необходимо распределить суммарный DB budget ниже 60 с maintenance reserve; idle-time policy и Redis hard connection cap требуют combined load, не произвольных значений.
3. Многорепличные Redis/Rabbit outage, jitter и herd recovery не измерены. Serial Hub critical delivery выполняет bounded roster/state I/O; устранение повторных queries/cache требует latency workload и явной invalidation модели.
4. Recording list metadata остаётся пропорциональной числу child segments/events. Для долгих записей нужен отдельный summary projection contract; нынешняя правка сохраняет DTO.
5. Полный FFmpeg CPU/per-recording disk total, длительные composite/individual-track/STT нагрузки и retained temp после ошибок требуют отдельного продолжительного измерения. MinIO bootstrap BucketExists→MakeBucket допускает параллельную race; это исходная reliability находка, не регрессия Flush и не реализованная P1 оптимизация.

## 18. P2 follow-up

P2 записаны отдельно от реализованных P1: исходный staticcheckS1024; единичный profiling bind-failure waiter до process cancel; local-only debug completed-records listing собирает metadata всего bucket до limit; будущая cleanup/TTL политика для сохранённых recovery chunks. Удаление recovery-файлов без подтверждённой политики не выполнялось.

## Воспроизведение

Тесты нагрузки opt-in, обычный `go test ./...` их пропускает. Экспортируйте тестовые env в subprocess без вывода credentials. PostgreSQL DSN должен быть localhost с CREATE DATABASE: fixture создаёт/удаляет собственную случайную БД. Redis использует уникальный namespace. Для записи helper создаёт собственные изолированные PG/Rabbit/MinIO/Redis. Требуются cached Linux arm64 images и worker image с FFmpeg; список проверяется до создания контейнеров. Имя evidence directory должно быть новым.

```sh
go test ./internal/infrastructure/sfu -run '^$' -bench '^BenchmarkP1RecordPacket$' -benchmem -count=6 -benchtime=1s
RECORDER_P1_MEDIA=true RECORDER_P1_PROFILE_DIR="$PWD/tmp/p1/sfu" go test -v ./internal/infrastructure/sfu -run '^TestP1MediaPerformance$' -count=1 -timeout=10m -memprofilerate=65536
RECORDER_P1_REDIS_LOAD=true RECORDER_P1_PROFILE_DIR="$PWD/tmp/p1/redis" go test -v ./tests/integration -run '^TestP1RedisReconciliationLoad$' -count=1 -timeout=1m
go test ./tests/integration -run '^$' -bench '^BenchmarkP1PresencePage/(SequentialGet|Missing)' -benchmem -benchtime=1s -count=5
RECORDER_P1_DB_PERF=true RECORDER_P1_DB_CONFERENCES=1000 RECORDER_P1_DB_PROFILE_DIR="$PWD/tmp/p1/db" go test -v ./tests/integration -run '^TestP1DBWorkloads$' -count=1 -timeout=10m
go test -race ./tests/integration -run '^TestP1RecordingBatchPreservesPage$' -count=1
python3 tools/p1_audit/recording_pod.py --phase before --baseline-ref 86bded3 --output "$PWD/tmp/p1/recording-before"
python3 tools/p1_audit/recording_pod.py --phase after --rabbit --output "$PWD/tmp/p1/recording-after"
```

Before DB требует baseline 86bded3 с measurement harness, без batch/index; для recording baseline используется Go overlay исходного capture.go. Алгоритм sequential GET сохранён только в benchmark для повторного comparison. Benchstat: `golang.org/x/perf/cmd/benchstat@v0.0.0-20260929162123-406019bb8b68`. Команды/настройки recording pod и профильные summaries сохранены в `evidence/recording`, SQL/EXPLAIN — в `evidence/db`, ownership/нагрузка — в `evidence/sfu`.

## 19. Изменённые файлы

Production: `internal/infrastructure/{composite/capture.go,postgres/record_repository.go,redis/realtime.go}`, `internal/usecase/{realtime/hub.go,recorder/conference_reader.go,recordings/service.go}`, новая миграция `database/migrations/000027_recording_page_index.up.sql`.

Измерения/регрессии: `internal/infrastructure/composite/{performance_test.go,flush_performance_test.go}`, `internal/infrastructure/rabbitmq/performance_test.go`, `internal/infrastructure/sfu/{manager_test.go,p1_performance_test.go}`, `internal/infrastructure/redis/realtime_test.go`, `internal/usecase/realtime/reconciliation_test.go`, `internal/transport/websocket/persistent_session_test.go`, `tests/integration/p1_{db_performance,recording_performance,recordings_batch,redis_performance}_test.go`. Изолированный recording pod: `tools/p1_audit/recording_pod.py` и `tools/p1_audit/README.md`. Отчёты: этот документ и `P1_BEFORE_AFTER.md`.

Рабочее дерево содержит правки; commit/release/deploy не создавались. Следующий этап автоматически не запускался.
