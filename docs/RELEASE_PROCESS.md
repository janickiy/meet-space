# Выпуск неизменяемых артефактов

Цепочка выпуска: исходники → validate/security → сборка →
scan/SBOM/package → staging → ручная проверка/наблюдение → решение оператора → production →
проверка либо откат. Evidence заполняется для конкретного выпуска и окружения.

**Область применения команд:** общий release workflow из
`scripts/release/common.sh`. Для установки через `install.sh` сначала прочитайте
[различия Compose-топологий](DEPLOYMENT.md#выбор-процедуры-по-типу-установки).
`manage.sh` не содержит updater; `release.sh` не использует автоматически
сгенерированные мастером overlays. Архив выпуска и runtime-топология — отдельные
характеристики, обе должны сохраняться при обновлении.

## Версия, артефакты и CI

Политика тегов: `vX.Y.Z`, допускается prerelease вроде `v1.2.3-rc.1`.
Major отражает несовместимые контракты, minor — совместимые возможности,
patch — совместимые исправления; даже patch отдельно оценивается по SQL и config.
Release notes составляются по [шаблону](RELEASE_NOTES_TEMPLATE.md).

Каждый выпуск связывается с полным Git SHA, UTC-временем и fingerprint исходников.
Registry release требует чистого source; package сверяет version/commit и source
fingerprint с OCI labels, а staging/production требуют
`sourceFingerprintVerified:true`. `--allow-dirty-local` допускается только
для локальной репетиции и фиксируется `sourceDirty:true`. Это не production-релиз.
Публичные `/version` и `/version.json` сверяются с манифестом, а не с mutable tag.

Текущий [GitLab pipeline](../.gitlab-ci.yml):

| Gate | Проверки |
| --- | --- |
| Validate | gofmt, vet, staticcheck, синтаксис release-скриптов; frontend lint, TypeScript, production build |
| Security | govulncheck; npm audit, блокирующий high/critical |
| Build | Бинарники приложения |
| Package тега | Docker build, SBOM и scan всех release-образов; high/critical блокируют пакет; публикация по digest |

CI проверяет код, сборку и безопасность. Пользовательские сценарии, браузеры,
доступность и нагрузку проверяют отдельно по [чеклисту](operations/RELEASE_CHECKLIST.md).

`release-package` требует отдельного Docker runner, registry credentials и
проверенного `TRIVY_IMAGE=…@sha256:…`. `deploy-dev` синхронизирует исходники
и пересобирает development-окружение; для production используется описанная
ниже процедура с неизменяемыми артефактами.

## Сборка и упаковка один раз

Примеры выполняются из корня checkout с подготовленными инструментами и
Docker. Каталоги/версии выбираются новыми; существующий release tag не перезаписывается.

```bash
RELEASE_VERSION=v1.2.3
RELEASE_COMMIT="$(git rev-parse HEAD)"
ARTIFACT_DIRECTORY=/absolute/artifacts/v1.2.3
bash scripts/release/build.sh --version "$RELEASE_VERSION" --commit "$RELEASE_COMMIT" --registry registry.example.org/team/meet --output "$ARTIFACT_DIRECTORY/build"
bash scripts/release/package.sh --build "$ARTIFACT_DIRECTORY/build/build.json" --output "$ARTIFACT_DIRECTORY/package" --mode registry
```

Это bootstrap-пакет для первого развёртывания. При обычном обновлении приложения
добавьте к команде упаковки `--infrastructure-manifest
/absolute/artifacts/previous/release.json`. Пакет сохранит **точно те же** образы
S3-хранилища, PostgreSQL, Redis, RabbitMQ, Coturn, proxy, Prometheus
и Grafana (только classic); новыми
будут только шесть служб приложения. Отдельно собранный новый образ S3 в этом
случае не используется. Предыдущий манифест должен быть проверен оператором и
иметь тот же режим (`local`, `registry` или `archive`) и совместимую топологию
classic/hardened; смена формата S3-тома этим флагом не выполняется. Его SHA-256 записывается в новый
манифест, а security scan повторяется и для сохранённой инфраструктуры.

В CI передайте предыдущий манифест через защищённую File variable
`RELEASE_INFRASTRUCTURE_MANIFEST`; job автоматически добавит этот флаг. Только
для первого развёртывания оставьте File variable пустой и явно задайте
`RELEASE_BOOTSTRAP=true`. Без выбранного режима job останавливается до сборки;
одновременно включать оба режима нельзя. Автоматически выбирать «последний» tag или менять инфраструктуру
вместе с кодом приложения запрещено. Изменение любого инфраструктурного digest
оформляется отдельным обслуживанием с backup, проверкой совместимости данных и
собственным планом отката. Обычный `release.sh deploy` такое изменение отклоняет.

Адрес registry в примере нужно заменить. Учётные данные Docker login получают
из secret store/CI, пароль передают через stdin, не аргумент командной строки.
`TRIVY_IMAGE` задаётся до package и должен быть закреплён digest. Флаг frontend
телеметрии `CLIENT_TELEMETRY_ENABLED=true` задаётся перед build при явном решении
его включить; по умолчанию отправки нет.

Для локальной репетиции: собственный prefix, например `meet-release-local`,
при необходимости `--allow-dirty-local` у build, `--mode local` у package.
`--skip-security-local` разрешён только локально и означает **непроверенный**
пакет. Local package сохраняет image ID, `images.tar`, `release.json`,
`SHA256SUMS`; registry package сохраняет registry digests, scan и SBOM.
Неполную сборку после ошибки не считать готовым пакетом и не дописывать поля
`securityScanned` вручную. Неизменяемость манифеста не делает неизвестный
источник автоматически доверенным.

Операторские бинарники тоже готовятся заранее в CI/на build-host и проверяются
для архитектуры машины запуска, а не собираются при аварийном rollback.
Образы включают служебные migration/backup
бинарники. Режим `--mode archive` создаёт проверенный переносимый архив и
принимается staging/production при чистом source и пройденном security gate;
`local` предназначен только для репетиции. `build.sh --layout hardened-shared-host`
выбирает hardened-образы с SeaweedFS; по умолчанию build/CI используют classic.
Текущий `install.sh` собирает hardened archive, проверяет его и применяет
standalone-конфигурацию через `wizard.py`. При переносе local archive сначала сверяют `SHA256SUMS`, затем
загружают архив Docker; во время инцидента старый commit не пересобирают.

## Миграции

Текущий набор — 35 `.up.sql`. В установке мастером тот же мигратор вызывается
сервисом `migrate` с её полным Compose-контекстом; приведённые ниже команды
`release.sh migrate` относятся к общему workflow.

Один назначенный оператор/CI job выполняет `release.sh migrate` **до** запуска
нового приложения. В production Compose `AUTO_MIGRATE=false`: старт проверяет
ledger, но не выполняет DDL. `release_schema_migrations` содержит имя файла,
SHA-256 точных байтов, время применения и duration. Применённый SQL нельзя
переименовывать, переводить комментарии или редактировать: добавляется новая миграция.

Migration job сериализован PostgreSQL advisory lock, использует транзакцию,
`lock_timeout=15s` и общий deadline 5 минут. При ошибке batch откатывается и
promotion прекращается. Старая БД без ledger один раз проходит имеющиеся
идемпотентные up-файлы; слепое заполнение ledger не предусмотрено. Измеряйте
длительность и блокировки на представительной копии, включая индексы миграции
`000021`. Не переносите короткий локальный замер на большую production-БД.

Рискованные изменения: Expand → совместимый deploy → backfill/verify → Contract
в отдельном последующем выпуске. Rollback приложения не выполняет DB down.
Ledger допускает более новые миграции, но не доказывает семантическую совместимость
старого кода; она проверяется на rehearsal и описывается в release notes.

`000022_remove_raised_hands` удаляет счётчик и время последнего поднятия руки
из аналитики. Функция исключена из API, realtime и интерфейса. Перед применением
нужен backup: down-скрипт восстанавливает только колонки, не прежние значения.
Старый API несовместим с этой схемой; обновление требует окна обслуживания:
остановить старые экземпляры API, применить миграцию, запустить новую версию.
Не смешивать версии в rolling deployment. Откат старого API возможен только
после согласованного восстановления схемы; при необходимости прежних счётчиков —
из backup. Старые временные ключи Redis истекают по часовому TTL и новой версией
не читаются и не создаются; очищать весь Redis не нужно.

## Проверка и продвижение

На staging выполнить подготовку baseline, write freeze/backup, migration,
deploy нового пакета, служебные и ручные проверки, forced relay/TURNS, запись
с ffprobe/playback и проверкой S3, затем откат и повторный deploy того же пакета.
Сохранять результаты, времена, версии, ограничения и отказы. Процедура backup:
[backup-restore.md](operations/backup-restore.md); команды deploy —
[DEPLOYMENT.md](DEPLOYMENT.md).

После установки пакета оператор выполняет проверки существующими аккаунтами:

1. Сверить публичные `/version` и `/version.json` с точными version/commit выбранного
   манифеста. Проверить HTTP-ответ главной страницы и загрузку её ресурсов.
2. Через закрытый доступ проверить `/health/live` и `/health/ready` каждого
   изменённого сервиса; готовность должна быть подтверждена, drain — выключен.
   Проверить очереди, БД/Redis/S3 и отсутствие роста ошибок в метриках/логах.
   Внутренние health, metrics и operations не открывать в публичном proxy.
3. С согласованным существующим non-admin аккаунтом вручную проверить вход,
   `auth/me`, capabilities, ICE config, запрет admin и изменённые экраны.
   JWT, cookies и пароли в evidence и shared-файлы не сохранять.
4. С согласия участников проверить WS/media, forced relay/TURNS и доступ к
   ранее созданным записям/материалам. Новые встречи или записи создавать только
   как отдельную явно согласованную проверку функционала, не автоматически при выпуске.
5. Записать фактические версии, результаты, время и ограничения в evidence.
   Успешные health/version не подтверждают реальный RTP, TURN, FFmpeg, файлы,
   waiting room или полный browser UX; эти возможности проверяются отдельно.

Пример безопасных публичных GET-проверок (сопоставление с манифестом обязательно):

```bash
curl --fail --silent --show-error https://meet-staging.example.org/version
curl --fail --silent --show-error https://meet-staging.example.org/version.json
curl --fail --silent --show-error --output /dev/null https://meet-staging.example.org/
```

## Approval production

Для каждой изменяющей production-операции нужен существующий абсолютный
`--evidence` JSON и `RELEASE_PRODUCTION_APPROVAL`, точно равный версии выбранного
манифеста. Evidence должен описывать **реальные** проверки и содержать:

```json
{
  "version": "v1.2.3",
  "commit": "FULL_40_CHARACTER_SHA",
  "manifestSHA256": "SHA256_OF_EXACT_RELEASE_JSON",
  "migrationCompatible": false,
  "backupVerified": false,
  "rollbackReady": false,
  "stagingSmokePassed": false,
  "mediaSmokePassed": false,
  "securityPassed": false,
  "applicationRollbackCompatible": false,
  "approvedBy": "",
  "evidenceReferences": []
}
```

Этот пример намеренно не проходит gate. Для deploy/migrate/drain/resume нужны
первые шесть подтверждений совместимости/backup/rollback/staging/media/security;
для rollback вместо трёх последних проверяется `applicationRollbackCompatible`.
`stagingSmokePassed` и `mediaSmokePassed` означают документированные проверки
staging и медиа. Непроведённые проверки остаются `false`.
`approvedBy` и `evidenceReferences` обязательны. Commit и hash манифеста должны
совпасть побайтно. Скрипт проверяет структуру утверждений, не истинность подписанта
и не подлинность стороннего отчёта; проверка evidence — ответственность оператора.

```bash
export RELEASE_PRODUCTION_APPROVAL=v1.2.3
bash scripts/release/release.sh deploy --environment production --project recorder-production --env-file /srv/meet/config/production.env --manifest /srv/meet/releases/v1.2.3/release.json --evidence /srv/meet/evidence/v1.2.3.json
unset RELEASE_PRODUCTION_APPROVAL
```

## Поэтапное включение и наблюдение

Встроенного targeting по пользователям/accounts и traffic-splitting canary нет.
Флаги конфигурации действуют на развёртывание. Реализуемый минимум: проверить
выпуск в отдельном изолированном окружении/пилотном deployment, затем вручную
включить согласованный набор функций в целевом env и пересоздать совместимые
процессы. Не направляйте разные экземпляры одного сервиса на несогласованные
флаги или provider contracts. Выключение флага не удаляет ранее созданные данные.

После deploy проверить HTTP/auth/WS, media join/ICE/TURN, завершение записей,
backlog Rabbit/PG jobs, БД/Redis/S3, provider errors и клиентские ошибки.
Предлагаемый порядок наблюдения и SLI — [runbook](operations/README.md).
Системные ошибки auth, media/WS, повреждение/недоступность записи, несовместимость
схемы или неконтролируемый рост ресурсов блокируют продвижение и требуют
решения об отключении вспомогательной функции либо [rollback](ROLLBACK.md).
