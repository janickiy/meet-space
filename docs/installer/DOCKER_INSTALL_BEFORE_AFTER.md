# Docker image build optimization: measurements and verification

> Исторический отчёт: отражает состояние проекта на дату проверки. После удаления распознавания речи и переименования сервиса записи его перечень компонентов не является описанием текущей версии. Актуальные сведения: [архитектура](../ARCHITECTURE.md).

Date: 2026-10-22. Baseline source: `ac8b8826908a69c0d3de073d0c0f56a6f3dc4687`.
See [the pre-change audit](DOCKER_INSTALL_AUDIT.md) for scope and the distinction
between installing Docker Engine and compiling application images.

## Environment and method

The real before/after benchmark used local Docker Desktop, native Linux ARM64
containers, Docker Engine/CLI 29.4.2 and Buildx v0.33.0-desktop.1. The Docker VM
exposed 10 CPUs and 16,748,032,000 bytes RAM. Both independent builders used the
installer's pinned BuildKit image, one CPU (`period=100000`, `quota=100000`),
2048 MiB RAM and an equal RAM+swap limit. The host had approximately 346 GiB
free disk at preflight. No application server installation was modified.

Each builder started with an independent empty cache. API was built first,
then worker, using the original Dockerfiles for the baseline and optimized
Dockerfiles afterward. Go settings were identical: `GOMAXPROCS=1`, `GOFLAGS=-p=1`,
`GOGC=20`, `GOMEMLIMIT=768MiB`, `CGO_ENABLED=0`, target `linux/arm64`. Application
Go sources, runtime dependencies and binary build arguments were unchanged.

Wall time was measured with Python's monotonic `time.perf_counter()`. Individual
RUN durations came from BuildKit plain logs. Network download times varied;
they are not attributed to the compiler-cache improvement. APK runtime work
can overlap Go compilation. Additional Dockerfile smoke builds started only
after the timed worker compiler RUN had completed; their network activity can
affect full-image wall times, so compiler RUN durations are the primary result.

## Measured results

| Operation | Before | After | Interpretation |
| --- | ---: | ---: | --- |
| API module download RUN | 58.9 s | 48.6 s | Network-dependent cold downloads; not a claimed cache speedup |
| API compilation RUN (cold) | 105.3 s | 109.0 s | First compilation still required; cache population adds work |
| API complete image wall time | 186.959 s | 186.362 s | Essentially unchanged first image |
| Worker compilation RUN after API | 106.0 s | 1.1 s | Shared compiler cache removes repeated dependency compilation |
| Worker APK/runtime dependency RUN | 40.1 s | 65.2 s | Network variation; no runtime package optimization claimed |
| Worker complete image wall time | 111.015 s | 69.871 s | Remaining time dominated by FFmpeg/runtime installation |
| API + worker complete wall time | 297.974 s | 256.233 s | Scoped two-image sample, not a full installer benchmark |
| Unchanged cached API rerun | Not measured | 1.076 s | All build layers reused |
| Unchanged cached worker rerun | Not measured | 1.210 s | All build layers reused |
| Unchanged cached API + worker rerun | Not measured | 2.286 s | Uses the already populated after builder |
| Fresh Prometheus compilation | Previous server: 746.9 s | Not remeasured | Cache improves retries; first compilation is still necessary |
| Whole fresh MeetSpace installation | Not measured | Not measured | No claim of a total installation speedup or time guarantee |

The worker compiler RUN was about **96 times faster** (approximately 99% less
time) in this sample. This does not mean that first installation, image download,
FFmpeg installation, or a complete production deployment is 96 times faster.

## Changes

- The five application Go Dockerfiles share locked module and compiler cache
  mounts scoped to Go toolchain family and target architecture.
- Go compilation stages copy `cmd`, `internal` and `tools`, excluding unrelated
  documentation/frontend inputs. Runtime assets and debug stages are unchanged.
- Prometheus retains module/compiler caches across attempts. Upstream hashes,
  dependency patching, build flags and flattened runtime are unchanged.
- The source installer keeps the same build/version/registry identity on retry.
- `build.sh --resume` saves `build-progress.json` atomically after each image.
  Every reused image must match the checkpoint's immutable ID, source fingerprint,
  commit, version, platform, creation label and build options. Unrecorded existing
  release tags are never overwritten.
- Each image reports its start, elapsed time and saved progress. Failure reports
  name the current image and exit status with retry guidance.
- Packaging, SBOM generation, all 13 security scans, HIGH/CRITICAL gates, pinned
  upstream inputs and package integrity verification remain required and unchanged.

## Verification performed

Syntax and whitespace checks:

```bash
bash -n scripts/release/build.sh install.sh
git diff --check
```

