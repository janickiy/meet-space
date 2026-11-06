# Находки комплексной оптимизации MeetSpace

Дата: 2026-11-06. База: `1e27a55`; diff — ветка `codex/full-optimization-20261106`.
Подтверждённых новых P0 в проверенных сценариях не найдено. Это не заключение об
отсутствии всех уязвимостей или ошибок. Границы покрытия перечислены ниже.

## Подтверждённые исправления

| ID | Приоритет | Исходник / причина / риск | Доказательство и проверка | Статус |
|---|---|---|---|---|
| MEDIA-01 | P1 | `internal/infrastructure/sfu/manager.go`, новый `tcp_listener.go`: закрывался listener, но не ICE mux и принятые сокеты вне peer registry; socket переживал успешный Shutdown | `TestShutdownClosesUnclaimedTCPConnection` FAIL до исправления; TCP/STUN/idle/50-close циклы, `-race -count=20` PASS | Исправлено: владение mux и принятыми соединениями, закрытие при гонке Accept/Close |
| WORKER-01 | P1 | `internal/infrastructure/rabbitmq/commands.go`: ожидание mutex игнорировало deadline; уже отменённый запрос закрывал здоровое общее AMQP-соединение | Два исходных FAIL: deadline20ms ждёт >1s; canceled request закрывает conn. Реальный RabbitMQ start/stop/reconnect/shutdown, `-race -count=20` PASS | Исправлено: context-aware semaphore, ранняя проверка отмены; отмена начавшегося I/O сохраняет принудительное закрытие |
| DB-01 | P1 | `internal/infrastructure/postgres/chat_notifications.go`: `UUID::text` в коррелированных EXISTS мешал использовать индекс ID сообщения; цена росла с историей | 30k сообщений/100 уведомлений; до Seq Scan 51/100 раз, после PK Index Scan. `TestAuditNotificationHistoryUsesMessageIndex` и equivalence/ACL test | Исправлено: CASE с проверкой канонического UUID из JSON, без миграций/новых индексов |
| REALTIME-01 | P1 | `internal/usecase/realtime/hub.go`: событие комнаты резервировало slice на все сокеты процесса | `BenchmarkEntriesSmallRoom`: 81 920 → 24 B/op при 10k сокетов/2 адресатах; тест ready/room/global filtering | Исправлено: ёмкость локальной выборки соответствует совпадениям; глобальный snapshot сохранён |
| FE-01 | P1 | `frontend/src/api.ts`: 204 возвращался до проверки identity; avatar JSON проверял аккаунт только после заголовков | До исправления FAIL для 204/profile/group avatar после смены аккаунта. `api-response-lifecycle.test.mjs` | Исправлено: проверка версии до 204 и после полного JSON. Это клиентская изоляция, не доказанный серверный ACL bypass |
| FE-02 | P1 | `frontend/src/api.ts`: catch JSON проглатывал AbortError, выдавал 502/запускал recovery при 401 | Отменённое чтение 200/401, сохранённый успешный 204 и malformed JSON; 8 новых тестов | Исправлено: отмена остаётся отменой, stale binary errors не запускают действия новой сессии |
| FE-03 | P1 | `main.tsx`, `landing/metadata.ts`: все тексты документации включались в стартовый JS рабочего приложения | Chrome `/login`: gzip 222 020 → 170 498 B; 17 публичных страниц с прежними main/H1/SEO/JSON-LD, SPA search PASS | Исправлено: короткий docs-catalog, lazy PublicPage и совместимая SSR Suspense boundary |
| MEDIA-02 | P2 | `sfu/egress.go`: промежуточный список и второй обход для одной эксклюзивной подписки записи на каждом RTP | Benchmark и тесты эксклюзивности, независимого буфера и concurrent close. Аллокации не уменьшились; активный выигрыш не доказан | Упрощено: один egress под прежней блокировкой, без изменения RTP/ACL |
| CI-01 | P2 | `.gitlab-ci.yml`: admin frontend отсутствовал в validation/security, `cmd/admin` не входил в список Go artifacts | Фактический список jobs и `cmd/admin`; локальные admin tests/build/audit PASS, YAML parse/round-trip PASS | Добавлены `admin-check`, `security-admin`, сборка admin CLI. Запуск GitLab pipeline требует runner |
| CLEAN-01 | P3 | `internal/domain/records/library_test.go`: `t.Fatal(fmt.Sprintf(...))` нарушал настроенный staticcheck | S1038 до исправления; staticcheck после PASS | Заменено на Fatalf, удалён один ненужный импорт |
| CLEAN-02 | P3 | `go.mod`: уже импортируемые напрямую `x/sync` и `yaml.v3` отмечены indirect | `go mod tidy -diff`, `go mod verify`; версии и go.sum неизменны | Исправлена только классификация двух зависимостей |

Основные точки исправлений на момент отчёта:

