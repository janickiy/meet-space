# MeetSpace web audit summary

Date: 2026-10-26. Baseline: `3ea9a4e83e82484c25402963b2e68a0642d986f6` on `main`, with pre-existing meeting-toolbar edits preserved. This audit produced a local, uncommitted patch. Nothing was pushed, deployed, deleted from a remote host or tested against production.

## Executive result

The audit confirmed and fixed a notification-stream resource leak, unbounded aggregate password-hashing work, two reachable Go dependency advisories, a vulnerable curl package in a locally built web image, and unintended development-service exposure on all host interfaces. Regression tests, repeatable local browser checks and CI definitions were added. No P0 issue was confirmed in the inspected scope.

The narrow fixes are suitable for code review and isolated staging acceptance. **Do not label the entire application production-ready from this audit:** real authenticated multi-user browser flows, negotiated WebRTC/TURN, recording, long-running calls and infrastructure failure recovery were not executed. No measured claim of faster video, lower production latency or lower process RSS is made.

## Verified fixes and before/after evidence

| Finding | Before | After |
| --- | --- | --- |
| Rejected notification SSE responses were not released | Synthetic errors: 0/50 bodies canceled; Chrome: 5/5 connections still open after GC for both HTTP 503 and invalid MIME | 50/50 bodies canceled; Chrome: 0/5 open in each case; stale identity/abort paths also covered |
| Password derivations had no shared concurrency ceiling | Third derivation entered a blocking fake hasher; already-canceled requests could start password work | Two slots shared by login, registration and account deletion; excess returns existing rate-limit error/HTTP 429; cancellation and release tested |
| Go standard-library/RabbitMQ advisories | govulncheck reported two reachable advisories | Go minimum 1.26.10 and amqp091-go 1.14.0; zero reachable advisories in the checked snapshot |
| Cached web-image package layer retained vulnerable curl | curl and libcurl 8.22.0-r0 flagged for one HIGH advisory | Both web/proxy Dockerfiles enforce at least 8.23.0-r0; final API/frontend/proxy image scans report zero HIGH/CRITICAL findings |
| Development listeners were exposed beyond loopback | Ten wildcard host mappings, including database/cache/storage/debug interfaces | Bound to 127.0.0.1; all 12 configuration assertions pass; public HTTPS/media ports unchanged |

The Argon2 limit corresponds to **128 MiB of concurrently active derivation working memory by configuration**, not a measured process-memory ceiling. Existing 64 MiB password parameters were not weakened. Completed allocations may remain until GC; in-flight derivations are not interruptible. The limit is per API service instance, not cluster-wide.

Total JavaScript assets changed from 833,591 to 833,870 bytes (+279); consistently gzipped assets changed from 259,174 to 259,225 bytes (+51). No meaningful bundle reduction was claimed or attempted without a measured bottleneck.

## Testing and memory results

- Go module verification, vet, Staticcheck, full race-enabled test run and entry-point compilation passed.
- New PostgreSQL regressions passed separately with actual migrations and synthetic data: durable session revocation plus 14 cross-user/role authorization cases. Ordinary `go test` skips these integration cases without the explicit audit DSN; the new CI has a separate disposable-database job.
- Frontend formatting, TypeScript, production build and 12 Node regression tests passed.
- Browser smoke passed 40/40 checks combined across baseline/patched builds at desktop/mobile widths; an additional patched-only snapshot run passed 20/20.
- SFU tests completed 80 join/leave cycles with application registries empty and goroutines stable at two. Bounded WebSocket/egress queues and subprocess kill/reap behavior were verified.
- After 50 warmup + 100 anonymous SPA navigation cycles, DOM nodes/listeners/documents stayed at 693/179/4. Heap increased similarly in both builds; separate snapshots primarily attribute growth to compiled code/browser instrumentation, with zero net growth in grouped object/closure/string shallow sizes. This is not retained-dominator analysis or proof about authenticated/media paths.
- API, frontend and proxy images built locally for Linux/arm64. Frontend container startup, security headers and asset-cache behavior passed. Production TLS and all worker/infrastructure images were not verified.

There is no confirmed unresolved critical/high finding from the reproduced defects in this patch. That statement is bounded by the tests above, not an assertion that the entire project has no vulnerabilities. The six root-user image/config scanner flags, module-only unused OpenPGP advisory and untested areas are explicitly retained in the security report.

## Prioritized remaining work

