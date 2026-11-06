# MeetSpace: итог комплексного аудита и оптимизации

Дата: 6 ноября 2026 года. Исходный commit: `1e27a55b881ac64c2fa5ad15f09a96e825b24328`.

Выполнены обзор архитектуры и контрактов, проверки серверной части, SFU, очередей,
БД, frontend, существующей админки, установщика и CI. Исправлены семь подтверждённых
проблем P1, два пункта P2 и два пункта P3. Подтверждённых новых P0 в проверенных
сценариях не обнаружено. Полные длительные испытания медиатрафика, записи и
восстановления всей установки остаются отдельной работой; их результаты не подменяются
unit-тестами или короткими локальными замерами.

Изменения находятся в **`/Users/aleksandranickij/htdocs/meetspace-audit-20261106`**,
ветка **`codex/full-optimization-20261106`**. Это отдельная рабочая копия, созданная
после внешнего переключения исходного каталога на другую ветку во время аудита.
Пользовательские staged-документы сохранены. На момент завершения локального аудита
commit, push, merge и развёртывание на удалённом сервере ещё не выполнялись.
Последующая публикация изменений фиксируется в истории Git; результаты проверок
ниже относятся к завершённому аудиту.

## Что исправлено

| Область | Проблема и результат |
|---|---|
| SFU / сетевые ресурсы | Shutdown теперь закрывает ICE mux и принятые TCP-соединения, включая ожидающие первого STUN-пакета и не принадлежащие зарегистрированному peer. Закрытие защищено от гонки с Accept |
| RabbitMQ / команды записи | Ожидание занятого publisher учитывает deadline. Отменённый ожидающий запрос больше не закрывает исправное общее соединение и не вызывает ненужный reconnect |
| PostgreSQL / уведомления | Убрано приведение индексируемых UUID-столбцов к тексту. JSON ID проверяются перед приведением к UUID; повреждённые значения безопасно исключаются. ACL и прежнее требование канонического UUID сохранены |
| Realtime / память | Для события небольшой комнаты slice адресатов больше не резервируется по числу всех сокетов процесса. Полный обход остаётся O(N) |
| Frontend / смена аккаунтов | Поздний ответ 204 и результат загрузки аватара после смены аккаунта не возвращаются как успешный результат новой сессии |
| Frontend / отмена запросов | AbortError при чтении JSON больше не превращается в 502 и не запускает ненужное восстановление сессии при 401 |
| Frontend / загрузка | Полные статьи документации исключены из стартового JS рабочего приложения. Публичные страницы загружаются лениво, каталог SEO содержит только slug/title. Пререндеринг и содержание сохранены |
| SFU / горячий путь | Упрощена передача RTP единственной эксклюзивной подписке записи. Уменьшение аллокаций и достоверное ускорение активной записи не заявляются |
| CI / админка | Добавлены typecheck, tests, build и npm audit для существующей админки; административный CLI включён в Go build artifacts |
| Уборка | Исправлено предупреждение staticcheck в тесте; две уже используемые напрямую Go-зависимости помечены direct. Версии зависимостей и go.sum не изменились |

Публичные маршруты, JSON/WS-события, формат команд записи, схема БД и миграции
не менялись. Проверки доступа, TLS и лимиты не ослаблялись. Админка не создавалась
заново: проверялась её существующая реализация.

## Измеренный эффект

| Локальный сценарий | До | После |
|---|---:|---:|
| Страница уведомлений: median SQL execution, 30 000 сообщений / 100 уведомлений | 211.765 ms | 1.012 ms |
| Счётчик непрочитанных на тех же данных | 318.192 ms | 0.756 ms |
| Выбор двух адресатов комнаты при 10 000 локальных сокетов | 81 920 B/op | 24 B/op |
| Фактически загруженный JS `/login`, gzip | 222 020 B | 170 498 B |

В PostgreSQL повторный Seq Scan заменился индексным поиском по ID. Начальная
JS-загрузка стала меньше на 23.21%; суммарный размер всех JS chunks немного вырос
из-за разделения. Microbenchmark realtime теперь делает две небольшие аллокации
вместо одной большой. Эти результаты не являются замером RPS, TTI или ускорения
всей платформы. Методика, пять повторов и разброс — в
[сравнении производительности](meetspace-performance-before-after.md).

## Выполненные проверки

Среда: Go 1.26.10, Node 24.22.1, macOS arm64 / Apple M5, Docker 29.4.2.
Системный Node 23 не соответствует engines проекта; проверки выполнены на Node 24.

