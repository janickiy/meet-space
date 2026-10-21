# Откат приложения

Откат — запуск предыдущих **готовых неизменяемых образов** на совместимой
текущей схеме БД. Он не удаляет миграции и не восстанавливает потерянные данные.
На single-host Compose возможен перерыв; переноса живой комнаты между SFU нет.
Готовность rollback подтверждается репетицией конкретного выпуска.

Команды ниже относятся к общему release workflow (classic либо shared-host).
Для установки мастером нельзя подставлять её env/manifest в `release.sh`: он
выбирает другие overlays. Сначала сохраните и сверяйте полный
[Compose-контекст установки](DEPLOYMENT.md#compose-контекст-установки-мастером),
предыдущие image IDs, конфигурацию и неизменность томов/сети/proxy/TURN.
Готовой команды автоматического отката в `manage.sh` нет.

## Когда остановить продвижение

Не продолжать выпуск при несовпадении версий с manifest, провале readiness,
системных ошибках входа/WS/медиа, недоступных или повреждённых записях,
необъяснимом росте ошибок, очередей или ресурсов. Сначала зафиксировать время,
версии, безопасные логи и метрики. Не публиковать JWT, SDP, секреты, пользовательский
контент и подписанные URL. Для изолированного сбоя дополнительной функции можно
рассмотреть её отключение, но это не заменяет исправление повреждения данных.

## Предварительные условия

- Есть предыдущий `release.json`, его контрольная сумма, registry digests
  либо проверенный archive с immutable image IDs,
  доступные образы и соответствующий безопасный env. Старый commit не пересобирают.
- Старое приложение проверено с **уже применённой текущей** схемой и форматом
  данных. `release_schema_migrations` проверяет целостность SQL, не эту совместимость.
- Нет неразобранной смены PostgreSQL/Redis/RabbitMQ/S3/Coturn image:
  release helper намеренно не обновляет и не понижает эти зависимости автоматически.
- Есть проверенный согласованный backup и отдельное решение о допустимых потерях
  при восстановлении. Обычный application rollback не должен их вызывать.
- Назначены оператор и принимающий решение, согласованы окно и уведомление
  пользователей; прекращены новые встречи/записи, завершаются текущие.

Если совместимость не доказана, не выставляйте evidence в `true`. Возможные пути:
совместимый forward fix, отдельная миграция вперёд либо аварийное восстановление
согласованной БД и объектов с явно принятой потерей изменений после backup.
Автоматического DB down и автоматического production restore здесь нет.

## Последовательность

1. Приостановить продвижение, зафиксировать фактические образы каждого сервиса.
   При частичном deploy может работать смесь версий — не считать её целиком новой
   или старой. Не удалять данные, контейнерные volumes и каталоги записи.
2. Проверить предыдущий manifest и evidence, совместимость схемы, config и
   токенов. Дать активным встречам/записям завершиться; внешний admission/write
   freeze сохранять до окончания операции.
3. Выполнить `validate` и `rollback` с **предыдущим** manifest. Helper получает
   образы, выполняет drain перед заменой, ждёт нулевую активность и readiness
   последовательно для каждого приложения. Миграции не запускаются.
4. Проверить `/version`, `/version.json`, health, auth, WS/media, доступ к ранее
   созданным файлам и очереди вручную по [процессу выпуска](RELEASE_PROCESS.md#проверка-и-продвижение).
   Тестовые аккаунты и synthetic-конференции автоматически не создаются.
5. Зафиксировать начало/конец, простой и результаты. Возвращать нагрузку только
   после решения оператора и наблюдения, описанного в [runbook](operations/README.md).

Пример для staging; пути должны указывать на реальные проверенные артефакты:

```bash
PREVIOUS_MANIFEST=/srv/meet/releases/v1.2.2/release.json
bash scripts/release/release.sh validate --environment staging --project recorder-staging --env-file /srv/meet/config/staging.env --manifest "$PREVIOUS_MANIFEST"
bash scripts/release/release.sh rollback --environment staging --project recorder-staging --env-file /srv/meet/config/staging.env --manifest "$PREVIOUS_MANIFEST"
curl --fail --silent --show-error https://meet-staging.example.org/version
curl --fail --silent --show-error https://meet-staging.example.org/version.json
```

Production дополнительно требует approval, привязанный к **возвращаемой** версии:

```bash
export RELEASE_PRODUCTION_APPROVAL=v1.2.2
bash scripts/release/release.sh rollback --environment production --project recorder-production --env-file /srv/meet/config/production.env --manifest /srv/meet/releases/v1.2.2/release.json --evidence /srv/meet/evidence/rollback-to-v1.2.2.json
unset RELEASE_PRODUCTION_APPROVAL
curl --fail --silent --show-error https://meet.example.org/version
curl --fail --silent --show-error https://meet.example.org/version.json
```

GET-ответы сверить с точными version/commit возвращаемого манифеста; сами по себе
они не подтверждают готовность. Через закрытый доступ проверить health/readiness
каждого изменённого сервиса, отсутствие drain, очереди и метрики. Вход и доступ
к материалам проверить согласованным существующим аккаунтом без автоматического
создания пользователей/встреч. Не снимать write freeze до завершения этих проверок.

Evidence содержит версию, commit и SHA-256 **предыдущего** manifest;
`migrationCompatible`, `backupVerified`, `rollbackReady`,
`applicationRollbackCompatible` равны `true` только при подтверждении;
`approvedBy` и `evidenceReferences` заполнены. Полная схема и границы этой
ручной проверки — [RELEASE_PROCESS.md](RELEASE_PROCESS.md).

## Если операция остановилась

`RELEASE_DRAIN_TIMEOUT` по умолчанию 300 секунд; timeout не вызывает принудительный
restart. Процесс остаётся draining. Проверить активные комнаты/WS/записи/jobs,
дать им завершиться, затем повторить согласованную операцию. Старый выпуск без
drain endpoint требует отдельного maintenance-плана, обхода проверки нет.

`resume` нужен после намеренного общего drain для возобновления **той же** версии:
он пересоздаёт процессы, так как drain необратим до restart. Он не предназначен
для возврата другой версии и отказывается при несовпадении установленного образа.
Если все приложения уже переведены на нужный manifest, но остались draining,
использовать `release.sh resume` с этим manifest и обычными approval-gates.

После аварийного завершения helper может остаться lock-каталог проекта. Прежде
чем удалять ровно этот пустой lock, убедиться, что операции другого оператора/CI
не идут. Не применять массовое удаление lock-каталогов.

Восстановление данных — отдельная процедура [backup-restore.md](operations/backup-restore.md),
а не скрытая часть rollback.
