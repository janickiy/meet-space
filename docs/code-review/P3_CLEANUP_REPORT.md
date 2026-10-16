# P3 — финальная чистка кода

## Текущий проход — 2026-10-16

**Статус:** выбранная косметическая чистка P3 выполнена; проверки поведения, race, static/lint и репрезентативные media benchmarks прошли. Исходный HEAD: `a604c0d49ce5651f481fe0c0a144e5a049759ff4` **плюс существовавшие P1/P2-изменения**, сохранённые без отката. Это сравнение рабочих состояний, не чистых commits. Production deployment не выполнялся.

До начала P3 не было открытых P0 race/resource blockers. Ограничение P2 — нестабильный throughput при 10 участниках — явно документировано в [P2_BEFORE_AFTER.md](P2_BEFORE_AFTER.md); P3 его не закрывает. Исходники, HEAD, status, hashes и diff сохранены до правок в `tmp/p3-audit-20261016/`. Исторический отчёт 10 октября ниже не является результатом текущего запуска.

### 1. Executive summary / scope

Изменены 13 Go-файлов и три текущих руководства; этот отчёт — четвёртый документ. Единственное исполняемое изменение: одинаковая проверка UUID вынесена из двух приватных helpers в один внутри прежнего пакета `app/personal`. Остальные Go-правки — корректировка комментариев и вызванное удалением inline-комментариев стандартное форматирование.

Проверены naming, локальные условия, error/log terminology, comments/TODO/debug, generated markers, зависимости, небольшой dead code и актуальность руководств. Не найдено основания для новых package/file splits, интерфейсов, generics, options structs или изменения constructors. Имена Participant, ParticipantSession, MediaPeer и Conference/Room сохраняют разные роли. Короткие Go-имена в малых scopes оставлены.

### 2. Naming changes

`directActionID` использовался не только в direct actions, но и в общих закреплениях сообщений личных и групповых чатов. Вместо него и `groupParameter` используется приватный `uuidParameter(c, name)`: имя точно описывает нормализацию UUID из параметра маршрута.

Заменены 15 вызовов. Прежние имена параметров `id`/`userId`, порядок их чтения и последующих действий сохранены. Нет переименований публичных Go declarations, пакетов, полей JSON или событий.

### 3. Local simplifications

Проверка UUID теперь читается одинаково во всех затронутых handlers: parse/normalize → существующий HTTP error при ошибке → early return перед repository. Тело нового helper совпадает с прежним `groupParameter`; при `name="id"` оно совпадает с прежним `directActionID`.

Сложные lifecycle-функции не переписывались. В SFU удаление шаблонных комментариев между `func(...)` и `{` возвращает обычное читаемое оформление callbacks; это не изменение nesting, блокировок или defer semantics.

### 4. Duplicate code removed

Два одинаковых UUID helpers заменены одним в `handler.go` того же пакета. Отдельный generic/common слой не появился. Удалён ставший лишним импорт `domain/chat` из `direct_actions_handler.go`; остальные использующие его файлы сохранены.

| Метрика | До P3 | После P3 |
|---|---:|---:|
| UUID helpers для выбранных actions | 2 | 1 |
| Production internal packages | 74 | 74 |
| Все функции/методы в AST | 1430 | 1429 |
| Public declarations | 1527 | 1527 |
| Изменённые package imports / type fields / interfaces | — | 0 |

Количество строк не используется как основной показатель качества. Доказательство сравнения границ: `api-boundaries.json`; доказательство эквивалентности helper и вызовов: `batches/ad/source-equivalence.json`.

### 5. Dead code removed

Удалена только дублирующая приватная реализация проверки UUID и один неиспользуемый после объединения import. Новых доказанно unreachable функций, obsolete endpoints, flags, типов или файлов не обнаружено. Staticcheck был чистым до и после P3. Compatibility, recovery, диагностические инструменты и opt-in tests сохранены.

### 6. Comments / TODO / debug cleanup

В `sfu/{manager,peer,tracks,egress}.go`:

- удалены 18 шаблонных inline-блоков внутри callbacks (78 строк) и один повторяющийся двухстрочный комментарий;
- исправлены 16 ложных описаний параметров: peer не массив байтов, published track не `testing.T`, receiver не HTTP client, captured timestamp — Unix nanoseconds;
- сохранены WHY-комментарии о блокировках, владении, закрытии, Pion, RTP и recovery.

В пяти других Go-файлах исправлены конкретные контракты:

- `notifications/streams.go`: счётчик хранит per-user capacity, а global counter принадлежит `eventStreams`;
- `notifications/events.go`: контекст подключения Redis ограничивает установку подписки, её длительный lifecycle закрывает `serve`; строка verifier — JWT, не произвольные JSON bytes;
- `domain/apperrors/errors.go`: Kind — категория ошибки, Message — безопасный текст ошибки; New всегда создаёт ошибку, Unwrap возвращает Cause либо Kind;
- `httpresponse/response.go`: BindJSON — 32 KiB, запрет unknown/trailing JSON, optional empty body; Pagination — defaults 20/0, limit 1–100 и offset≥0;
- `websocket/handler.go`: code — код ошибки протокола, а не приглашения; replyTo длиннее 36 символов опускается.

