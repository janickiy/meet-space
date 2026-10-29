# MeetSpace web test report

> Исторический отчёт: отражает состояние проекта на дату проверки. После удаления распознавания речи и переименования сервиса записи его перечень компонентов не является описанием текущей версии. Актуальные сведения: [архитектура](../../ARCHITECTURE.md).

Audit date: 2026-10-26. Baseline commit: `3ea9a4e83e82484c25402963b2e68a0642d986f6`.
This report records the original working-tree audit, not a published release. For the later integration with upstream `d896401` and its rerun checks, see [the publication follow-up](WEB_AUDIT_SUMMARY.md#repository-publication-integration-2026-10-26). Historical image IDs and browser metrics below do not describe that newer combined tree.

## Environment and boundaries

| Component | Actual environment |
| --- | --- |
| Host | macOS 27.0.1 (26A434), arm64, 18 logical CPUs, 48 GiB RAM |
| Go | Baseline 1.26.9; final 1.26.10, darwin/arm64; database runner also Linux/arm64 |
| Node | Supported 24.19.0; globally installed 23.1.0 was excluded from comparable measurements |
| Frontend | Locked React 19.3.0, TypeScript 7.0.2, Vite 8.3.5 |
| Browser | Chrome 156.0.8078.47, headless, 1440×1000 and 390×844 viewports |
| Docker | Client/server 29.9.2; locally built Linux/arm64 images |
| Database | PostgreSQL 16.15, disposable `postgres:16-alpine` container |
| Scanners | govulncheck v1.8.0; Staticcheck v0.7.0; Trivy 0.75.0 |

No production or remote staging instance was contacted. No production credentials, user data, `.env`, Docker volumes or existing databases were used. PostgreSQL used synthetic accounts, 512 MiB RAM, 1 CPU and a 256 MiB temporary filesystem; its port was bound only to loopback. The optional Go test container used 2 GiB RAM and 2 CPUs. Image scans were limited to 2 GiB and 2 CPUs. These are test-resource limits, not load-test results.

The initial checkout had no Go or frontend regression tests. Baseline `go test ./...` succeeded with `[no test files]`; that is compilation evidence, not initial behavioral coverage. Existing changes to `RealtimePanel.tsx` and `conference.css` were preserved and were not authored by this audit.

## Executed checks

| Suite | Result and coverage |
| --- | --- |
| `go mod verify` | PASS; downloaded module checksums verified |
| `go vet ./...` | PASS, baseline and final |
| Staticcheck v0.7.0 | PASS after correcting an identical-expression error in a newly added test |
| `go test -race -count=1 ./...` | PASS; 82 root packages, with tests now in nine packages; PostgreSQL tests skip without their explicit synthetic DSN |
| Isolated PostgreSQL integration | PASS separately in host-Go and container-Go modes; real 35 migrations, durable session/JWT revocation and 14 repository authorization subtests |
| Auth resource regressions | PASS; third concurrent password derivation rejected at capacity two; canceled work never starts; capacity released after errors; sequential work succeeds |
| HTTP/JWT/guest controls | PASS; origin checks, secure cookie creation/deletion, required claims, guest resource boundaries |
| Development port configuration | PASS, 12 subtests; ten formerly wildcard mappings failed before the fix |
| SFU lifecycle/races | PASS; 80 join/leave cycles, concurrent join/shutdown, rebinding denial, bounded egress overflow |
| WebSocket queues | PASS; critical overflow and oversized messages close the client; low-priority overflow remains bounded without stopping media |
| Subprocess lifecycle | PASS; output capped at 64 KiB; SIGTERM-resistant helper killed and reaped in approximately 3.003 seconds |
| Go entry-point compilation | PASS: `go build ./cmd/... ./tools/...` |
| Frontend formatting/types/build | PASS under Node 24.19.0 with locked dependencies |
| Frontend Node regressions | PASS, 12/12: seven notification-stream tests and five media lifecycle tests |
| Browser notification reproduction | PASS after fix: five unfinished HTTP 503 responses and five wrong-content-type responses each leave zero open connections; baseline leaves five in each scenario |
| Built frontend browser smoke | PASS, 40/40 checks combined across two builds and two viewports; a further patched-only snapshot run passed 20/20 |
| Browser navigation memory | Executed: 50 warmup + 100 measured navigation cycles per build; see the memory report for GC/snapshot limitations |
| Local frontend container | PASS: health, SPA route, CSP, nosniff, referrer policy, uncached HTML, one-year hashed-asset caching, missing-asset 404 and `nginx -t` |
| Docker builds | PASS: final API, frontend and reverse-proxy images for Linux/arm64 |
| npm audit | Zero known advisories in the audited lockfile snapshot; not proof of security |
| govulncheck | Two reachable findings before dependency updates; zero reachable after; one advisory in an unused OpenPGP package remains module-only |
| Trivy image scan | Final API/frontend/proxy: zero HIGH/CRITICAL findings in this database snapshot; frontend previously had two affected packages for one curl advisory |
| Trivy configuration scan | Six HIGH root-user heuristic flags; contextualized in the security report, not counted as six demonstrated exploitable defects |
| Tracked secret-pattern search | No match for the limited private-key/AWS/GitHub-token patterns; not a history or comprehensive secret scan |
| Patch whitespace | `git diff --check` PASS |

The nine Go test files contain 22 `Test...` functions, including one subprocess helper, with additional table-driven subtests. Do not interpret this as full API coverage. Media tests use synthetic tracks/peers or unnegotiated local Pion connections; subprocess tests use a helper rather than an actual recorded conference.

## Reproduction commands

Run from the repository root with Go 1.26.10 and a supported Node version. On the audited Mac, Node was selected by prefixing `PATH` with `/Users/aleksandranickij/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin`.

```sh
go mod verify
go vet ./...
go test -race -count=1 ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
go build ./cmd/... ./tools/...
bash -n scripts/audit-postgres.sh
bash scripts/audit-postgres.sh
AUDIT_GO_IN_DOCKER=true bash scripts/audit-postgres.sh
```

The database runner creates and removes only its own disposable container. Integration tests additionally reject non-loopback DSNs, unexpected databases/query parameters and databases with existing user relations. Do not replace this DSN with an operator's database.

```sh
cd frontend
npm ci --ignore-scripts --no-audit
npm run check
npm audit --audit-level=high
```

Browser scripts use Playwright and an installed Chromium-compatible browser. These variables selected the existing local tools, without adding dependencies to the product lockfile:

```sh
export PLAYWRIGHT_MODULE_PATH=/Users/aleksandranickij/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright/index.mjs
export CHROME_EXECUTABLE_PATH='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
node tools/audit/notification-stream-browser.mjs
node tools/audit/frontend-browser-smoke.mjs \
  tmp/web-audit-20261026/locked-baseline-dist \
  tmp/web-audit-20261026/frontend-after-dist
AUDIT_NAVIGATION_WARMUP=50 AUDIT_NAVIGATION_CYCLES=100 \
AUDIT_HEAP_SNAPSHOTS=1 AUDIT_OUTPUT_DIR=tmp/web-audit-recheck \
  node tools/audit/frontend-browser-smoke.mjs \
  tmp/web-audit-20261026/frontend-after-dist
node tools/audit/frontend-image-smoke.mjs meetspace-web-audit-frontend:20261026-fixed
```

The notification comparison obtains its baseline from `git show HEAD:frontend/src/api.ts`; after committing the patch, reproduce against the recorded baseline revision instead. Static build directories are local audit artifacts, not committed fixtures. The browser runner serves them on loopback, mocks unauthenticated API calls and blocks external requests. The initial build using Node 23/Vite 8.3.1 is not a valid before/after comparison; the retained `locked-baseline-dist` was built with Node 24.19/Vite 8.3.5.

Runtime images were built with the repository Dockerfiles, explicit `BUILD_VERSION=audit-local-dirty` and the baseline SHA as `BUILD_COMMIT`. API used `--target runtime --build-arg TARGETARCH=arm64`. A first diagnostic frontend build used a non-SHA build marker and was correctly rejected by build metadata validation; the valid-SHA builds passed. No release manifest or provenance claim was made for this dirty checkout.

Image scanning used `docker image save`, then a read-only archive mount, not the Docker socket:

```sh
docker run --rm --memory=2g --cpus=2 \
  --mount type=bind,src=/absolute/path/to/image.tar,dst=/image.tar,readonly \
  aquasec/trivy@sha256:af6acf9a6b85dfe389a1941505c0ce9efef52a4719635e1a962f022a3d855daa \
  image --scanners vuln --severity HIGH,CRITICAL --exit-code 1 \
  --format json --input /image.tar
```

During this audit, explicit local evidence/cache mounts also saved JSON results. Trivy DB `UpdatedAt` was `2026-10-26T07:15:18.945937093Z`. The scope does not include lower severities, all worker/infrastructure images, all architectures or every build tool dependency.

| Final local image | Image ID |
| --- | --- |
| API | `sha256:a29842ce5d9bba8ac8c088777416aeed0b84cbd0966a769f0db18ce169df2ef5` |
| Frontend | `sha256:2c795c96351a30bc16861b82545acfb8e8ecd645b5e708009f2ee602e5b00cbc` |
| Proxy | `sha256:d75c6e3c6ea0d0fbc806852d4eb1b8e1cfbbf2766f6661583310df512362a259` |

## Evidence retained locally

Raw outputs, browser screenshots, heap snapshots, profiles and image archives are under the ignored directory `tmp/web-audit-20261026/`; they are not part of the repository patch. Reports contain the relevant redacted results. Important files:

- `evidence/go-race-final.log`, `go-vet-final.log`, `staticcheck-final.log`, `go-build-final.log`.
- `evidence/postgres-final.log`, `postgres-ci-mode.log`, `lifecycle-final.log`.
- `evidence/frontend-check-final.log`, `notification-stream-browser-final.log`, `frontend-image-smoke.log`.
- `evidence/govulncheck-before.log`, `govulncheck-after.log`, `npm-audit-before.json`.
- `evidence/trivy-api-final.json`, `trivy-frontend.json`, `trivy-frontend-after.json`, `trivy-proxy-after.json`, `trivy-config-baseline.json`.
- `browser/frontend-smoke.json`, `browser-warmed/frontend-smoke.json`, `browser-snapshots/heap-comparison.json` and associated PNG/heap snapshot files.

`evidence/auth-security.log` is the auth reviewer's redacted execution transcription; the independently rerun PostgreSQL logs above are raw command output. Empty successful Go build/vet logs are expected. Diagnostic image archives/cache were retained for reproducibility; disposable test containers were removed, and no unrelated Docker resources were pruned.

## CI changes

Added `.github/workflows/web-audit.yml`: Go race/vet/build/checksums/govulncheck, a separate disposable-database authorization job, frontend formatting/tests/types/build/npm audit. Checkout is SHA-pinned with read-only permissions and no persisted Git credentials. `.gitlab-ci.yml` now runs Go race tests and frontend regressions and uses Go 1.26.10. `npm run check` includes the new tests.

Equivalent commands passed locally, including the Docker-based database runner. The hosted GitHub/GitLab workflows have **not** been executed in this task. Browser smoke remains an explicit local tool, not a claimed hosted E2E gate. Container provenance/license review and a full-image scanner matrix remain release work.

## Compatibility and unexecuted scenarios

| Area | Status |
| --- | --- |
| Anonymous desktop/mobile Chromium UI | Executed; no horizontal overflow or uncaught errors in covered screens |
| Firefox, Safari, real mobile browsers/tablets | Not executed |
| Desktop/Android packaged clients | Not executed; no API route, response schema or database migration changed |
| Authentication | Unit and repository/service integration executed; real browser login/logout/register against the API not executed |
| Meetings/guest/waiting-room/moderation | Selected scope and repository authorization tests only; no complete workflow E2E |
| Real 2–5-user WebRTC, TURN/NAT, devices, screen sharing, reconnect/multi-tab | Not executed; requires isolated full stack, synthetic users and agreed resources |
| Messaging/uploads/replies/edit/delete/folders | Selected authorization and client lifecycle tests; full API contract/UI workflows not executed |
| Recording/playback/download/storage failure | Authorization and subprocess cleanup only; no real recording or outage run |
| Redis/RabbitMQ outage, DB saturation, restart/drain/backup restore | Static inspection only; no fault injection |
| Load/latency/throughput/CPU/FDs/packet loss/jitter | Not measured; no agreed full-stack load scenario |
| CWV/Lighthouse/React profiler/throttled network | Not measured; static loopback smoke is not a realistic performance environment |
| Production TLS/firewall/storage ACLs/SMTP/provider delivery | Not tested or changed |

The password concurrency cap intentionally adds HTTP 429 under aggregate overload, including account deletion password checks. This needs realistic staging concurrency/UX validation; it is not an API schema break. Passing the above tests supports review and staged acceptance of the narrow fixes, not an unconditional production-readiness certificate.
