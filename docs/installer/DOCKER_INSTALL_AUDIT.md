# Docker image build and installation performance audit

Date: 2026-10-22. Audited source: `ac8b8826908a69c0d3de073d0c0f56a6f3dc4687`.

## Scope and evidence

The user's request identifies **Docker image compilation** as the slow stage.
The supplied audit template instead describes installing Docker Engine. These
are separate operations. This audit follows the actual request, records the
Engine installation path, and does not claim an unmeasured Engine bottleneck.
This initial audit was written before production code changes.

The real entrypoint is `install.sh`, which fetches a fixed GitHub revision and
runs `scripts/install/from-source.py`. The latter makes a retained clean checkout,
calls `scripts/install/wizard.py:docker_setup()`, creates a bounded BuildKit in
low-memory mode, then runs `scripts/release/build.sh` and `package.sh`. Deployment
uses a verified image archive and Compose `--no-build`.

## Baseline observations

The previous retained server build ran on Ubuntu 24.04, Linux amd64, approximately
4 GiB host RAM and a 30 GiB root disk. Its dedicated builder was restricted to
one CPU and 2048 MiB RAM, without extra swap; Go used `-p=1`, `GOMAXPROCS=1`,
`GOGC=20`, and `GOMEMLIMIT=768MiB`. The hostname, IP, credentials and application
data are deliberately omitted. Exact host CPU count, network characteristics,
mirror timings and Engine/Compose versions were not captured for that run.

| Operation | Observed baseline | Evidence / limitation |
| --- | --- | --- |
| Prometheus binary build RUN | 746.9 s | Retained `build-resume.log`, step 17; successful build |
| Prometheus final image export | 8.4 s | Same log, step 22 |
| Five application Go builds | Repeated overlapping dependency compilation | Dockerfile inspection; no trustworthy per-service baseline table retained |
| SeaweedFS build | Long compilation observed | Exact duration not retained here; no invented value |
| Whole fresh image build | Not measured continuously | Earlier run was cancelled for disk pressure, then resumed separately |
| Docker Engine installation | Not measured | Engine was already running during the observed build |
| APT locks / DNS / repository / package download / dpkg / systemd startup | Not measured | No fresh disposable Ubuntu/Debian installation benchmark was run |

The compilation delay is independently visible in the active Go compiler and
in the Dockerfile RUN duration. A read-only attempt to retrieve more server
diagnostics during this audit timed out. No package-management experiment was
run on that host, which the user is reserving for their own installation.

## Confirmed code findings

- `dockers/{api,media-worker,worker,product-worker,live-worker}/Dockerfile` share
  a `go mod download` layer, but none mounts a shared Go compiler cache. Each
  binary build starts without the previous service's compiled dependency cache.
- Those build stages use `COPY . .`; changes to frontend, documentation and
  deployment files unnecessarily invalidate the Go source-copy layer.
- `dockers/observability/Dockerfile.release` compiles both upstream Prometheus
  binaries, but has no persistent module/compiler cache for interrupted RUNs.
- `dockers/object-storage/Dockerfile` already uses module and compiler cache
  mounts. Its pinned rebuild patches vulnerable upstream binaries; replacing it
  with an unverified upstream image would weaken the security policy.
- `from-source.py:build_and_bundle()` generates a new version, registry prefix
  and build directory on every incomplete retry.
- `build.sh` records `build.json` only after all images complete and refuses any
  existing release tag. A late failure therefore repeats completed image work.
- The low-memory mode deliberately trades speed for bounded RAM and CPU. Its
  hard limits must remain in force; removing them is not an optimization fix.
- `package.sh` scans all 13 deployed components. HIGH/CRITICAL findings block
  the package. This is a separate, required gate and must not be skipped.

## Docker Engine path (not the measured bottleneck)

`install.sh` may run `apt-get update` and install prerequisites. `wizard.py:apt()`
refreshes indexes on each call. `docker_setup()` supports automatic installation
on Ubuntu/Debian, checks distro/codename and architecture, uses Docker's official
HTTPS APT repository and a fingerprint-checked signed-by keyring, installs
Engine/CLI/containerd/Buildx/Compose, enables the service, and verifies the local
daemon plus Compose >= 2.24.4. Already installed Engine is not reinstalled.
There is no host `apt-get upgrade` or `dist-upgrade` in this path.

Secondary issues: missing Compose aborts instead of a targeted repair; repeated
index refreshes outside repository changes may be redundant; APT lock/network
waits are not bounded explicitly; partially created repository files are not
reused on retry. These are documented rather than mixed into the image build
optimization. Fresh Engine benchmarks require disposable supported Linux hosts.

## Implementation plan

1. Measure a scoped before/after Go image benchmark with an isolated local
   builder. Record architecture/resources and distinguish it from a fresh VPS.
2. Add shared module/compiler cache mounts to the application Go build stages
   and persistent caches to the Prometheus rebuild. Keep runtime contents,
   compiler arguments, upstream pins and verification unchanged.
3. Copy only Go build inputs into Go compilation stages.
4. Preserve the release identity across `--resume`; atomically checkpoint each
   completed image. Reuse requires matching source fingerprint, commit, platform,
   build options, immutable image ID and provenance labels. Unknown existing
   tags remain an error, never an overwrite.
5. Print meaningful per-image status and wall-clock durations, including failure
   and retry guidance, without fabricated completion percentages.
6. Run interruption/resume/provenance-mismatch checks and real Docker cache
   measurements. Document measured results and checks that were not run in
   `DOCKER_INSTALL_BEFORE_AFTER.md`.

Do not change the user's retained installer package, start their installation,
alter server containers/volumes, bypass scans, or publish changes as part of this
local optimization request.