Нативный Go scanner подтвердил одинаковые token streams во всех девяти comment-only файлах, включая автоматически вставляемые `;` и string literals. Кода с `Code generated`, `DO NOT EDIT`, `go:generate` либо действующего генератора этих комментариев не найдено. Production Go/TS scan не обнаружил TODO/FIXME/HACK и `fmt.Print*`/`console.log/debug`/`debugger`, требующих удаления. Вывод CLI-аудита и резервного копирования оставлен намеренно.

### 7. Tests и логические batches

Новые тесты, повторяющие простое объединение двух одинаковых helpers, не добавлялись. Существующие opt-in контракты и assertions не менялись, skipped tests не удалялись. Результаты разделены по scope:

| Batch | Изменение | Проверка |
|---|---|---|
| A naming | `directActionID`/`groupParameter` → `uuidParameter` | Проверка всех 15 call sites и параметров, HTTP contracts |
| B local simplification | Один понятный путь UUID validation вместо двух копий | Exact body equivalence, те же ранние возвраты |
| C comments/debug | SFU и error/SSE/HTTP/WS contracts | Go scanner identity, focused normal/race |
| D small duplication | Удалена вторая копия UUID helper | 5 affected packages normal/race с реальным Redis |
| E tests/docs | Сохранён harness; актуализированы три руководства | 21 локальная Markdown-ссылка существует, сверка с routes/auth/backend |
| F dead/dependencies | Один лишний import; прочие удаления не обоснованы | Staticcheck и `go mod tidy -diff` без замечаний |

Независимый review проверил UUID merge относительно snapshot, второй — все четыре SFU comment diffs. Семантических изменений не найдено. Изменения не коммитились одним большим commit, существующий staging не сбрасывался.

### 8. Docs consistency changes

Обновлены `FRONTEND_GUIDE.md`, `frontend.md`, `USER_GUIDE.md` по фактическому коду:

- MeetSpace вместо устаревшего default Meetrix;
- `/history/*` перенаправляет в `/conferences?view=past`, `/search` и `/app/search` — в `/app`;
- страницы записей, папки, личные/групповые чаты, действия главной и мобильная навигация;
- постоянная серверная авторизация, refresh выбранного аккаунта и переключение аккаунтов;
- гостевой вход по приглашению, отдельные «Аудио»/«Видео», раздел «Аккаунты», выход из sidebar;
- существующее удаление записей с действительными backend-ограничениями;
- серверный поиск переписок и поиск сообщений;
- наличие opt-in Go audit harness вместо неверного утверждения об отсутствии всех тестов.

Дополнительно уточнено: realtime events обновляют локальное состояние; серверные snapshot/reconciliation его восстанавливают. Это документация существующей системы. API/UI ради текстов не изменялись, исторические отчёты не переписывались под новые результаты.

### 9. Dependencies

Удалено и обновлено зависимостей: **0**. `go mod tidy -diff` завершился с exit 0 и пустым diff. Go module files, frontend manifests/lock и frontend source не менялись. TS noUnusedLocals/noUnusedParameters уже включены; оснований для удаления runtime dependencies или CSS без проверки использования не найдено.

### 10. Public compatibility

Сохранены REST routes, JSON fields/null-vs-empty, safe error texts/statuses, WS/Rabbit envelopes, Redis keys, DB schema, storage paths, config defaults и публичные Go signatures. P2 package imports, type fields/interfaces и списки public declarations совпадают до/после. Нормализация `chat.UUID`, порядок validation и прекращение запроса до repository при ошибке сохранены.

В Pion/FFmpeg lifecycle, concurrency model и RTP hot path исполняемых изменений нет. Не менялись logging ownership и error taxonomy. Удалён только private duplicate helper; клиентам не нужна миграция.

### 11. Test / race / lint results

| Проверка | До | После |
|---|---|---|
| `go test -count=1 ./...` | PASS | PASS |
| `go test -race -count=1 ./...` | PASS | PASS |
| `go vet ./...` | PASS | PASS |
| staticcheck v0.7.0 | PASS | PASS |
| govulncheck v1.8.0 | PASS с оговоркой ниже | Та же оговорка |
| Frontend Prettier | PASS | PASS |
| `go mod tidy -diff` | Не требовалось повторять P2 | PASS, без diff |

Vulncheck: 0 достижимых уязвимостей и 0 в импортируемых packages; остаётся 1 advisory в требуемом модуле вне импортируемых packages. Обновлений dependencies в косметическом этапе нет.

Обычный `go test ./...` не включает `.go.txt` templates и сам по себе проверяет сборку. Дополнительное runtime evidence P3:

- **12 top-level контрактных tests в 6 packages**, normal и race, настоящий Redis/Pion, без skips: HTTP errors, group presence/UUID, SSE limits/authorization/cleanup, WS errors и rejected Join ownership.
- **WS/Redis:** reconnect после disconnect и transient prune failure, 1020 connect/disconnect/reconnect циклов двух hubs; на последнем checkpoint 14 goroutines / 15 FD, local sockets/Redis keys/connected sessions = 0.
- **SFU:** 10 полных lifecycle + 10 source-churn циклов с race; после shutdown 2 goroutines / 6 FD, rooms/peers/tracks/subscriptions/recording outputs/audio taps = 0.
- **AMQP:** cancellation/blocked write/close/handshake contracts с race ×3 — PASS.
- **Key API:** HTTP error/presence/UUID/WS contracts плюс реальные PostgreSQL group creation authorization/roles/atomic rollback — PASS. Это не полный браузерный e2e всех страниц.
- **FFmpeg:** настоящие process/cancellation/pipe/decoder/finalization tests — PASS; после cleanup child processes/temp files/temp bytes = 0.

