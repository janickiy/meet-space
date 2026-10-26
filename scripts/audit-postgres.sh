#!/usr/bin/env bash
# Run authorization regressions against a newly created, disposable database.
# This script never reads .env, mounts application data or uses real credentials.
set -euo pipefail
repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
container_id=
cleanup() {
  if [[ -n "$container_id" ]]; then
    docker rm -f "$container_id" >/dev/null
  fi
}
trap cleanup EXIT
container_id="$(docker run -d --label meetspace.audit=postgres-regression \
  --memory=512m --cpus=1 --tmpfs /var/lib/postgresql/data:rw,size=256m \
  --publish 127.0.0.1::5432 \
  --env POSTGRES_USER=meetspace_audit --env POSTGRES_PASSWORD=synthetic-audit-only \
  --env POSTGRES_DB=meetspace_audit postgres:16-alpine)"
ready=false
for ((attempt=0; attempt<60; attempt++)); do
  if docker exec "$container_id" pg_isready -U meetspace_audit -d meetspace_audit >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 1
done
[[ "$ready" == true ]] || { echo 'Disposable PostgreSQL did not become ready.' >&2; exit 1; }
cd "$repo_dir"
if [[ "${AUDIT_GO_IN_DOCKER:-false}" == true ]]; then
  # Sharing only this new database's network namespace keeps localhost literal.
  docker run --rm --network "container:$container_id" --memory=2g --cpus=2 \
    --mount "type=bind,source=$repo_dir,target=/src,readonly" --workdir /src \
    --env GOTOOLCHAIN=local --env GOFLAGS=-mod=readonly \
    --env 'MEETSPACE_AUDIT_POSTGRES_DSN=postgres://meetspace_audit:synthetic-audit-only@127.0.0.1:5432/meetspace_audit?sslmode=disable' \
    golang:1.26.10-bookworm \
    go test -race -count=1 -v ./internal/infrastructure/postgres -run TestAudit
else
  address="$(docker port "$container_id" 5432/tcp)"
  [[ "$address" =~ ^127\.0\.0\.1:[0-9]+$ ]] || { echo 'Expected a loopback-only port.' >&2; exit 1; }
  MEETSPACE_AUDIT_POSTGRES_DSN="postgres://meetspace_audit:synthetic-audit-only@$address/meetspace_audit?sslmode=disable" \
    go test -race -count=1 -v ./internal/infrastructure/postgres -run TestAudit
fi
