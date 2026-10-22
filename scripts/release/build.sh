#!/usr/bin/env bash
# Собирает приложение один раз; публикация и развёртывание используют эти же образы.
set -Eeuo pipefail
source "$(dirname "$0")/common.sh"
version= registry= commit= output= allow_dirty=false layout=classic platform= provenance= builder= low_memory=false resume=false
while (($#)); do
  case "$1" in
    --version) version="$2"; shift 2;; --registry) registry="$2"; shift 2;;
    --commit) commit="$2"; shift 2;; --output) output="$2"; shift 2;;
    --layout) layout="$2"; shift 2;; --platform) platform="$2"; shift 2;;
    --snapshot-provenance) provenance="$2"; shift 2;;
    --builder) builder="$2"; shift 2;;
    --low-memory) low_memory=true; shift;;
    --resume) resume=true; shift;;
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
[[ ! -L "$output/build.json" && ! -L "$output/build-progress.json" ]] || fail "Build metadata must not be a symbolic link"
if [[ "$resume" == false && ( -e "$output/build.json" || -e "$output/build-progress.json" ) ]]; then
  fail "Build metadata already exists; use --resume for the same build or choose a new directory"
fi
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
telemetry="${CLIENT_TELEMETRY_ENABLED:-false}"
[[ "$telemetry" == true || "$telemetry" == false ]] || fail "CLIENT_TELEMETRY_ENABLED must be true or false"
metadata="$(jq --argjson lowMemory "$low_memory" --arg telemetry "$telemetry" '
  .lowMemoryBuild=$lowMemory | .buildOptions={lowMemory:$lowMemory,clientTelemetryEnabled:$telemetry}
  | .imageIDs={} | .timingsSeconds={}' <<<"$metadata")"

# write_build_metadata atomically saves generated progress outside the source checkout.
# Arguments: destination basename within the caller-selected artifact directory.
write_build_metadata() {
  local destination="$1" pending
  pending="$(mktemp "$output/.build-progress.XXXXXXXX")"
  printf '%s\n' "$metadata" > "$pending"
  mv -f -- "$pending" "$output/$destination"
}

# verify_built_image binds a checkpoint to the actual image, source and build platform.
# Arguments: service name, exact release tag, and optional immutable checkpoint image ID.
verify_built_image() {
  local service="$1" ref="$2" expected_id="${3:-}" inspected
  inspected="$(docker image inspect "$ref")" || fail "Saved image is missing for $service; restore it or start a new build directory"
  jq -e --arg version "$version" --arg commit "$commit" --arg source "$source_digest" \
    --arg created "$time" --arg platform "$platform" --arg id "$expected_id" '
      length == 1 and (.[0] |
        (.Id | test("^sha256:[a-f0-9]{64}$")) and ($id == "" or .Id == $id)
        and ($platform == "" or (.Os + "/" + .Architecture) == $platform)
        and .Config.Labels["org.opencontainers.image.version"] == $version
        and .Config.Labels["org.opencontainers.image.revision"] == $commit
        and .Config.Labels["org.opencontainers.image.created"] == $created
        and .Config.Labels["io.go-recorder.source-sha256"] == $source)' \
    <<<"$inspected" >/dev/null || fail "Saved image identity/provenance mismatch for $service; refusing to reuse or overwrite it"
  jq -r '.[0].Id' <<<"$inspected"
}

checkpoint=
if [[ "$resume" == true ]]; then
  if [[ -f "$output/build.json" ]]; then checkpoint="$output/build.json";
  elif [[ -f "$output/build-progress.json" ]]; then checkpoint="$output/build-progress.json"; fi