Первый media запуск внутри sandbox не смог привязать UDP/mDNS socket (`operation not permitted`) и получил timeout. Сохранён `before/media-sandbox.log`; повтор **того же before binary** с разрешёнными локальными соединениями прошёл. Это ограничение среды, не скрытый исправленный продуктовый дефект.

После проверок удалены только три собственных fixture-контейнера, их anonymous volumes и сеть; `preexisting_not_running_at_cleanup: []`. Generated test binaries и собственный локальный build cache удалены. Source snapshots, JSON, logs и pprof сохранены под ignored `tmp/p3-audit-20261016/`.

### 12. P1 performance regression result

Перед edits сохранены исходники и собраны before binaries. After binaries используют тот же performance harness. Apple M5, Go1.26.9 darwin/arm64; timed suites выполнялись последовательно, отдельно от сборок и race/static jobs.

**RTP microbenchmarks:** 6×300ms для 160/1200-byte payload и 0/1/2 sinks. Allocation counts и bytes/op совпадают: 0/0 без sinks, 176 B/1 alloc для audio, 1280 B/1 alloc для video (в том числе два sinks). Geomean времени −0,73%; значимого замедления не обнаружено. Это не заявление об ускорении продукта.

**Реальные Pion media scenarios:** 22s warmup и 5s measurement. Clients и server работают в одном процессе; CPU/allocations включают обе стороны:

| Сценарий | Packets до→после | CPU seconds до→после | Allocated bytes до→после |
|---|---:|---:|---:|
| 2 peers | 1000→1000 | 0,346→0,323 | 5 232 848→5 225 952 |
| 5 peers + screen + recording/audio tap | 11 000→11 000 | 1,161→1,036 | 37 795 776→37 756 120 |

Application packet/recording/audio-tap drops = 0; после cleanup владельцы rooms/peers/tracks/subscriptions/recording/audio taps обнулены. Регрессия в этих репрезентативных P3 сценариях не обнаружена. Доказанное token identity дополнительно подтверждает отсутствие нового кода/allocations на media пути.

FFmpeg smoke также измерил 12 finalizations двумя параллельными pipelines над уже закодированными 4s 640×360, 10 fps H264/AAC segments: p50 86,082ms, max88,766ms; 8 durable fixture-файлов до/после, temp size стабилен. Это один новый after measurement, не статистическое сравнение ускорения; он исключает SFU/capture/SQL/upload/fixture encoding.

Нестабильный 10-peer throughput из P2 здесь не переизмерялся и не объявляется исправленным. Production capacity, WAN/TURN и полный recording end-to-end SLO этими локальными проверками не подтверждаются. Evidence: `rtp-benchstat.txt`, `before/media/`, `after/media/`, `after/p0-*`, `after/recording/`.

### 13. Files changed (только текущий P3)

Go — 13 файлов:

- `internal/app/personal/{handler,group_handler,direct_actions_handler,pins_handler}.go`
- `internal/app/notifications/{events,streams}.go`
- `internal/app/httpresponse/response.go`
- `internal/domain/apperrors/errors.go`
- `internal/transport/websocket/handler.go`
- `internal/infrastructure/sfu/{manager,peer,tracks,egress}.go`

Документы — 4 файла:

- `docs/FRONTEND_GUIDE.md`
- `docs/frontend.md`
- `docs/USER_GUIDE.md`
- `docs/code-review/P3_CLEANUP_REPORT.md`

Новых production файлов, новых versioned test tools и migrations нет. Предыдущие P1/P2 изменения сохранены отдельно от этого списка.

### 14. Deferred non-cosmetic issues

- Строгий performance sign-off на 10 участников требует стабильного нагрузочного окружения; прежний P2 result остаётся inconclusive.
- Крупные Capture/mediaworker handlers, ORM type в domain, broad repositories и typed call transitions — P2 debt, не косметические правки.
- Не выполнялась массовая чистка всех шаблонных комментариев. За текущими понятными границами нужны отдельная проверка точности и небольшой reviewable diff.
- Не проводились изменение UI, браузерный e2e, production deployment или новый refactoring stage.

P3 остановлен после отчёта; дальнейшие этапы автоматически не запускались.

---

## Исторический проход — 2026-10-10

Статус: **P3 завершён; baseline/after gates, runtime smoke и representative performance — PASS.** Baseline: HEAD `a0781257debba3f911ee180067cc0fad5bb06204` **плюс существовавшие P0/P1/P2/UI-правки**, не чистый checkout. Текущий [P2](P2_BEFORE_AFTER.md) завершён без открытых race/resource blockers. История 6 октября ниже сохранена отдельно и не выдаётся за свежие результаты. Evidence: `tmp/p3-cleanup-20261010/` (ignored). Рабочий Docker и удалённый сервер не обновлялись; следующий этап не начат.

### 1. Executive summary / scope

