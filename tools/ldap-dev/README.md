# Локальный тестовый LDAP

Изолированный стенд LLDAP v0.6.3 для проверки корпоративного входа и синхронизации пользователей MeetSpace на Mac через Docker. Образ закреплён по digest в `compose.yml`. Стенд подключается через настройки LDAP в административной панели; он не заменяет проверку на настоящем Active Directory.

## Запуск

Из корня проекта:

```bash
cd tools/ldap-dev
python3 setup.py
```

Нужны Docker Compose, Python 3 и OpenSSL. Скрипт создаёт локальные TLS сертификаты, пароли, пользователей и группу, запускает контейнер и дожидается его готовности. Повторный запуск сохраняет существующие пароли, учётные записи и их права; он не возвращает изменённые вручную членства в исходное состояние.

Пароли находятся в `secrets/credentials.json`, сертификаты — в `secrets/tls/`, закрытые ключи — в `secrets/` и `secrets/tls/`. Каталог `secrets/` и `.env` исключены из Git; файл паролей создаётся с правами `0600`. Не переносите эти тестовые данные в рабочее окружение.

| Сервис | Адрес на Mac |
| --- | --- |
| Веб-панель | `http://127.0.0.1:17170` |
| LDAP без TLS, только для локальной диагностики | `ldap://127.0.0.1:3890` |
| LDAPS | `ldaps://127.0.0.1:6360` |

Базовый DN: `dc=meetspace,dc=test`; пользователи находятся в `ou=people`, группы — в `ou=groups`. DN для bind: `uid=svc-meetspace,ou=people,dc=meetspace,dc=test`. Учётная запись `svc-meetspace` включена в `lldap_strict_readonly` и предназначена для чтения каталога. `alice` и `bob` состоят в `meetspace-users`; `outsider` не состоит в этой группе, но **не заблокирован**. Пароли всех четырёх пользователей и `admin` записаны в `secrets/credentials.json`.

Проверка LDAPS и фильтра группы из каталога `tools/ldap-dev` (пароль bind вводится по запросу). Используется клиент OpenLDAP из Homebrew, уже установленный на этом Mac; встроенный `/usr/bin/ldapsearch` не прошёл проверку подключения с тестовым CA.

```bash
LDAPTLS_CACERT="$PWD/secrets/tls/ca.crt" LDAPTLS_REQCERT=demand \
  "$(brew --prefix openldap)/bin/ldapsearch" -x -H ldaps://127.0.0.1:6360 \
  -D 'uid=svc-meetspace,ou=people,dc=meetspace,dc=test' -W \
  -b 'ou=people,dc=meetspace,dc=test' \
  '(&(objectClass=person)(memberOf=cn=meetspace-users,ou=groups,dc=meetspace,dc=test))' \
  uid mail cn entryUUID memberOf
```

Проверка возвращает `alice` и `bob`. На macOS у `ldapsearch` используйте `127.0.0.1`: при `localhost` клиент может подставить имя Mac, которого нет в сертификате.

## Подключение MeetSpace

Для backend, запущенного непосредственно на Mac, адрес каталога — `ldaps://127.0.0.1:6360`. Для контейнеров MeetSpace используется `ldaps://ldap:6360` в общей сети `meetspace-ldap-test`. Сначала запустите стенд через `setup.py`, чтобы эта сеть существовала.

Из корня проекта запустите MeetSpace с готовым override. Он подключает к сети LDAP **и API, и product-worker**, который выполняет фоновую синхронизацию:

```bash
docker compose -f docker-compose.yml -f docker-compose.ldap-dev.yml \
  up -d --build api product-worker
```

Если используется локальная административная HTTPS-панель, сохраните её override. Текущий локальный стенд использует все три файла:

```bash
docker compose -f docker-compose.yml -f docker-compose.local-admin.yml \
  -f docker-compose.ldap-dev.yml up -d --build api product-worker https
```

При последующих обновлениях и пересоздании контейнеров используйте тот же набор Compose-файлов. Ручной `docker network connect` и монтирование CA в контейнер API не нужны. В `docker-compose.ldap-dev.yml` нет секретов; рабочее развёртывание не зависит от тестовой сети, если этот override не подключён.

Перед настройкой LDAP включите и подготовьте административную панель по [инструкции](../../docs/ADMIN_DEPLOYMENT_RU.md). API и product-worker должны использовать одинаковый `PROVIDER_TOKEN_ENCRYPTION_KEY`: он нужен для хранения и чтения зашифрованного bind-пароля.

В административной панели откройте **Настройки → LDAP / Active Directory** и заполните:

| Поле | Значение для этого стенда |
| --- | --- |
| Тип каталога | LDAP (OpenLDAP / LLDAP) |
| Адрес сервера | `ldaps://ldap:6360` для Docker или `ldaps://127.0.0.1:6360` для backend на Mac |
| Базовый DN | `dc=meetspace,dc=test` |
| DN пользователей | `ou=people,dc=meetspace,dc=test` |
| Bind DN | `uid=svc-meetspace,ou=people,dc=meetspace,dc=test` |
| Bind-пароль | Пароль `svc-meetspace` из локального `secrets/credentials.json` |
| DN группы доступа | `cn=meetspace-users,ou=groups,dc=meetspace,dc=test` |
| Фильтр пользователей | `(objectClass=person)` |
| Атрибуты логина / ID / email / имени / членства | `uid` / `entryUUID` / `mail` / `cn` / `memberOf` |
| Вложенные группы | Выключены |
| CA сертификат PEM | Содержимое `tools/ldap-dev/secrets/tls/ca.crt` целиком, включая строки BEGIN/END |

CA вставляется **в поле административной панели**, а не задаётся путём к файлу. Сертификат сервера покрывает имена `localhost`, `ldap`, `host.docker.internal` и адрес `127.0.0.1`; проверка цепочки доверия и имени сервера остаётся включённой. Для интеграции с этим стендом используйте LDAPS: диагностический порт `3890` без TLS не подходит для входа MeetSpace.

Сохраните параметры и нажмите **«Проверить подключение»**. Проверка читает первую страницу каталога, не изменяя аккаунты. Затем включите LDAP, сохраните настройки и нажмите **«Синхронизировать сейчас»**. Дождитесь успешного завершения в статусе синхронизации; после этого проверьте **«Корпоративный вход»** с логинами `alice` и `bob` и их паролями. `outsider` не должен входить, пока не добавлен в группу доступа. Для повторных проверок учитывайте изменения членства, сохранённые в LLDAP.

Дальнейшие изменения пользователей импортируются по заданному интервалу или по кнопке ручного запуска. Ограничения синхронизации, поведение сессий и профиль Active Directory описаны в [руководстве LDAP](../../docs/LDAP.md).

## Обслуживание

Команды выполняются из `tools/ldap-dev`:

```bash
docker compose -f compose.yml up -d --wait
docker compose -f compose.yml restart ldap
docker compose -f compose.yml logs --tail=100 -f ldap
docker compose -f compose.yml down
```

`down` останавливает стенд, сохраняя именованный том с данными. Не добавляйте `-v`, если хотите сохранить учётные записи и изменения.

В этом стенде подтверждены атрибуты `uid`, `mail`, `cn`, `entryUUID` и `memberOf`. LLDAP позволяет проверить bind, поиск и членство в группе, но не воспроизводит AD атрибуты и поведение `objectGUID`, `userAccountControl` и вложенных групп. Совместимость с Windows AD проверяется отдельно.