| Priority | Item | Evidence/status and acceptance needed |
| --- | --- | --- |
| Release gate | Authenticated 2–5-user full-stack acceptance | Not executed. Exercise registration/login/logout, meetings, guest/waiting room, moderation, chat/files/folders, access revocation and real recording with synthetic accounts |
| Release gate | Real media reliability/soak | No negotiated multi-client call/TURN fallback, network loss, screen-share/device switching or recording-failure run; agree resource limits before long-running/load tests |
| Release gate | All release images/platforms and deployment checks | Only three Linux/arm64 images scanned; scan workers/storage/infra and AMD64 release artifacts; validate TLS, migrations, backup/restore and rollback in disposable staging |
| P2 policy | Durable session expiry | Revocation works, but no server-enforced idle/absolute expiration was found; browser refresh-cookie age is 400 days. Decide intended session policy before changing compatibility |
| P2 policy | Signed recording link lifetime | A 24-hour signed URL remains a bearer capability after membership changes; decide acceptable lifetime/revocation behavior |
| P2 hardening | Root-user/config flags and build provenance | Review actual entrypoint privilege dropping and Compose user overrides; avoid blindly setting `USER` on stateful images. Finish license/provenance/complete secret-history review |
| P2 hypothesis | Long-lived client maps/sets | Caption announcement IDs, call intent IDs and unconsumed prejoin devices need targeted long-session/expiry tests before changing retention behavior |
| P2 performance | Large chat histories and public entry bundle | Manually loaded pages are not virtualized/capped and shared layout is eager; measure realistic LCP/INP/CLS, React commit cost and history/network growth before optimizing |
| P2 capacity | Password concurrency limit UX | Validate expected concurrency and overload handling on staging; preserve the bound and password strength rather than disabling protection |
| P2 coverage | Failure/recovery and database performance | Redis/RabbitMQ/S3 outages, pool saturation, query plans, queue depths, p50/p95/p99, FD counts and packet loss/jitter remain unmeasured |

These release gates are missing evidence, not newly proven vulnerabilities. No changes to session policy, signed-link compatibility, database schema, API contracts or media codec/transport behavior were made speculatively.

## Changed files

Production/runtime changes:

- `frontend/src/api.ts`: cancel rejected/stale notification response bodies and reject obsolete requests.
- `internal/usecase/auth/password_work.go`, `service.go`, `accounts.go`: shared bounded password derivations.
- `go.mod`, `go.sum`, `Makefile`, `.gitlab-ci.yml`: corrected dependency/toolchain minimum and validation gates.
- `docker-compose.yml`, `docs/DEPLOYMENT.md`: loopback-only development service access and SSH-tunnel guidance.
- `frontend/Dockerfile`, `dockers/production/Dockerfile`: patched curl/libcurl floor, including cached-layer invalidation.
- `frontend/package.json`, `.github/workflows/web-audit.yml`: repeatable frontend/Go/database checks.

Regression tests and audit tools:

- `internal/app/auth/session_security_test.go`.
- `internal/config/deployment_test.go`.
- `internal/infrastructure/security/jwt_test.go`.
- `internal/infrastructure/postgres/authorization_integration_test.go`.
- `internal/infrastructure/sfu/lifecycle_test.go`.
- `internal/infrastructure/ffmpeg/bounded_test.go`.
- `internal/transport/http/middleware/auth_test.go`.
- `internal/transport/websocket/backpressure_test.go`.
- `internal/usecase/auth/password_work_test.go`.
- `frontend/tests/notification-stream.test.mjs`, `media-lifecycle.test.mjs`.
- `scripts/audit-postgres.sh`.
- `tools/audit/notification-stream-browser.mjs`, `frontend-browser-smoke.mjs`, `compare-heap-snapshots.mjs`, `frontend-image-smoke.mjs`.

The existing edits in `frontend/src/components/RealtimePanel.tsx` and `frontend/src/pages/conference.css` belong to prior work, were included in the checkout used for builds, and were not changed by this audit. Local test containers were removed; audit images, raw evidence and scanner cache remain available locally. No global Docker cleanup was performed.

## Detailed reports

- [Security and threat model](WEB_SECURITY_AUDIT.md).
- [Memory/resource lifecycle](WEB_MEMORY_LEAK_AUDIT.md).
- [Performance and measurement limits](WEB_PERFORMANCE_REPORT.md).
- [Tests, environment, exact commands and unexecuted suites](WEB_TEST_REPORT.md).

Raw local artifacts are under ignored `tmp/web-audit-20261026/`. They contain synthetic-test output and are not automatically included in a commit or release. Review this patch, then run the missing full-stack acceptance suite in an explicitly agreed isolated environment before publishing a release. This task stops at audit/fixes and does not deploy them.