Небольшая чистка в существующих пакетах: понятные имена и менее вложенный цикл расчёта присутствия, один общий шаг подтверждения обработки уведомления, однократное вычисление длины поисковой строки, актуальные комментарии и документация личных чатов. Нет изменения архитектуры, интерфейса продукта, миграций, потоков/таймеров, Pion/FFmpeg lifecycle или media hot path. Новых production helpers/interfaces/packages/config flags не добавлено.

Проверены naming/локальные conditions, comments/TODO/debug, зависимости и мелкий dead code. Карта P2 содержит 73 production packages; P3 не повторяет архитектурное разделение. Membership, physical session и media peer сохраняют разные роли; Conference в backend и «Встреча» в UI не переименовываются. Однозначные короткие локальные `ctx`, `err`, `i`, `n` сохранены. Безопасного основания для split больших lifecycle-файлов либо удаления compatibility/recovery кода не найдено.

### 2. Naming changes

В `analytics.Aggregate`: `cid→conferenceID`, `groups→intervalsByParticipant`, `ids→participantIDs`, `id→participantID`, `list→participantIntervals`, `v→interval`, `duration→participationMS`, `times→changeTimes`, `bucket→bucketMS`. Девять смысловых переименований явно различают конференцию, участников, интервалы и единицы измерения. Типы и порядок аргументов неизменны; комментарий аргумента согласован с новым именем. Публичные identifiers/types/events и пакеты не переименованы.

### 3. Local simplifications

Цикл merge сначала добавляет первый/непересекающийся интервал и продолжает обход; оставшийся guard только продлевает последний интервал. Соприкасающиеся границы объединяются, вложенный интервал не укорачивает внешний. Сортировка, обрезка границами встречи, отбрасывание пустых/обратных интервалов, порядок участников, неизменность входа и пустые JSON-массивы сохранены.

| Метрика выбранной функции | До | После |
|---|---:|---:|
| Aggregate: LOC / AST branch nodes | 62 / 13 | 62 / 13 |
| Вложенность условий внутри merge loop | 2 | 1 |
| Notifications.Tick: LOC / AST branch nodes | 30 / 8 | 26 / 7 |
| ActionService.Search: LOC / AST branch nodes | 10 / 2 | 11 / 2 |

LOC — span объявления, branch nodes — if/for/range/case/comm включая closures, не cyclomatic complexity. Исходные значения — P2 after inventory для неизменённых тогда файлов; текущие — `inventory-after.json`. Уменьшение строк не является KPI: имя `queryLength` добавило строку, сделав проверку понятнее.

### 4. Duplicate code removed

- `Notifications.Tick`: два одинаковых `Published + error return` заменены одним после условной публикации. Подавленное уведомление по-прежнему отмечается обработанным без создания события; ошибка публикации не подтверждается, ошибки останавливают дальнейший обход. Generate→Pending→Publishable→Publish(если разрешено)→Published, стабильный event ID, recipient и payload сохранены.
- `ActionService.Search`: `RuneCountInString` вызывается один раз, результат — `queryLength`. Trim, UTF-8 validation, 2–200 символов, limit1–50, текст ошибки, validation precedence и repository forwarding прежние. Для invalid UTF-8 это чистое вычисление также выполняется; публичный результат тот же. Generic helper не вводится.

### 5. Dead code removed

Новых доказанно неиспользуемых функций, файлов или private types для удаления не найдено. Staticcheck чистый; экспортируемый compatibility/debug/recovery код не удаляется по отсутствию прямого локального caller. Функций и файлов удалено **0**. Старые skipped/opt-in tests сохранены; новая проверка не маскируется удалением сценариев.

### 6. Comments / TODO / debug cleanup

Исправлены реальные неточности: `ratelimit.Result.ResetAt` — значение `time.Time`, не pointer; ноль не формирует `X-RateLimit-Reset`. `Notification.PublishedAt` означает завершение публикации **или подавления** доставки; `Repository.Published` описывает тот же контракт. `Cursor.At` — CreatedAt граничного уведомления, нулевое время недопустимо, а не «nil допустим». Неочевидный стабильный ID повторной доставки оставлен с WHY-комментарием.

Production Go/TS/TSX scan не нашёл TODO/FIXME/HACK и `fmt.Print*`/`println`/console debug, требующих удаления. FFmpeg child-process test output и диагностические инструменты сохранены. Generated code не редактировался; масштабной косметической чистки комментариев не было.

### 7. Test cleanup и batches

Новые небольшие регрессионные тесты сначала прошли на прежнем production-коде, затем после изменений. Analytics: точные duration/timeline, non-nil empty collections, порядок участников и неизменность входа (3 table cases плюс прежние 2 теста). Notifications: 8 сценариев порядка вызовов/ошибок, suppression, стабильного envelope/payload; отдельный nil/empty pending. Search: 9 Unicode/limit/precedence cases и сохранение repository result/error. Старые assertions и skips не удалялись.

| Batch | Действие | Проверка |
|---|---|---|
| A | Семантические локальные имена analytics | Unit/race до и после, checkpoint `analytics/A-naming.go.txt` |
| B | Guard/continue в merge loop | Unit/race, точные boundary assertions, root diff review |
| C | Неточные comments + Publish/ack wording | Relevant unit/race, source contract review |
| D | Общий ack и queryLength | Unit/race до/после, независимый read-only diff review |
| E | Новые явные tests и актуальная документация | Unit/race + сверка docs с routes/usecase/hooks |
| F | Dependency/dead-code audit без принудительного удаления | Staticcheck, tidy-diff, manifests |