| Команда или сценарий | Итог |
|---|---|
| `go build ./...`, `go vet ./...` | PASS до и после изменений |
| `go test ./...`, `go test -race -count=1 ./...` | Исходный прогон PASS; внешние интеграции проверены дополнительно |
| `bash scripts/audit-optimization.sh` | Полный итоговый `go test -mod=readonly -race -count=1 ./...`: 34 пакета PASS, 45 без тестов, 0 FAIL. Реальные PostgreSQL, Redis и RabbitMQ в одноразовых контейнерах |
| `go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...` | Исходно S1038; после исправления PASS |
| `go mod tidy`, `go mod verify` | Только две direct/indirect переклассификации; проверка модулей PASS |
| `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...` | 0 вызываемых уязвимостей; отчёт отмечает 2 на уровне импортируемых пакетов и 2 на уровне модулей без найденных вызовов |
| SFU TCP/egress `go test -race ... -count=20` | PASS; реальные loopback sockets, отмена/закрытие, владение RTP-буфером |
| RabbitMQ `go test -race ... -count=20` | PASS; отмена ожидания, живое соединение, принудительный transport close, reconnect, consumer shutdown |
| Redis lifecycle `go test -race ... -count=10` | PASS; presence, 25 pubsub close циклов, одноразовый ticket под конкуренцией, два устройства |
| PostgreSQL `TestAuditNotification*` | PASS после исправления. Исходный код с тем же fixture не проходит проверку индексного плана, но проходит сравнение ACL-семантики |
| `npm test` в frontend | 59 PASS, включая 8 новых регрессий; до изменений 51 PASS |
| `npm run lint`, `npm run build` в frontend | PASS; build включает TypeScript check. Обычная и marketing-only сборки |
| `npm test`, `npm run build` в admin | 34 PASS, TypeScript/build PASS; отдельного lint script в проекте нет |
| `npm audit --audit-level=high` в frontend и admin | Exit 0, 0 найденных npm-уязвимостей в обоих отчётах |
| Chrome: публичные страницы | 17 маршрутов, совпадение main/H1/title/description/canonical/JSON-LD до/после, SPA-навигация и поиск, без JS-ошибок |
| Chrome: admin regression/cycles/soak | 8 regression checks; 100 переходов, 50 модальных циклов, 10 login/logout; 180 s soak. DOM/listeners стабильны; вывод о плато heap не сделан |
| `python3 -I -m unittest discover -s scripts/install -p 'test_*.py' -v` | 38 tests: 36 PASS, 2 opt-in proxy tests затем выполнены отдельно |
| `RUN_ADMIN_PROXY_TESTS=1 python3 -I -m unittest discover -s scripts/install -p test_admin_proxy.py -v` | 2/2 PASS на реальном изолированном Nginx, включая encoded-path обходы |
| `python3 -I -m unittest discover -s scripts/release -p 'test_*.py' -v` | 4/4 PASS |
| Compose `config --quiet` | 11 корректных сочетаний профилей PASS; синтетические настройки из примеров |
| `bash -n`, gofmt, `git diff --check` | 23 исходных shell scripts и новый runner корректны; Go formatting и diff без замечаний |
| `.gitlab-ci.yml` | Prettier YAML parse/round-trip PASS. Удалённый GitLab pipeline не запускался |

Список команд приведён с рабочими каталогами в названиях строк. Логи и JSON
сохранены в [evidence/2026-11-06](evidence/2026-11-06/).
Пустой log означает отсутствие вывода команды, а не самостоятельное доказательство
успеха; exit status 0 наблюдался при выполнении и отражён в таблице выше.
Baseline SQL/клиентские regression-логи намеренно содержат FAIL до исправления.

## Как повторить основные проверки

Из корня рабочей копии, с версиями Go из go.mod и Node из engines:

```sh
go build ./...
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
bash scripts/audit-optimization.sh
npm --prefix frontend ci
npm --prefix frontend run check
npm --prefix admin ci
npm --prefix admin test
npm --prefix admin run build
```

Runner требует уже загруженные Docker images `postgres:16-alpine`, `redis:7-alpine`,
`rabbitmq:4.2.9-management`. Он создаёт пустую БД, задаёт случайные loopback-порты,
лимиты ресурсов и tmpfs, выполняет реальные интеграционные тесты, удаляет только
свои container ID. Рабочий `.env`, production-сервисы и volumes не используются.
Без этих переменных окружения обычный `go test` пропускает opt-in интеграции;
это не равно проверке PostgreSQL/Redis/RabbitMQ.

Для браузерных проверок нужен установленный Playwright и Chrome; путь к модулю
можно передать через `PLAYWRIGHT_MODULE_PATH`:

```sh
node frontend/tests/browser/public-routes.cjs frontend/dist /tmp/meetspace-public-routes.json
node admin/tests/browser/regressions.cjs admin/dist /tmp/meetspace-admin-regressions.json
node admin/tests/browser/cycles.cjs admin/dist /tmp/meetspace-admin-cycles.json
node admin/tests/browser/soak.cjs admin/dist /tmp/meetspace-admin-soak.json 180
```

## Оставшиеся проверки и риски

1. Нужен полный стенд записи с синтетическими AV, FFmpeg и S3: crash/restart между
   upload и DB commit, нехватка диска, недоступность storage, повторные команды.
   Проверенные broker/outbox/subprocess сценарии не покрывают всю цепочку.
2. Нужны длительные многопользовательские WebRTC-сессии с TURN, потерями пакетов,
   сменой сети и реальными Firefox/Safari/Android. Реальные микрофоны, камеры и
   демонстрация экрана в этой работе не проверялись.
3. В 180-секундном admin soak post-GC heap вырос на 429 532 B при стабильных DOM
   и listeners. Причина устойчивого роста не доказана; требуется 30–60 минут и
   анализ retaining paths. Заявлять отсутствие утечки по этому прогону нельзя.
4. Не измерялись API p50/p95/p99/RPS, production CPU/RSS/fd и ёмкость сервера.
   Профили RTP fanout, O(N) socket snapshot и крупных frontend-списков — backlog.
5. Не выполнены полная установка/upgrade/rollback/restore на копии production-size
   данных и реальный GitLab pipeline. Shellcheck локально отсутствует.

Спорные кандидаты на удаление оставлены на месте. Нет массового обновления
зависимостей, удаления тестов, миграций или рабочих конфигураций.

## Документы аудита

- [Исходная точка, архитектура и контракты](meetspace-optimization-baseline.md).
- [Находки, места изменений, регрессии и backlog](meetspace-findings.md).
- [Производительность: методика и значения до/после](meetspace-performance-before-after.md).
- [Безопасная уборка и оставленные кандидаты](meetspace-cleanup.md).
- [Журналы и измерения](evidence/2026-11-06/).