fi
if [[ -n "$checkpoint" ]]; then
  # Compare every input that affects the build before accepting any prior image.
  jq -e --argjson expected "$metadata" --arg services "${build_services[*]}" '
    .schemaVersion == $expected.schemaVersion and .version == $expected.version
    and .commit == $expected.commit and .registry == $expected.registry
    and .sourceDirty == $expected.sourceDirty and .sourceSHA256 == $expected.sourceSHA256
    and .deploymentLayout == $expected.deploymentLayout and .platform == $expected.platform
    and .buildOptions == $expected.buildOptions and .lowMemoryBuild == $expected.lowMemoryBuild
    and .sourceSnapshot == $expected.sourceSnapshot
    and .objectStorageProvider == $expected.objectStorageProvider
    and .omittedComponents == $expected.omittedComponents
    and (.builtAt | type == "string" and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$"))
    and (.images | type == "object") and (.imageIDs | type == "object")
    and (.timingsSeconds | type == "object")
    and ((.images | keys) == (.imageIDs | keys))
    and ((.images | keys) == (.timingsSeconds | keys))
    and ((.images | keys) - ($services | split(" ")) | length == 0)
    and all(.imageIDs[]; type == "string" and test("^sha256:[a-f0-9]{64}$"))
    and all(.timingsSeconds[]; type == "number" and . >= 0)' "$checkpoint" >/dev/null \
    || fail "Build checkpoint inputs do not match; source, platform and options must remain unchanged on --resume"
  metadata="$(jq '.' "$checkpoint")"
  time="$(jq -r .builtAt <<<"$metadata")"
fi

current_service=preparation
# report_build_failure identifies the failed stage without logging environment secrets.
# Arguments: process exit status and a short static failure reason.
report_build_failure() {
  local status="$1" reason="$2"
  printf '\nBuild stopped at %s after %s s (%s, exit %s). Completed images are saved in %s/build-progress.json.\nResume with the same arguments and --resume.\n' \
    "$current_service" "$((SECONDS - ${started:-$SECONDS}))" "$reason" "$status" "$output" >&2
  exit "$status"
}
trap 'report_build_failure "$?" "command failed"' ERR
trap 'report_build_failure 130 "interrupted"' INT
trap 'report_build_failure 143 "terminated"' TERM
if [[ -z "$checkpoint" ]]; then write_build_metadata build-progress.json; fi
index=0
for service in "${build_services[@]}"; do
  index=$((index + 1))
  current_service="$service"
  started=$SECONDS
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
  saved_ref="$(jq -r --arg service "$service" '.images[$service] // ""' <<<"$metadata")"
  if [[ -n "$saved_ref" ]]; then
    [[ "$saved_ref" == "$ref" ]] || fail "Checkpoint release tag mismatch for $service"
    saved_id="$(jq -er --arg service "$service" '.imageIDs[$service]' <<<"$metadata")"
    verify_built_image "$service" "$ref" "$saved_id" >/dev/null
    printf '[%s/%s] %s: verified saved image; compilation skipped.\n' "$index" "${#build_services[@]}" "$service"
    continue
  fi
  [[ "$checkpoint" != "$output/build.json" ]] || fail "Completed build manifest is missing $service"
  docker image inspect "$ref" >/dev/null 2>&1 && fail "Refusing to replace existing release tag: $ref"
  # Upstream Prometheus is identical across application releases; keep its binary
  # reproducible and cacheable while recording this release time in OCI labels.
  image_build_time="$time"
  if [[ "$service" == prometheus ]]; then image_build_time=unknown; fi
  printf '[%s/%s] %s: building (started %s).\n' "$index" "${#build_services[@]}" "$service" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  started=$SECONDS
  "${build_command[@]}" --target runtime --file "$dockerfile" --tag "$ref" \
    --build-arg "BUILD_VERSION=$version" --build-arg "BUILD_COMMIT=$commit" --build-arg "BUILD_TIME=$image_build_time" \
    --build-arg "CLIENT_TELEMETRY_ENABLED=$telemetry" \
    --label "org.opencontainers.image.version=$version" --label "org.opencontainers.image.revision=$commit" \
    --label "org.opencontainers.image.created=$time" --label "io.go-recorder.source-sha256=$source_digest" "$context"
  elapsed=$((SECONDS - started))
  image_id="$(verify_built_image "$service" "$ref")"
  metadata="$(jq --arg service "$service" --arg ref "$ref" --arg id "$image_id" --argjson elapsed "$elapsed" \
    '.images[$service]=$ref | .imageIDs[$service]=$id | .timingsSeconds[$service]=$elapsed' <<<"$metadata")"
  write_build_metadata build-progress.json
  printf '[%s/%s] %s: complete in %s s; progress saved.\n' "$index" "${#build_services[@]}" "$service" "$elapsed"
done
write_build_metadata build.json
trap - ERR INT TERM
printf 'Built %s. Package these images without rebuilding.\n' "$version"