Первая версия нового notification assertion ошибочно сравнивала `json.RawMessage` с Go map. Этот **дефект написанного теста** исправлен до production refactor: сравниваются сериализованные bytes. Исходный failed log сохранён, затем прежний production прошёл исправленные unit/race (`behavior-tests-before-corrected.log`, `behavior-tests-before-race.log`). Это не продуктовый bug fix.

### 8. Docs consistency

`docs/PERSONAL_MESSAGES.md` теперь описывает существующий `/conversations/{id}/peer-presence`, account/ACL boundary, лимит, 2s lookup, unknown→503, общий foreground polling для шапки/информации. Удалены ложные утверждения об отсутствии статуса в UI и локальном поиске: список фильтруется сервером, курсор привязан к фильтрам. Уточнены групповые аватары ссылкой на существующий `GROUP_CHATS.md`. Маршруты и UI ради документации не менялись.

### 9. Dependencies

Удалено/обновлено зависимостей **0**. `go mod tidy -diff` до и после завершился с exit0 без diff. Go module files, frontend manifest/lock и весь frontend побайтно сохранены относительно текущего baseline. Нет нового framework, generics или DI слоя.

### 10. Public compatibility

HTTP routes/status/error strings, JSON fields/tags/null-vs-array, WS/Rabbit event names/envelope, SQL schema, Redis keys и storage paths не менялись. Порядок side effects/ошибок подтверждён call-order тестами и diff review. Публичные сигнатуры по типам и порядку параметров сохранены; изменение имени аргумента Aggregate не меняет Go API. Границы P2, leases, locks, defer/cancel lifetime, таймауты и retries остались прежними. Нет скрытого product bug fix.

### 11. Test / race / lint results

Свежие baseline **и after**: `go test -count=1 ./...`, `go test -race -count=1 ./...`, `go vet ./...`, staticcheck v0.7.0, govulncheck v1.8.0, frontend Prettier lint и `go mod tidy -diff` — PASS. Gofmt/diff checks чистые. Govulncheck: 0 reachable и 0 imported-package vulnerabilities; 1 required-module advisory `GO-2026-5932` для неимпортируемого `golang.org/x/crypto/openpgp` — не заявляется «все зависимости без уязвимостей».

Финальный runtime runner завершился с exit0; полный source manifest неизменен на всём интервале. Opt-in проверки запускались с настоящими отдельными зависимостями, не с пользовательскими данными:

| Проверка | Результат |
|---|---|
| Account + conference WS lifecycle/recovery, по 1020 соединений | PASS 17.450s, восстановление после Redis disconnect и prune failure |
| 100 одновременных WS reconnect, race | PASS 3.897s |
| 23 key API/admission/membership/presence/unread/recording tests, race | PASS 42.422s |
| 7 selected P3 integration tests: analytics, search, notification delivery/locks/dedup | PASS 7.144s, PostgreSQL/Redis |
| SFU lifecycle + source churn (по 5 циклов), 2/3/5 peers, audio-tap isolation, race | PASS 60.201s |
| Две реальные recording pipelines со screen/source changes | PASS 16.25s, FFmpeg/MinIO/PostgreSQL/Redis/Rabbit |

В этих выбранных наборах нет skips/race warnings. Записи подтверждены декодированием video/audio: 2 комнаты, 2 восстановления closed segments, 2 auto-stop при завершении встречи (`after/recording-pod/recording/full-stack-checks.json`). Lifecycle cleanup: conference WS после прогрева G14/FD14, local sockets/session rows/Redis keys0 во всех завершённых раундах; SFU после source churn G2/FD6/rooms/peers/tracks/subscriptions0. Pion EOF diagnostics при teardown сохранены в сыром логе; assertions и race проверки успешны. Это короткие smoke, не универсальное доказательство отсутствия утечек на любой нагрузке.

Все созданные тестовые контейнеры/сети/volumes удалены; 131 исходный контейнер (включая остановленные) сохранил ID/StartedAt/running state. Перед очисткой native fixtures — 0 тестовых БД и Redis keys; leftovers0. Receipts: `after/cleanup-final.json`, `after/recording-pod/recording/cleanup.json`. Default all-package tests не выдаются за полное opt-in покрытие; только перечисленные отдельные запуски подтверждают соответствующие сценарии.

### 12. P1 performance preservation

Свежие RTP baseline/after: `BenchmarkP1RecordPacket`, 6 variants ×6 samples, benchtime1s, Go1.26.9 darwin/arm64 на Apple M5 Pro/18 logical CPUs/48GiB. Команды одинаковы, прогоны последовательны, без других агентских тяжёлых задач. Shared host, не dedicated capacity lab. Benchstat: `egress-comparison.txt`.

