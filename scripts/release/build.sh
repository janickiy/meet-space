#!/usr/bin/env bash
# Собирает приложение один раз; публикация и развёртывание используют эти же образы.
set -euo pipefail
source "$(dirname "$0")/common.sh"
version= registry= commit= output= allow_dirty=false layout=classic platform= provenance= builder= low_memory=false
while (($#)); do
  case "$1" in
    --version) version="$2"; shift 2;; --registry) registry="$2"; shift 2;;
    --commit) commit="$2"; shift 2;; --output) output="$2"; shift 2;;
    --layout) layout="$2"; shift 2;; --platform) platform="$2"; shift 2;;
    --snapshot-provenance) provenance="$2"; shift 2;;
    --builder) builder="$2"; shift 2;;
    --low-memory) low_memory=true; shift;;
    --allow-dirty-local) allow_dirty=true; shift;; *) fail "Unknown build argument: $1";;
  esac
done
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$ ]] || fail "--version must be a SemVer release tag"
[[ "$registry" =~ ^[a-zA-Z0-9][a-zA-Z0-9./:_-]+$ && "$registry" != */ ]] || fail "--registry must name the image repository prefix"
[[ "$commit" =~ ^[a-f0-9]{40}$ ]] || fail "--commit must be the full source SHA"
[[ "$output" == /* ]] || fail "--output must be an absolute artifact directory"
[[ "$layout" == classic || "$layout" == hardened-shared-host ]] || fail "Unsupported release layout"
[[ -z "$platform" || "$platform" == linux/amd64 || "$platform" == linux/arm64 ]] || fail "Unsupported target platform"
[[ -z "$builder" || "$builder" =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]+$ ]] || fail "Invalid builder name"
[[ "$low_memory" == false || -n "$builder" ]] || fail "Low-memory builds require an explicitly constrained builder"
dirty=false
if git -C "$RELEASE_ROOT" rev-parse --git-dir >/dev/null 2>&1; then
  [[ "$(git -C "$RELEASE_ROOT" rev-parse HEAD)" == "$commit" ]] || fail "Commit does not match checked out source"
  [[ -z "$(git -C "$RELEASE_ROOT" status --porcelain)" ]] || dirty=true
  [[ "$dirty" == false || "$allow_dirty" == true ]] || fail "Release source must be clean; only local rehearsal may allow dirty source"
elif [[ "${CI_COMMIT_SHA:-}" != "$commit" ]]; then fail "Source without .git must be bound to CI_COMMIT_SHA"; fi
mkdir -p "$output"
[[ ! -e "$output/build.json" ]] || fail "Build metadata already exists; choose a new directory"
time="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
command -v rg >/dev/null || fail "ripgrep is required to fingerprint source files"
# Контрольная сумма включает имена и содержимое исходников, в том числе незакоммиченные файлы локальной репетиции.
source_digest="$(cd "$RELEASE_ROOT"; rg --files --hidden --no-require-git -g '!.git/**' -g '!release-artifacts/**' -g '!.env' -g '!.env.local' -g '!.env.production' -g '!.env.staging' -g '!secrets/**' -g '!dockers/https/certs/**' | LC_ALL=C sort | while IFS= read -r file; do printf '%s\n' "$file"; file_sha256 "$file"; done | file_sha256 /dev/stdin)"
metadata="$(jq -n --arg version "$version" --arg commit "$commit" --arg builtAt "$time" --arg registry "$registry" --arg sourceSHA256 "$source_digest" --arg layout "$layout" --arg platform "$platform" --argjson dirty "$dirty" '{schemaVersion:1,version:$version,commit:$commit,builtAt:$builtAt,registry:$registry,sourceDirty:$dirty,sourceSHA256:$sourceSHA256,deploymentLayout:$layout,platform:$platform,images:{}}')"
if [[ -n "$provenance" ]]; then
  [[ "$provenance" == /* && -f "$provenance" ]] || fail "Absolute snapshot provenance file required"
  jq -e --arg commit "$commit" '.schemaVersion == 1 and .mode == "working-tree-snapshot" and .snapshotCommit == $commit and .originalRepositoryModified == false' "$provenance" >/dev/null || fail "Source snapshot provenance mismatch"
  metadata="$(jq --slurpfile provenance "$provenance" '.sourceSnapshot=$provenance[0]' <<<"$metadata")"
fi
if [[ "$layout" == hardened-shared-host ]]; then
  metadata="$(jq '.objectStorageProvider="seaweedfs" | .omittedComponents=[{name:"grafana",reason:"upstream bundled plugins have unresolved HIGH findings; Prometheus remains enabled"}]' <<<"$metadata")"
fi
build_services=(api media-worker worker product-worker live-worker frontend minio)
if [[ "$layout" == hardened-shared-host ]]; then build_services+=(coturn proxy postgres redis prometheus); fi
build_command=(docker build)
if [[ -n "$builder" ]]; then build_command=(docker buildx build --builder "$builder" --load); fi
if [[ -n "$platform" ]]; then build_command+=(--platform "$platform"); fi
if [[ "$low_memory" == true ]]; then
  build_command+=(--build-arg GOMAXPROCS=1 --build-arg GOFLAGS=-p=1
    --build-arg GOGC=20 --build-arg GOMEMLIMIT=768MiB
    --build-arg NODE_OPTIONS=--max-old-space-size=768)
fi
metadata="$(jq --argjson lowMemory "$low_memory" '.lowMemoryBuild=$lowMemory' <<<"$metadata")"
for service in "${build_services[@]}"; do
  context="$RELEASE_ROOT"; dockerfile="$RELEASE_ROOT/dockers/$service/Dockerfile"
  if [[ "$service" == frontend ]]; then context="$RELEASE_ROOT/frontend"; dockerfile="$context/Dockerfile"; fi
  if [[ "$layout" == hardened-shared-host ]]; then
    case "$service" in
      minio) dockerfile="$RELEASE_ROOT/dockers/object-storage/Dockerfile";;
      coturn) dockerfile="$RELEASE_ROOT/dockers/turn/Dockerfile";;
      proxy) dockerfile="$RELEASE_ROOT/dockers/production/Dockerfile";;
      prometheus) dockerfile="$RELEASE_ROOT/dockers/observability/Dockerfile.release";;
      postgres|redis) dockerfile="$RELEASE_ROOT/dockers/$service/Dockerfile.release";;
    esac
  fi
  ref="$registry/$service:$version"
  docker image inspect "$ref" >/dev/null 2>&1 && fail "Refusing to replace existing release tag: $ref"
  # Upstream Prometheus is identical across application releases; keep its binary
  # reproducible and cacheable while recording this release time in OCI labels.
  image_build_time="$time"
  if [[ "$service" == prometheus ]]; then image_build_time=unknown; fi
  "${build_command[@]}" --target runtime --file "$dockerfile" --tag "$ref" \
    --build-arg "BUILD_VERSION=$version" --build-arg "BUILD_COMMIT=$commit" --build-arg "BUILD_TIME=$image_build_time" \
    --build-arg "CLIENT_TELEMETRY_ENABLED=${CLIENT_TELEMETRY_ENABLED:-false}" \
    --label "org.opencontainers.image.version=$version" --label "org.opencontainers.image.revision=$commit" \
    --label "org.opencontainers.image.created=$time" --label "io.go-recorder.source-sha256=$source_digest" "$context"
  metadata="$(jq --arg service "$service" --arg ref "$ref" '.images[$service]=$ref' <<<"$metadata")"
done
printf '%s\n' "$metadata" > "$output/build.json"
printf 'Built %s. Package these images without rebuilding.\n' "$version"
