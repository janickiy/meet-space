# Переменные GitLab CI/CD

Источники: [`.gitlab-ci.yml`](../.gitlab-ci.yml),
[`scripts/ci-deploy.sh`](../scripts/ci-deploy.sh).

Pipeline запускается для веток и тегов. `deploy-dev` автоматически выполняется
после успешных предыдущих стадий в ветке **`main`**. Это прежний development
workflow: rsync исходников, сборка на удалённом хосте и базовый
`docker-compose.yml`. Он не выполняет immutable production promotion и не
подходит для обновления standalone-установки `install.sh`.

## Загрузка исходников для всех jobs

`GIT_STRATEGY=none`: job получает архив конкретного `CI_COMMIT_SHA` через GitLab
API во временный каталог. Поэтому следующая переменная нужна не только deploy:

| Переменная | Тип / хранение | Назначение |
| --- | --- | --- |
| `CI_SOURCE_ARCHIVE_TOKEN_B64` | Variable, masked/hidden | Base64 токена с доступом `read_api` к архиву этого проекта |

Base64 — кодирование, не шифрование. Отключите подстановку ссылок на другие
переменные для секретов. Ограничьте scopes, срок токенов и доступ к runner.
`Protected` переменные доступны только подходящим защищённым веткам/тегам:
согласуйте это с правилами job. Не выводите секреты и не включайте `set -x`.

## Проверки и инструменты

| Job | Текущее поведение |
| --- | --- |
| `validate` | Go 1.26.9: gofmt, vet, staticcheck v0.7.0; `bash -n` для deploy/release scripts |
| `frontend-check` | Node 24: `npm ci`, lint, typecheck, build |
| `security-go` | govulncheck v1.8.0 |
| `security-frontend` | `npm audit --audit-level=high` |
| `go-build` | Бинарники api, worker, media-worker, product-worker, live-worker, main с version/commit/time |
| `release-package` | Теги `vX.Y.Z` / prerelease: сборка, scan, SBOM и registry package |
| `deploy-dev` | Только `main`: legacy rsync/build/up |

Ручные продуктовые проверки дополняют CI и описаны в
[чеклисте](operations/RELEASE_CHECKLIST.md).

## Пакет выпуска по тегу

| Переменная | Назначение |
| --- | --- |
| `TRIVY_IMAGE` | Проверенный scanner, закреплённый `@sha256:…`; обязателен |
| `CI_REGISTRY`, `CI_REGISTRY_IMAGE`, `CI_REGISTRY_USER`, `CI_REGISTRY_PASSWORD` | Доступные job registry параметры; пароль передаётся `docker login --password-stdin` |
| `RELEASE_INFRASTRUCTURE_MANIFEST` | Защищённая File variable с предыдущим проверенным `release.json` для обычного обновления |
| `RELEASE_BOOTSTRAP` | `true` только для первой установки без предыдущей инфраструктуры |
| `CLIENT_TELEMETRY_ENABLED` | Необязательный build-флаг frontend; по умолчанию выключен |

`RELEASE_INFRASTRUCTURE_MANIFEST` и `RELEASE_BOOTSTRAP=true` взаимоисключающие.
Без выбора режима package job останавливается. Нужен runner с Docker-in-Docker,
TLS к daemon и разрешениями для сборки. Текущий CI вызывает `build.sh` без
`--layout`, то есть classic. Source installer отдельно строит hardened archive;
эти процессы нельзя считать одинаковыми. Подробности формата, scan и promotion —
[RELEASE_PROCESS.md](RELEASE_PROCESS.md).

## Legacy development deploy

Настраивайте только для выделенного development-хоста с подготовленными `.env`,
сертификатами и внешней сетью gateway. Не направляйте его на каталог `/opt/meetspace`
установки мастером или на действующий production project.

| Переменная | Тип / хранение | Назначение |
| --- | --- | --- |
| `DEPLOY_HOST` | Variable | Проверенный IP/DNS development-хоста |
| `DEPLOY_USER` | Variable | Выделенный SSH-пользователь с доступом к Docker |
| `DEPLOY_PATH` | Variable | Абсолютный каталог development checkout на сервере |
| `DEPLOY_PORT` | Variable | SSH-порт, по умолчанию `22` |
| `DEPLOY_SSH_PRIVATE_KEY_B64` | Variable, masked/hidden | Предпочтительный отдельный SSH-ключ в base64 |
| `DEPLOY_SSH_PRIVATE_KEY` | File variable | Альтернатива base64 ключу; скрипт также принимает текст значения |
| `DEPLOY_PASSWORD` | Variable, masked/hidden | Парольная альтернатива, если ключ не настроен |
| `DEPLOY_ENV_FILE_B64` | Variable, masked/hidden | Необязательная полная development-конфигурация `.env` в base64 |
| `DEPLOY_ENV_FILE` | File variable | Альтернатива env в base64 |
| `BUILD_VERSION` | Variable | Безопасный ID сборки; при отсутствии берётся первые 12 символов `CI_COMMIT_SHA` |

Приоритет авторизации: ключ base64, затем ключ File/text, затем пароль. Приоритет
конфигурации: env base64, затем env File/text; иначе используется серверный `.env`.
При его отсутствии скрипт копирует `.env.example`, однако пустые обязательные
пароли/ключи не создают готовую рабочую конфигурацию. Подготовьте их заранее по
[README](../README.md). Основной медиатракт использует `MEDIA_NAT_IPS` и порт
50010; `WEBRTC_NAT_IPS` относится к legacy recorder ingest. Публичный origin и
`MINIO_PUBLIC_ENDPOINT` должны соответствовать реальному HTTPS входу.

Скрипт выполняет `rsync --delete`, сохраняя перечисленные в нём data/cert/env
каталоги. Новые нестандартные каталоги автоматически не защищаются. Затем он
собирает api/worker/media/product/live/frontend/minio и запускает базовый Compose
с `--remove-orphans`. Автоматического drain, backup, миграционного окна и rollback
здесь нет. `StrictHostKeyChecking=accept-new` принимает первый неизвестный ключ;
предварительно проверьте host key и подготовьте доверенный `known_hosts` runner.

Изменение пароля зависимости в `.env` не обновляет учётную запись существующего
PostgreSQL/RabbitMQ/S3 автоматически. Не удаляйте данные для смены пароля.
Production процедуры находятся в [DEPLOYMENT.md](DEPLOYMENT.md), а не в этой
legacy job.