| Payload / sinks | До ns/op | После ns/op | p-value | B/op / allocs до = после |
|---|---:|---:|---:|---:|
| 160 / 0 | 7.539 | 7.387 | .009 | 0 / 0 |
| 160 / 1 | 133.2 | 133.8 | .903 | 176 / 1 |
| 160 / 2 | 165.8 | 164.4 | .331 | 176 / 1 |
| 1200 / 0 | 7.662 | 7.401 | .002 | 0 / 0 |
| 1200 / 1 | 225.0 | 229.0 | .132 | 1280 / 1 |
| 1200 / 2 | 256.8 | 257.8 | .513 | 1280 / 1 |

Значимых временных различий при активных sinks не найдено; B/op и allocs/op одинаковы во всех samples. Незначительное по абсолютной величине ускорение no-sink веток (0.15–0.26ns) статистически различимо, но **не приписывается P3**: SFU код не менялся. Существенной регрессии representative RTP workload не выявлено. Нет обещания общей производительности продукта, эквивалентности любой latency или ускорения от чистки. FFmpeg finalization benchmark и полный DB/Redis performance corpus повторно не выполнялись: их реализация не затронута; реальная запись и query-budget/ACL regressions прошли отдельно.

### 13. Files changed in this P3 pass

1. `internal/usecase/analytics/analytics.go`
2. `internal/usecase/analytics/analytics_test.go`
3. `internal/usecase/notifications/service.go`
4. `internal/usecase/notifications/service_test.go` (новый)
5. `internal/usecase/chat/actions.go`
6. `internal/usecase/chat/actions_search_test.go` (новый)
7. `internal/domain/notifications/notification.go`
8. `internal/domain/ratelimit/result.go`
9. `docs/PERSONAL_MESSAGES.md`
10. `docs/code-review/P3_CLEANUP_REPORT.md`

Итого 10 файлов, только 3 production файла с исполняемыми изменениями; 2 domain файла — comments only. Остальные существовавшие P0/P1/P2/UI изменения сохранены, не считаются результатом P3. Staged user file не тронут; автоматических commits/deploy нет. Evidence helpers остаются в ignored tmp.

### 14. Deferred non-cosmetic issues

Новых продуктовых дефектов в выбранном scope не выявлено. Сохраняется долг P2: Composer.Capture, mediaworker.command, integrations.Fanout, persistence JSON coupling в records, legacy/recovery compatibility и исключения error mapping. Они не переписываются ради меньшего LOC. Browser/device/WAN/TURN/long-soak и полный P1 DB/Redis corpus в P3 не входят; существующие query-budget/ACL tests повторяются. Следующий этап автоматически не начинается. Docker рабочего проекта и удалённый сервер не обновляются.

---

## Исторический проход — 2026-10-06

Дата: 2026-10-06. Baseline: `4aecd7db0939c7efaeeab9d30e7a629a004f493c`.
Предыдущие этапы: [P0](P0_RELIABILITY_AUDIT.md), [P1](P1_BEFORE_AFTER.md),
[P2](P2_BEFORE_AFTER.md). Открытых P0 race/resource blockers в этих результатах нет.
Локальные логи и измерения: `tmp/p3-cleanup-20261006/evidence/` (ignored).

## 1. Итог и scope

Выполнена ограниченная чистка: локальные имена в приглашениях, recording service
и сборке presence; четыре упрощения условий; два повторяющихся вычисления;
документация выбранных contracts; одно прежнее замечание staticcheck в тесте.
Функциональные ошибки в рамках P3 не исправлялись. Новых helpers, packages,
интерфейсов, конфигурации и зависимостей не добавлено.

Naming audit опирался на карту 67 production packages из P2 и просмотр
conferences/realtime/media/recordings/auth, transport и их callers. Различаются:
Participant — сохранённое членство; realtime.Session — физическое WS-подключение;
media peer — Pion-подключение; Recording — задача записи и её durable state.
Имена публичных типов, ошибок, событий и пакетов сохранены. Conference в backend
и «Встреча» в UI обозначают одну сущность; массовое переименование не требуется.
Крупные файлы не дробились: разделение service/dispatcher и WS lifecycle/dispatch
уже сделано в P2, а оставшиеся крупные lifecycle-функции требуют отдельной задачи.

## 2. Naming changes

| Область | Изменения | Причина |
| --- | --- | --- |
| Приглашения | `actor → actorID`, `conference → conferenceID`, `id → userID`, `seenUsers → seenUserIDs`, `clean → normalized` | Видно, какие IDs передаются и что содержит запрос после нормализации |
| Импорт домена приглашений | `d → domain`, также в соответствующем тесте | Совпадает с соседними conference usecases и раскрывает роль типов |
| Recording service | `seconds → segmentDurationSec`, `id → recordID`, `kind → eventType` | Длительность сегмента, UUID записи и тип события имеют отдельный смысл |
| Presence snapshot | `active → activeSessions`, `byParticipant → connectionsByParticipant`, `s → activeSession`, `p → participant`, `ids → connectionIDs`, `moderator → isModerator` | Физические подключения явно сгруппированы по членству участника |

Короткие `ctx`, `err`, `tx`, `i` и однозначные IDs в малом scope оставлены.
Сигнатуры методов по типам и порядку аргументов не менялись.

## 3. Local simplifications

- `stateFor`: ранний возврат полного roster для модератора; фильтрация обычного
  участника читается без внешнего `if`. Максимальная вложенность этих ветвей 3 → 2.
