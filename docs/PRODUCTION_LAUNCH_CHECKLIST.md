# Решение о запуске production

Отмечайте пункты после проверки конкретного выпуска. Для каждого подтверждения
сохраните дату, ответственного и evidence.

## Ответственность и артефакт

- [ ] Назначены владелец выпуска, оператор, принимающий rollback-решение и контакты инцидента.
- [ ] Утверждены release notes, scope, известные ограничения и окно обслуживания.
- [ ] Registry либо archive release собран из clean commit; версия/SHA/time/source hash совпадают с manifest.
- [ ] Зафиксирована фактическая топология: standalone мастер, classic либо hardened shared-host. Выбраны соответствующие overlays; нет случайной замены MinIO на SeaweedFS или томов/сети.
- [ ] Все image references неизменяемы, checksum manifest проверен; предыдущий пакет доступен без пересборки.
- [ ] Текущие CI gates gofmt/vet/staticcheck/build и frontend lint/typecheck/build прошли для этого commit.
- [ ] Критические сценарии вручную проверены в Chromium и Firefox; CSP проверена в ответах и браузере, ограничения зафиксированы.
- [ ] govulncheck/npm audit/image scan прошли, SBOM сохранены; HIGH/CRITICAL не обходятся флагом; ограничения отчётов зафиксированы.
- [ ] Docker/registry/CI runner и `TRIVY_IMAGE` проверены; production не использует legacy `deploy-dev`.

## Инфраструктура и безопасность

- [ ] Staging независим от production, DNS и HTTPS/WSS доступны с реальных клиентских сетей.
- [ ] TURNS доверен браузерами; UDP/TCP/relay firewall проверен, выбранная ICE pair действительно relay при forced test.
- [ ] SFU public IP/NAT/mux и TURN relay range согласованы с Compose/firewall; проверены корпоративная и мобильная сети.
- [ ] PG/Redis/Rabbit/S3/private control/metrics/pprof не доступны публично; admin требует серверное право.
- [ ] Origins/CSP/CORS/trusted proxy настроены точно; IP rate limit различает клиентов и не доверяет произвольному forwarding.
- [ ] Все secrets независимы, вне Git/образов, файлы защищены; provider encryption key включён в защищённый recovery-план.
- [ ] Ротация JWT/service/metrics/TURN/provider secrets проверена; logout не ошибочно считается отзывом всех JWT.
- [ ] Диски/spool/volumes, права UID 1000, резерв места, CPU/RAM и FD соответствуют измеренной нагрузке.
- [ ] Принят риск single-host SPOF и отсутствия бесшовной миграции живых комнат; план outage понятен.
- [ ] Доставка alert ответственному проверена: наличие правила в Prometheus не считается доставленным уведомлением.

## Схема и данные

- [ ] `AUTO_MIGRATE=false`, migration запускается явно один раз до deploy; ledger/checksums не переписаны.
- [ ] Миграции и блокировки измерены на репрезентативной копии; старый код проверен на новой схеме.
- [ ] Согласованный write freeze/backup выполнен; все писатели, uploads и очереди учтены.
- [ ] Restore-check выполнен на отдельном target; checksum/privacy/signed access и бизнес-связи проверены.
- [ ] Утверждены реальные RPO/RTO, срок хранения, защита off-host backup и решение о потерях при disaster recovery.
- [ ] Осознаны ограничения backup: текущие версии объектов/ContentType, без history/tags/произвольной metadata и broker/spool snapshot.
- [ ] Восстановленные тестовые БД/bucket и тестовые fixtures учтены для адресной последующей очистки.

## Продуктовые сценарии

- [ ] Согласованный non-admin аккаунт проверяет login/me/capabilities и отказ admin; секреты не попадают в отчёт.
- [ ] Регистрация, вход/выход, профиль и подтверждённые ограничения восстановления пароля соответствуют USER_GUIDE.
- [ ] Создание, приглашение, prejoin, waiting room/admission, роли и reconnect прошли в браузерах.
- [ ] Камера/микрофон/screen share проверены на реальных устройствах, разрешениях, смене сети и нескольких участниках.
- [ ] Запись каждого включённого режима стартует/останавливается; ffprobe и playback подтверждают media. Composite/screen_focus имеют MP4/JPEG, audio_only — AAC в MP4 без preview, individual_tracks — audio MP4 и архив исходных треков. Доступ проверен.
- [ ] Личные и групповые чаты, поиск в переписке, папки, звонки и история звонков проверены; вложения и download защищены. Завершённые встречи открываются через `/conferences?view=past`, записи — `/recordings`.
- [ ] Поиск названий встреч и сообщений соблюдает права доступа; email/push/calendar проверены с настроенными провайдерами.
- [ ] Email/push/calendar/OAuth, если включены, проверены с реальными credentials, consent и revoke/refresh; по умолчанию остаются noop/disabled.
- [ ] Телеметрия, если включена, не передаёт текст ошибок/контент/URL с ID/токены; backend rate limit и opt-out проверены.
- [ ] Пользовательская доступность и адаптивность проверены вручную; наличие сборки не объявлено сертификацией доступности.

## Репетиция, запуск и наблюдение

- [ ] На настоящем staging пройдены migration → deploy → smoke/media → rollback → повторный deploy того же пакета.
- [ ] Проверен drain активных WS/комнат/recordings/jobs; timeout не вызывает скрытый force restart.
- [ ] До recorder drain прекращены новые `record.start`, обработаны pending starts и штатно завершены записи; учтена возможная задержка stop из-за requeue в общей очереди.
- [ ] Заполнено правдивое version-bound evidence, production approval равен выбранной версии.
- [ ] Согласован объём пилота: отдельный deployment либо глобальные флаги; не обещан несуществующий cohort targeting.
- [ ] До запуска выбраны наблюдаемые SLI, ожидаемый объём трафика и условия остановки; SLO основаны на измерениях целевого окружения.
- [ ] После запуска версии/health/auth/WS/media/recordings/очереди/storage проверены; фактические counts и окно наблюдения сохранены.
- [ ] Ответственный явно принял go/no-go; при no-go выполнен безопасный rollback/forward fix и уведомление.

## Evidence запуска

| Поле | Заполняется оператором |
| --- | --- |
| Версия / commit / hash manifest | Не заполнено |
| Staging URL / production URL | Заполнить для конкретного выпуска |
| CI, scan, browser и media evidence | Не заполнено |
| Backup / restore-check / rollback rehearsal | Не заполнено |
| Начало / конец / число реальных операций наблюдения | Не заполнено |
| Решение / ответственный / время | Не принято этим документом; заполнить по фактической приёмке |

Процедуры: [DEPLOYMENT](DEPLOYMENT.md), [RELEASE_PROCESS](RELEASE_PROCESS.md),
[ROLLBACK](ROLLBACK.md), [backup/restore](operations/backup-restore.md),
[runbook](operations/README.md), [продуктовый checklist](operations/RELEASE_CHECKLIST.md).