The modified Python installer was parsed with `ast.parse`, without generating
bytecode in the checkout. Temporary verification programs are under ignored
`tmp/docker-build-optimization/`, not shipped as project tests:

```bash
python3 -I tmp/docker-build-optimization/check-resume.py
python3 -I tmp/docker-build-optimization/check-source-resume.py
```

All **17 isolated checks passed**. They exercise an actual copied release script
in temporary Git fixtures with a fake Docker CLI, not a production daemon:

- Failure at image 3 preserves the first two checkpoints; resume builds only
  the remaining 10 images and a completed retry performs zero builds.
- Source fingerprint, commit, version/creation/source labels, image ID,
  architecture, platform, memory mode and telemetry mismatches are rejected.
- Missing checkpoint images and unrecorded pre-existing tags fail safely.
- The source installer preserves its release identity and passes `--resume`;
  tampered state is rejected before external commands.

Real Docker verification built all five optimized application images successfully:

| Additional image with shared cache | Complete wall time | Exit code |
| --- | ---: | ---: |
| media-worker | 3.493 s | 0 |
| product-worker | 60.074 s | 0 |
| live-worker | 3.720 s | 0 |

The product-worker compiler RUN took 1.5 s; its runtime package installation
took 56.1 s. API and worker main binaries extracted from the before/after images
had identical SHA-256 values, respectively:

```text
API    816b4276aa309af103517120e1a299cf8a61fb714502b1c236330742cd046dbd
worker 68affe42d1da287b260c25cc2bfaf2519730e256c0dc04797472aaebd2f185eb
```

Prometheus compile-stage configuration also passed BuildKit's
`--call=check --target build` without warnings. This is not a full compile or a
whole-installer architecture compatibility test.

Baseline Dockerfiles and raw timing/log artifacts are retained in the same
ignored temporary directory. The benchmark command is:

```bash
for phase in before after; do
  docker buildx create --name "meetspace-perf-$phase-20261022" \
    --driver docker-container \
    --driver-opt 'image=moby/buildkit@sha256:cec9f139f45e93c5c69c60f8b07cfad9f43f4ef6b6a6cd917527fea5ff2e3dea,memory=2048m,memory-swap=2048m,cpu-period=100000,cpu-quota=100000,restart-policy=no,env.GOMAXPROCS=1' \
    --bootstrap
done
python3 -I tmp/docker-build-optimization/benchmark.py before
python3 -I tmp/docker-build-optimization/benchmark.py after
python3 -I tmp/docker-build-optimization/benchmark.py extras
python3 -I tmp/docker-build-optimization/benchmark.py warm
```

After verification, only these two temporary benchmark builders, their caches
and the nine benchmark image tags were removed. The timing logs, baseline
Dockerfile copies and verification scripts were retained. Recreating the test
builders is required before rerunning the benchmark; deleted images are
reproducible from those inputs. The temporary macOS sleep assertion was released.

Changed files: `README.md`, `docs/INSTALLATION.md`, the two audit documents in
`docs/installer/`, `scripts/release/build.sh`, `scripts/install/from-source.py`,
the five application Go Dockerfiles, and
`dockers/observability/Dockerfile.release`.

## Checks not run and limitations

- No fresh Ubuntu 22.04/24.04 or Debian VM installation of Engine was timed.
  OS detection, daemon/Compose detection, APT locks, DNS/repository probes,
  `apt-get update`, GPG key retrieval/repository setup, package downloads,
  dpkg configuration, systemd startup and daemon readiness therefore have no
  before/after installation timings. Engine/APT code was not changed.
- A full fresh MeetSpace deployment and production end-to-end smoke test were
  not rerun. The user's retained ready-to-install server package is unchanged.
- Prometheus was not fully recompiled after this cache-only change. Local
  BuildKit lint emitted an `InvalidBaseImagePlatform` warning for the existing
  amd64-pinned Prometheus runtime on the ARM64 build host; it also did so with
  an explicit amd64 lint target. This is not treated as ARM64 installer validation.
  Native amd64 infrastructure verification and ARM64 infrastructure portability
  remain separate checks. The relevant upstream pins were not changed here.
- Cold Prometheus and SeaweedFS compilation remains expensive. Avoiding that
  work on a new server requires a separately designed, verified prebuilt-image
  release path (for example CI), not disabling security scans or silently using
  unverified upstream images.
- A crash after an image is loaded but before its checkpoint is saved can leave
  an unrecorded tag. Resume refuses to overwrite it; restore validated progress
  or use a fresh build directory after investigation.
- Deleting the private workspace/cache can require recompilation. No shared
  server caches, Docker volumes, application data or services were deleted.