- `Hub.state`: недоступные/отсутствующие connection IDs сразу становятся пустым
  срезом вместо промежуточного присваивания nil. Сортировка и JSON `[]` сохранены.
- `Logout`: вложенные проверки `err == nil` и пустого scope объединены. Порядок
  отзыва cookie, SID и legacy JWT прежний; ошибки и guest boundary сохранены.
- `ValidateRecordStatusFilter`: пустой и поддерживаемый фильтр используют один
  успешный return; short-circuit и публичный текст ошибки прежние.

Defer, cancellation, locks, cleanup timing и порядок I/O не изменялись.

## 4. Duplicate code removed

В invitation usecase число Unicode-символов поискового запроса вычисляется один
раз в `queryLength`; количество адресов и аккаунтов — один раз в `recipientCount`.
Порог 2–100 символов и лимит 1–20 исходных получателей сохранены. Лимит проверяется
до deduplication; порядок email/UserID validation и порядок результата прежние.
Общий helper для разных правил email и UUID не вводился.

## 5. Dead code

Staticcheck после исправления S1024 не выявил неиспользуемых private declarations.
Доказанного дополнительного dead code для удаления не найдено; функций и файлов
не удалено. Legacy routes, recovery, compatibility branches и диагностические
инструменты сохранены: они имеют callers либо opt-in tests.

## 6. Comments, TODO и debug

Переписана документация 21 выбранной декларации в recordings service,
realtime/media domain и presence snapshot. Сокращены 139 избыточных строк
документации; это показатель объёма чистки, а не производительности.

Уточнены contracts: Start/Stop запрашивают durable operation, worker освобождает
ресурсы; publication best-effort; Identity содержит также durable auth session;
Session принадлежит физическому WS-соединению; Publication.TrackID обозначает
поколение browser capture. Убраны неверные описания обычных time.Time как pointers.
В RecordingNotice комментарий различает звук для всех и текстовую плашку для
остальных участников. Исполнение компонента не менялось.

В отслеживаемых Go/TS/TSX файлах отсутствуют TODO/FIXME/HACK и стандартные
`Code generated … DO NOT EDIT` markers. Production `internal/` и `cmd/` не содержат
`fmt.Print*`/`println` debug-вызовов. Печать в FFmpeg subprocess tests и диагностических
CLI сохранена. Массовая чистка оставшегося callback boilerplate не выполнялась.

## 7. Test cleanup и batches

В persistent auth test выражение заменено на `time.Until(expiry)` (S1024).
Сценарии, assertions и границы токена/сессии сохранены. В invitation test
согласован import alias. Тесты не удалялись, skips не скрывались.

| Batch | Изменения | Проверки после batch |
| --- | --- | --- |
| A | Локальные имена | conferences/recordings/realtime/WS unit + race + diff check |
| B | Условия presence, logout, status filter | Relevant unit/race + PostgreSQL persistent auth (3 сценария) |
| C | Comments contracts и RecordingNotice | Relevant unit/race + 58 frontend tests в 3 файлах |
| D | Повторные вычисления invitation validation | Unit/race + 6 PostgreSQL invitation scenarios |
| E | S1024, test alias, документация | Unit/race; затем общий regression suite |
| F | Dead code и зависимости: изменений не требуется | Staticcheck, `go mod tidy -diff`, сравнение manifests, общие gates |

## 8. Docs consistency

`auth-conferences-api.md` больше не утверждает, что WS/SFU/гости/чат/запись
отсутствуют: первый этап отделён от актуального обзора [API.md](../API.md).
`conference-recording.md` исправляет противоречия: запуск — любой допущенный
участник с аккаунтом; stop — owner или инициатор; гости не управляют записью.
Английское голосовое уведомление получают все допущенные участники, включая
инициатора. Условие MP4/preview относится к composite. Исторические результаты
приёмки помечены как исторические, а текущие режимы связаны с API overview.

## 9. Dependencies

Удалённых/обновлённых зависимостей нет. `go mod tidy -diff` завершился с exit 0,
без diff. `go.mod`, `go.sum`, `frontend/package.json` и lockfile побайтно совпадают
с baseline. Загрузка недостающих модулей инструментом tidy изменила только cache.

## 10. Public compatibility

AST-сравнение всех 108 top-level declarations в семи изменённых production Go
файлах подтвердило одинаковые function/interface signatures, structs, fields,
tags, constants и variables после нормализации import aliases и имён аргументов.
Поля JSON/GORM, HTTP routes/status/errors, WS/Rabbit events, Redis keys, migrations
и storage paths не изменялись. Package boundaries и DI из P2 сохранены.
Срез connection IDs по-прежнему сериализуется как `[]`; nil/error returns и
порядок коллекций сохраняют прежний контракт. Pion/FFmpeg lifecycle и RTP path
не редактировались. Frontend diff содержит только комментарии.

## 11. Test, race, lint и lifecycle regression

Baseline gates выполнены до правок. Go 1.26.6, macOS arm64, Apple M5.

| Проверка | До | После |
| --- | --- | --- |
| `go test -count=1 ./...` | PASS, 16.336 s | PASS, 16.602 s |
| `go test -race -count=1 ./...` | PASS, 20.564 s | PASS, 22.828 s |
| `go vet ./...` | PASS | PASS |
| staticcheck v0.7.0 | Только прежний S1024 | PASS, без замечаний |
| govulncheck v1.8.0 | 0 вызываемых vulnerabilities | 0 вызываемых vulnerabilities |
| Frontend Prettier lint | PASS | PASS |
| `git diff --check` / gofmt изменённых Go файлов | Чистый baseline | PASS |