| ID | Файл:строка в итоговой рабочей копии |
|---|---|
| MEDIA-01 | `internal/infrastructure/sfu/manager.go:211`, `internal/infrastructure/sfu/tcp_listener.go:30` |
| WORKER-01 | `internal/infrastructure/rabbitmq/commands.go:182` |
| DB-01 | `internal/infrastructure/postgres/chat_notifications.go:55` |
| REALTIME-01 | `internal/usecase/realtime/hub.go:929` |
| FE-01 / FE-02 | `frontend/src/api.ts:232`, `frontend/src/api.ts:370` |
| FE-03 | `frontend/src/main.tsx:34`, `frontend/src/landing/docs-catalog.ts:1`, `frontend/src/entry-public.tsx:1` |
| MEDIA-02 | `internal/infrastructure/sfu/egress.go:234` |
| CI-01 | `.gitlab-ci.yml:71`, `.gitlab-ci.yml:98`, `.gitlab-ci.yml:112` |
| CLEAN-01 / CLEAN-02 | `internal/domain/records/library_test.go:15`, `go.mod:25` |

Номера строк меняются при совместной разработке. Исходным результатом аудита был
рабочий diff относительно указанного SHA и новые тесты; последующая фиксация и
слияние изменений отражаются в истории Git.

## Проверки доступа и отказов

DB equivalence сравнивает новый и исходный предикаты на реальном PostgreSQL:
некорректные/пустые/null/нестроковые UUID, верхний регистр, UUID без дефисов/в скобках,
перевод строки; muted/hidden/cleared/deleted сообщения и группы, уход из группы,
собственные сообщения, участник waiting/kicked/rejected/left, настройки чата встречи.
`CASE` не полагается на порядок вычисления WHERE и не расширяет старую семантику.
План проверяет точное имя `id`, а не подстроку `conversation_id`, и присутствие узла.

Админка проверена read-only: отдельные opaque administrator sessions,
Secure/HttpOnly/SameSite=Strict cookie с path `/api/v1/admin`, username/password,
Origin+CSRF на мутациях, актуальная DB-авторизация, защита последнего администратора.
Публичные Gin и Nginx отвергают administrative namespace, включая проверенные
encoded paths. Internal HTTP port не считается заменой HTTPS или авторизации.
Реальные изолированные Nginx enabled/disabled тесты: 2/2 PASS.

## Оставшийся backlog и непроверенные гипотезы

| ID | Приоритет проверки | Место / гипотеза | Что требуется / почему оставлено |
|---|---|---|---|
| VERIFY-01 | P1 | Полная цепочка записи/outbox/FFmpeg/S3 при падении процесса | Отдельный полнофункциональный стенд, синтетическое AV, crash между upload и DB commit, disk-full/partial S3, повторные команды. Unit tests subprocess и broker reconnect не доказывают весь сценарий |
| VERIFY-02 | P1 | SFU `peer.go`/`tracks.go`, медленный ICE/TCP получатель, смена сети | Нагрузочный multi-browser RTP/RTCP, TURN, loss/jitter, Firefox/Safari/Android, длительные сессии. Блокирующий TCP write — гипотеза, настройки без профиля не менялись |
| VERIFY-03 | P1 | Длительный рост browser heap | Admin 180 s: +429 532 B, DOM/listeners стабильны. Нужны 30–60 min, retaining paths и реальные данные; короткий прогон не доказывает ни утечку, ни её отсутствие |
| VERIFY-04 | P1 | Границы производительности API/БД/worker pools | p50/p95/p99/RPS, CPU/RSS/fd/GC под согласованной нагрузкой. SQL microbenchmark не переносится на производственный SLA |
| VERIFY-05 | P1 | Совместимость установки/обновления production-size | Прогнать полный installer/upgrade/rollback и restore на копии данных/объектов с реальными TLS/TURN. Проверены config, unit/proxy tests и миграции на синтетической БД |
| PROFILE-01 | P2 | `sfu/tracks.go`: снимок подписчиков на RTP | Профиль mutex/CPU/allocs под fanout; immutable snapshot не введён без доказательства выгоды |
| PROFILE-02 | P2 | `realtime/hub.go:entries`: обход всех локальных сокетов | После исправления объёма памяти сложность O(N) остаётся; индекс комнат потребует отдельного профиля и lifecycle tests |
| PROFILE-03 | P2 | Frontend infinite-query, крупные истории и списки | Реальные большие наборы, React profiler и paging/network trace; не добавлялась виртуализация без подтверждения |
| TOOL-01 | P2 | GitLab jobs и shellcheck | YAML проверен и команды выполнялись локально; pipeline на GitLab не запускался, shellcheck отсутствует |
| CANDIDATE-01 | P3 | `frontend/src/pages/SelfHostedPage.tsx` | Нет рабочего импорта из текущей маршрутизации, но не доказана безопасность для внешних сборщиков; оставлен |
| CANDIDATE-02 | P3 | Игнорируемая `tmp/` исходной папки | Содержит локальные helpers, попадавшие в исходный `go list ./...`; принадлежит рабочему окружению пользователя. Не удалять без проверки использования; isolated worktree их не содержит |

Уязвимости из advisory feed, которые не вызываются текущим Go-кодом, не удалялись
путём слепого обновления зависимостей. Скан не гарантирует безопасность всех runtime-
конфигураций; после изменения использования зависимостей его нужно повторить.
