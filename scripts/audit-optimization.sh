#!/usr/bin/env bash
# MeetSpace — изолированные интеграционные проверки оптимизации.
# Copyright 2026 Alexander Yanitskiy. Licensed under Apache-2.0.
# Не читает .env, не использует данные установки и не удаляет чужие контейнеры.
# Требуются Docker, Go из go.mod и локальные образы, указанные ниже.
# Использование: bash scripts/audit-optimization.sh [пакеты или параметры go test]
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
postgres_id=
redis_id=
rabbit_id=

cleanup() {
  local result=$?
  local id
  trap - EXIT INT TERM
  for id in "$rabbit_id" "$redis_id" "$postgres_id"; do
    if [[ -n "$id" ]] && ! docker rm -f "$id" >/dev/null; then
      printf 'Не удалось удалить тестовый контейнер %s\n' "$id" >&2
      if [[ "$result" == 0 ]]; then result=1; fi
    fi
  done
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

command -v docker >/dev/null || { echo 'Требуется Docker.' >&2; exit 1; }
command -v go >/dev/null || { echo 'Требуется Go.' >&2; exit 1; }
docker info >/dev/null
# Явный запрет загрузки образов не допускает незаметного изменения стенда.
for image in postgres:16-alpine redis:7-alpine rabbitmq:4.2.9-management; do
  docker image inspect "$image" >/dev/null || {
    printf 'Нет локального образа %s. Сначала загрузите его командой docker pull.\n' "$image" >&2
    exit 1
  }
done

# create и start разделены: ID сохраняется до запуска и попадёт в cleanup
# даже при ошибке запуска. Все объявленные образами VOLUME заменены tmpfs.
postgres_id="$(docker create --pull=never --label meetspace.audit=optimization \
  --memory=768m --cpus=1 --pids-limit=256 --restart=no \
  --tmpfs /var/lib/postgresql/data:rw,nosuid,size=384m \
  --publish 127.0.0.1::5432 \
  --env POSTGRES_USER=meetspace_audit --env POSTGRES_PASSWORD=synthetic-audit-only \
  --env POSTGRES_DB=meetspace_audit \
  --health-cmd='pg_isready -h 127.0.0.1 -U meetspace_audit -d meetspace_audit -t 2' \
  --health-interval=1s --health-timeout=3s --health-retries=60 \
  postgres:16-alpine)"
redis_id="$(docker create --pull=never --label meetspace.audit=optimization \
  --memory=256m --cpus=1 --pids-limit=128 --restart=no \
  --tmpfs /data:rw,nosuid,size=32m --publish 127.0.0.1::6379 \
  --health-cmd='redis-cli ping' --health-interval=1s --health-timeout=3s --health-retries=60 \
  redis:7-alpine redis-server --save '' --appendonly no --maxmemory 128mb --maxmemory-policy noeviction)"
rabbit_id="$(docker create --pull=never --label meetspace.audit=optimization \
  --memory=768m --cpus=1 --pids-limit=256 --restart=no \
  --tmpfs /var/lib/rabbitmq:rw,nosuid,size=256m --publish 127.0.0.1::5672 \
  --env RABBITMQ_DEFAULT_USER=meetspace_audit --env RABBITMQ_DEFAULT_PASS=synthetic-audit-only \
  --health-cmd='rabbitmq-diagnostics -q ping' --health-interval=2s --health-timeout=5s --health-retries=30 \
  rabbitmq:4.2.9-management)"
docker start "$postgres_id" "$redis_id" "$rabbit_id" >/dev/null

printf 'Ожидание тестовых PostgreSQL, Redis и RabbitMQ (до 90 секунд)...\n'
ready=false
for ((attempt=0; attempt<90; attempt++)); do
  ready=true
  for id in "$postgres_id" "$redis_id" "$rabbit_id"; do
    state="$(docker inspect --format '{{.State.Status}} {{.State.Health.Status}}' "$id")"
    case "$state" in
      'running healthy') ;;
      running\ *) ready=false ;;
      *) printf 'Тестовый контейнер остановился: %s\n' "$state" >&2; exit 1 ;;
    esac
  done
  if [[ "$ready" == true ]]; then break; fi
  sleep 1
done
[[ "$ready" == true ]] || { echo 'Тестовые сервисы не готовы за 90 секунд.' >&2; exit 1; }

loopback_address() {
  local address
  address="$(docker port "$1" "$2")"
  [[ "$address" =~ ^127\.0\.0\.1:[0-9]+$ ]] || {
    echo 'Ожидался единственный порт, доступный только через loopback.' >&2
    return 1
  }
  printf '%s' "$address"
}
postgres_addr="$(loopback_address "$postgres_id" 5432/tcp)"
redis_addr="$(loopback_address "$redis_id" 6379/tcp)"
rabbit_addr="$(loopback_address "$rabbit_id" 5672/tcp)"
export MEETSPACE_AUDIT_POSTGRES_DSN="postgres://meetspace_audit:synthetic-audit-only@$postgres_addr/meetspace_audit?sslmode=disable"
export MEETSPACE_TEST_REDIS_ADDR="$redis_addr"
export MEETSPACE_TEST_RABBITMQ_URL="amqp://meetspace_audit:synthetic-audit-only@$rabbit_addr/%2F"

cd "$repo_dir"
if [[ "$#" == 0 ]]; then set -- ./...; fi
printf 'Запуск Go race/integration tests на временном стенде...\n'
go test -mod=readonly -race -count=1 "$@"