Govulncheck в обоих прогонах также сообщает об одной vulnerability в required
module без импортированного/вызываемого уязвимого кода. Это прежний результат,
не утверждение об отсутствии любых уязвимостей в dependencies.

Дополнительные обязательные smoke, с opt-in flags и реальными зависимостями:

- **API/auth/invitations/moderation/outbox + authenticated WS→SFU:** 20 top-level
  integration tests, PostgreSQL/Redis, race, PASS; 38.002 s. Skips в этом наборе нет.
- **SFU:** обычные full suites включают Opus/VP8 smoke для 2/3/5 peers и late join.
- **WS connect/reconnect:** recovery после Redis subscription disconnect,
  transient prune failure и 1020 lifecycles; race, PASS; 16.849 s. После раундов:
  local sockets=0, connected sessions=0, presence keys=0, goroutines=14. FD:
  11 idle → 13 warm → 14 после первого раунда, затем 14 до четвёртого; роста
  по последующим раундам не обнаружено. Это короткая нагрузка, не long soak.
- **Screen/recording:** Linux pod с реальными PostgreSQL/Redis/RabbitMQ/MinIO,
  Pion и FFmpeg; два параллельных pipelines, PASS; 24.768 s. Декодированный
  video/audio, recovery закрытых сегментов и conference auto-stop подтверждены
  в обеих комнатах (`full-stack-checks.json`: 2/2/2).
- **FFmpeg:** Opus→live captions и все четыре recording modes, PASS; 1.235 s.
  FFmpeg на macOS host отсутствует, поэтому эти проверки выполнены в cached
  Linux image. Они не выдаются за пройденные host tests.
- **Frontend comments:** 58 tests для notices, audio announcement и recording
  realtime, PASS. Browser E2E, TURN, реальное оборудование и внешние SMTP/STT/AI
  providers в этот этап не входили.

Тестовый Redis удалён с DBsize=0. Recording pod удалил свои containers/networks/
volumes; leftovers=0. Все 11 исходных контейнеров приложения сохранили start time
и running state. Release/deploy и создание commit агентом не выполнялись.

## 12. P1 representative performance

`BenchmarkP1RecordPacket`, 6 samples на вариант, benchtime 250 ms; до/после
последовательно, без конкурирующих audit tests во время benchmark.
ns/op — медианы; bytes/allocs совпали во всех samples.

| Payload / sinks | До ns/op | После ns/op | Δ | B/op / allocs до = после |
| --- | ---: | ---: | ---: | --- |
| 160 / 0 | 7.609 | 7.923 | +4.13% | 0 / 0 |
| 160 / 1 | 141.20 | 140.15 | −0.74% | 176 / 1 |
| 160 / 2 | 174.25 | 174.10 | −0.09% | 176 / 1 |
| 1200 / 0 | 7.660 | 7.808 | +1.93% | 0 / 0 |
| 1200 / 1 | 221.45 | 217.50 | −1.78% | 1280 / 1 |
| 1200 / 2 | 255.25 | 254.20 | −0.41% | 1280 / 1 |

Существенной регрессии representative RTP workload не обнаружено. При наличии
recorder sinks медианы немного ниже baseline; ускорение продукту не приписывается.
Для 0 sinks изменение составляет 0.15–0.31 ns/op, значения находятся в диапазоне
наблюдаемых samples до/после. Сам packet path не менялся; это не доказательство
нулевого изменения любой latency на общей пользовательской машине. DB workload
и длительные capacity measurements повторно не запускались: их реализация не
затронута, baseline/ограничения остаются в P1/P2 отчётах.

## 13. Files changed

- `internal/usecase/conferences/invitations.go`
- `internal/usecase/conferences/invitations_test.go`
- `internal/usecase/recordings/service.go`
- `internal/usecase/realtime/hub.go`
- `internal/usecase/auth/sessions.go`
- `internal/domain/records/validation.go`
- `internal/domain/realtime/realtime.go`
- `internal/domain/media/media.go`
- `tests/integration/persistent_auth_sessions_test.go`
- `frontend/src/components/RecordingNotice.tsx`
- `docs/auth-conferences-api.md`
- `docs/conference-recording.md`
- `docs/code-review/P3_CLEANUP_REPORT.md`

Evidence tooling остаётся в ignored tmp, вне production tree и dependency graph.

## 14. Deferred non-cosmetic issues

Новых поведенческих дефектов, требующих отдельного reproducer/fix, в выбранном
scope не выявлено. Сохраняется отдельный долг из P2: крупные Composer.Capture,
mediaworker.command и integrations.Fanout; concrete recorder orchestration;
GORM datatypes в records domain; совместное размещение media Controller/HTTPClient;
trusted worker error text и persisted diagnostics. Они требуют архитектурного
или compatibility review и не маскируются под косметику. Перечень и обоснования —
[P2 §17](P2_BEFORE_AFTER.md#17-remaining-p2-debt).

P3 завершён. Следующий refactoring stage автоматически не начинается.
