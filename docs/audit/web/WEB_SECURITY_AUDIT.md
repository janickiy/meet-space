# MeetSpace web security audit

Audit date: 2026-10-26. Baseline commit: `3ea9a4e83e82484c25402963b2e68a0642d986f6` plus the existing working-tree changes. Findings and fixes below describe the locally audited working tree, not a deployed release.

For subsequent integration with upstream `d896401`, including its expanded attachment policy and guest-session changes, see [the publication follow-up](WEB_AUDIT_SUMMARY.md#repository-publication-integration-2026-10-26). Source-review observations and image scans here describe the original audit baseline and patch, not a complete re-audit of those upstream additions.

## Scope and assurance boundary

The review covered the React/TypeScript web client, Go API and selected workers, authentication and authorization, attachment/recording access, outbound integrations, development networking, dependencies, and selected runtime images. It combined source review, regression tests, real PostgreSQL authorization tests, local browser fixtures, and dependency/image scanning.

Execution was limited to the developer machine and newly created, isolated audit containers. Production websites, SSH servers, real accounts, email delivery, production databases, and existing application volumes were not used. External access was limited to public dependency/advisory sources and image/module downloads. No production penetration test, load test, release publication, or deployment was performed.

Existing changes in `frontend/src/components/RealtimePanel.tsx` and `frontend/src/pages/conference.css` were preserved. They are not security fixes attributable to this audit.

The results support the individual claims below. They do not certify the application as vulnerability-free or constitute complete OWASP ASVS coverage.

## Architecture and threat model

MeetSpace is a collaboration application: meetings, personal/group messaging, attachments, recordings, invitations, and notifications. The web client reaches the Go HTTP API through Nginx. PostgreSQL stores identities, durable sessions, memberships, messages, and recording metadata. Redis supports realtime coordination and rate limiting; RabbitMQ carries asynchronous commands. Workers handle media/recording and product tasks; S3-compatible storage holds objects. WebSocket/SSE and the Pion-based media layer add long-lived connections beyond ordinary request/response traffic.

| Boundary | Untrusted input or actor | Security property reviewed |
| --- | --- | --- |
| Browser → API | Anonymous callers, signed-in users, scoped guests | Authentication, CSRF protection for cookie-backed operations, input limits, resource budgets |
| User → other users' resources | Guessed IDs, revoked memberships, wrong-owner requests | Server-side ownership/membership checks, not UI-only restrictions |
| API → PostgreSQL/object storage | Request-derived IDs, filenames, attachment references | Scoped queries, safe names/content validation, download authorization |
| Browser → realtime/media | Long-lived connections and interrupted navigation | Stream cleanup, bounded queues, lifecycle isolation |
| API/workers → external providers | Provider responses, configured SMTP/HTTP endpoints | TLS verification, timeouts, bounded responses, redirect handling |
| Host → containers | Development listeners, image dependencies, runtime identity | Least network exposure, patched dependencies, contextual hardening |

Assets at risk include account/session credentials, private conversations, meeting participation, recordings and attachments, and server/client availability. Relevant adversaries include unauthenticated request floods, authenticated outsiders, revoked participants, and compromised or misbehaving upstream services. Operator access to configuration and private signing material is a separate privileged trust boundary.

### Implemented service layout and exposure

The source entry points are API, media worker (Pion SFU), recording worker (FFmpeg/storage), product worker (asynchronous product/provider jobs), and live-caption worker. These paths are implemented, but provider delivery, transcription, summaries and live captions depend on operator configuration and were not exercised here. Their presence in code is not evidence that a particular deployment has them enabled.

| Listener/data flow | Production template default | Boundary |
| --- | --- | --- |
| Browser HTTPS/WSS → Nginx | Host 443/TCP | Public entry; shared-host overlay instead binds proxy 8080 to host 127.0.0.1:19080 behind an existing TLS proxy |
| Nginx → frontend/API | Frontend 8080, API 8085 | Private Compose network; no production API host mapping |
| API → worker controls | Recording 8090, media 8091, product 8092, live captions 8093 | Private HTTP control/health/metrics listeners |
| Browser ↔ Pion media | Host 50010/UDP and TCP | Deliberately public media transport; HTTP authentication does not replace media authorization |
| Browser ↔ TURN | 3478/UDP+TCP, 5349/TCP, relay 49160–49259/UDP | Public NAT traversal; credentials and expiry must remain separate from HTTP access tokens |
| API/workers → PostgreSQL, Redis, RabbitMQ, object storage | Private network, no base production host mappings | Membership/session persistence, coordination, jobs and private object access |
| Metrics/profiling | Prometheus 127.0.0.1:19090 when enabled; pprof disabled by default and loopback-bound | Operator-only; API operations also use configured authorization |

The hardened installation overlay replaces the historical MinIO implementation with SeaweedFS behind the same storage service name. Do not infer the deployed object-store implementation from that name alone. These facts come from `docker-compose.production.yml`, `docker-compose.hardened.yml`, `docker-compose.shared-host.yml` and application configuration; no live firewall or host-port audit was performed.

JWT bearer tokens protect ordinary API requests; refresh uses a Secure/HttpOnly/SameSite cookie scoped to auth routes. WebSocket tickets are scoped and short-lived; origin checks and bounded payloads/queues are present. Provider jobs may transfer message/recording data to configured STT/AI/calendar/email services. Their retention, consent and data-processing policies require operator review beyond code-level TLS checks. Object retention/backup/restore was not exercised; signed-link lifetime is discussed below.

## Confirmed findings and local fixes

### SEC-01 — P1: concurrent password derivations were not globally bounded

**Affected paths:** `internal/usecase/auth/service.go`, `internal/usecase/auth/accounts.go`, and `internal/infrastructure/security/password.go`.

Each Argon2id derivation reserves 64 MiB. Registration, login, and password-confirmed account deletion previously called the hasher without a shared concurrent-work budget. Per-IP request rate limits do not bound simultaneous work across different clients. This creates an availability risk from concurrent expensive derivations; it is not a password-hashing weakness.

**Reproduction:** before the fix, `TestPasswordWorkRejectsOverload` admitted a third concurrent derivation for all three operation paths. Cancellation tests also showed password work starting for already-canceled contexts. The regression uses a deterministic blocking hasher: it measures admitted concurrency, not process memory consumption or a successful denial-of-service attack.

**Fix:** `internal/usecase/auth/password_work.go` provides two shared slots on the auth service used by the API. Excess work immediately returns the existing rate-limit error, mapped to HTTP 429, without building a waiting queue. Context cancellation is checked before work starts; every completed/error path releases its slot. Existing Argon2 parameters and ordinary authentication behavior are preserved.

**Verification:** overload, canceled-context, error/slot-release, and 24 sequential-success checks pass with the race detector. The theoretical active Argon2 allocation budget is now `2 × 64 MiB = 128 MiB` per auth service, excluding other allocations. This is not an observed RSS ceiling. Already-running Argon2 work cannot be interrupted; multiple API processes each have their own budget.

Evidence: `tmp/web-audit-20261026/evidence/auth-security.log`, explicitly labeled as a redacted transcription of executed results, including the baseline failures.

### SEC-02 — P1: rejected notification streams could retain connections

**Affected path:** `frontend/src/api.ts`, `api.notificationEvents`.

HTTP errors and successful responses with an invalid SSE content type were rejected without canceling their response bodies. If a server or proxy kept such a response open, notification retries could accumulate live connections and exhaust the browser's per-origin connection pool.

**Reproduction:** a loopback-only Chrome 156 fixture issued five sequential requests for each scenario. For HTTP 503 and HTTP 200 with `text/html`, all five baseline responses remained open after garbage collection. With the fix, both scenarios retained zero open responses. The seven SSE unit regressions changed from six failures/one pass to seven passes.

**Fix:** cancel rejected bodies, release a 401 response before session recovery, and cancel a response received after abort or identity change. A valid SSE response remains open for its caller to consume.

This is a reproduced resource/reliability defect, not demonstrated authentication bypass or cross-account data disclosure. Heap figures from this small fixture include module warmup and are not a memory-leak rate measurement.

Evidence: `tmp/web-audit-20261026/evidence/notification-stream-browser-final.log`, `frontend/tests/notification-stream.test.mjs`, and `tmp/web-audit-20261026/evidence/frontend-check-final.log`.

### SEC-03 — P1: reachable Go dependency advisories

The baseline `govulncheck` run found two reachable advisories across 82 root packages and 73 modules:

| Advisory | Baseline → local fix | Reachability evidence |
| --- | --- | --- |
| [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809) | Go 1.26.9 → 1.26.10 | SMTP TLS handshake reaches `crypto/x509.Certificate.Verify`; the update distrusts specified compromised intermediate certificates |
| [GO-2026-6700](https://pkg.go.dev/vuln/GO-2026-6700) | `amqp091-go` 1.13.0 → 1.14.0 | RabbitMQ connection setup reaches `amqp091.DialConfig`; the update enforces the pre-negotiation frame bound |

The standard-library/toolchain floor and build/CI references were aligned, and `go.mod`/`go.sum` updated. Existing application Dockerfiles already selected Go 1.26.10; the inconsistent local minimum, Makefile and GitLab version were corrected. The Go 1.26.9 finding is therefore not proof that the deployed Docker binary used that version. The subsequent scan reports no reachable symbol or imported-package findings. Reachability identifies an affected call path; neither advisory was exploited against a running service during this audit.

One module-level warning remains: [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932), concerning the unmaintained `golang.org/x/crypto/openpgp` package. The application requires the containing module but does not import this package in the scanned graph. It is not counted as an exploitable application finding, and unrelated cryptographic functionality was not removed to silence it.

Evidence: `tmp/web-audit-20261026/evidence/govulncheck-before.log` and `govulncheck-after.log`.

### SEC-04 — P1 dependency remediation: vulnerable curl in runtime images

The baseline frontend runtime image contained `curl` and `libcurl` `8.22.0-r0`, both flagged HIGH for [CVE-2026-92392](https://curl.se/docs/CVE-2026-92392.html). This is one advisory affecting two installed packages, not two independent vulnerabilities. The upstream issue concerns an HTTP/2 upload use-after-free; application-specific exploitability was not demonstrated.

`frontend/Dockerfile` and `dockers/production/Dockerfile` now enforce `curl>=8.23.0-r0` and `libcurl>=8.23.0-r0` after package updates. The explicit floor invalidates an older cached upgrade layer. Newly rebuilt frontend and proxy images, plus the final rebuilt API image containing all source fixes, reported zero HIGH/CRITICAL findings in their final Trivy vulnerability scans.

These results apply to the specific locally built images and scanner database at audit time, not every dependency image, architecture, registry artifact, or production container. No claim is made that lower-severity findings or future advisories are absent.

Evidence: `trivy-frontend.json`, `trivy-frontend-after.json`, `trivy-proxy-after.json`, and `trivy-api-final.json` under `tmp/web-audit-20261026/evidence/`.

### SEC-05 — P1 deployment hardening: development listeners exposed beyond loopback

The development Compose template published internal services and debugger/API ports on all host interfaces by default. Combined with development credentials and debugger capabilities, use on an untrusted network could expose services that should be local. This was confirmed from the template and failing regression, not by probing a user's host or the internet.

Ten wildcard mappings were changed to `127.0.0.1`: API/debug API, debugger ports, worker control/debug control, PostgreSQL, Redis, and both MinIO ports. Twelve regression subtests also cover RabbitMQ's two already-loopback mappings. Public WebRTC transport ports were deliberately not changed, and production Compose networking was not rewritten.

Evidence: `internal/config/deployment_test.go`, `tmp/web-audit-20261026/evidence/dev-ports-before.log`, and `dev-ports-after.log`. The documented development connection model was updated in `docs/DEPLOYMENT.md`.

## Authentication and authorization verification

The following controls were tested rather than inferred only from their presence in code:

| Control | Executed coverage | Result |
| --- | --- | --- |
| Cookie-backed session request origin protection | Eight cases: trusted origin, CLI request, foreign/null origin, cross-site metadata, missing browser origin, form/text content | Passed |
| Session cookie flags | Secure configuration, HttpOnly, SameSite=Lax, host-only scope, auth path, deletion flags | Passed |
| JWT validation | Ten cases including wrong algorithm, missing/expired claims, future issue time, issuer/audience, subject, conflicting guest/session identity | Passed |
| Guest resource scope | Eight middleware route cases exercising conference scope and blocked unrelated resources | Passed |
| Durable session revocation | Real PostgreSQL repositories: login/refresh, wrong-user session binding, access/refresh rejected after logout | Passed |
| Resource authorization | Fourteen real PostgreSQL subtests, listed below | Passed |

The database cases cover allowed member recording access; denied outsider recording access; non-owner recording stop and meeting finish; outsider meeting/direct/group chat access; editing another sender's message; group self-promotion; unauthorized group avatar access; attachment/folder ownership; and removed/kicked membership access.

All 35 migrations were applied to fresh test schemas. The final helper permits only an explicitly named loopback `meetspace_audit` database, rejects connection-override query parameters, requires an empty database, and uses a schema-only `search_path`. Cleanup drops only the newly created schema/container. No application `.env` or production credentials are read.

`scripts/audit-postgres.sh` creates a temporary PostgreSQL container with a 512 MiB memory limit, one CPU, loopback-only ephemeral port, and a 256 MiB tmpfs database. Both host-Go mode and isolated Go-container mode passed. The initial PostgreSQL image was 16.15, linux/arm64, digest `sha256:0cefaabd882e42bcb2a2f3dc56d3ce4770560ba62fd7f197f1a2e008fd5731ab`. Initial audit containers were removed; their cached images may remain.

Evidence: `auth-security.log`, `postgres-final.log`, and `postgres-ci-mode.log` under `tmp/web-audit-20261026/evidence/`. The early transcribed log records a then-open schema-isolation recommendation; the final helper and later two logs supersede that limitation.

These tests exercise selected repository/service boundaries, not the full HTTP routing/authentication stack for every endpoint. A skipped PostgreSQL test without its explicit environment variable must not be interpreted as an authorization pass; use the supplied runner.

## Additional source review and local runtime checks

### Uploads and object access

The inspected upload path validates filenames, size and allowed extensions/content types, and checks scope/membership/ownership on attachment operations. The audited checkout allows JPEG, PNG, WebP, PDF, TXT, and CSV; it does not establish support for every archive format mentioned in historical requests. The upload limit is 10 MiB, and JPEG/PNG dimensions are checked against a 40-million-pixel bound. Text validation and MIME sniffing are present. No archive-extraction path was found in the reviewed upload flow.

The tested SQL boundaries prevented the selected cross-user attachment, folder, recording, and conversation operations. Real object-store transfer, antivirus scanning, malicious-document rendering, and exhaustive content-parser fuzzing were not performed. Authorization checks do not make arbitrary uploaded documents safe to open.

### Outbound integrations

Reviewed provider clients use configured endpoints rather than arbitrary request-supplied recording URLs, bounded timeouts/responses, and disabled automatic redirects. HTTPS is required except for explicit local HTTP configuration. SMTP uses hostname verification and a TLS 1.2 minimum. No actual SMTP message or third-party provider request was sent. These observations do not substitute for network egress controls or validation of an operator's deployed configuration.

### Browser and Nginx controls

An isolated frontend image smoke check passed health and Nginx syntax checks, CSP checks including `script-src 'self'` and `frame-ancestors 'none'`, `nosniff`, `no-referrer`, SPA no-cache behavior, one-year hashed-asset caching, and a missing-asset 404. Its allowed WebSocket origin was a synthetic `audit.invalid` value.

Evidence: `tmp/web-audit-20261026/evidence/frontend-image-smoke.log` and `tools/audit/frontend-image-smoke.mjs`. This was a local HTTP container check; production TLS, certificate chains, reverse-proxy configuration, and external headers were not tested.

### Runtime identities and scanner interpretation

The baseline Dockerfile configuration scan reported six HIGH `DS-0002` root-user flags: MinIO, PostgreSQL, Redis, recording worker, production proxy, and frontend Dockerfiles. These are six contextual hardening flags, not six demonstrated exploitable vulnerabilities. Entry-point privilege dropping, filesystem ownership, process behavior, Linux capabilities, and actual Compose overrides must be reviewed together before changing `USER`.

PostgreSQL/Redis entrypoints drop privileges, and the production recording-worker service explicitly selects `1000:1000`; those context details are not reflected by a Dockerfile-only check. Nginx root-master behavior and the historical MinIO default still require runtime hardening review. No blanket `USER` change or suppression was added merely to reduce the count. Follow-up should validate non-root execution and least privileges for each affected runtime without breaking database initialization, media processing, or Nginx startup. Evidence: `tmp/web-audit-20261026/evidence/trivy-config-baseline.json`.

### JavaScript dependencies and secrets

The locked frontend dependency audit reported zero advisories at audit time. This is a package-database result, not a source-code security review. The filename/pattern-based secret check produced no matches in its inspected scope. It was a heuristic check, not a complete entropy scan, Git-history audit, or assurance that credentials were never disclosed. Secret values were not printed, copied into reports, or used for testing.

## Remaining risks and unverified areas

1. **P2 policy review — durable session lifetime:** session cookies permit 400 days and the inspected durable-session lookup enforces revocation without an independent server-side absolute/idle expiry. Define an explicit lifetime/re-authentication policy before changing compatibility. Short access-token expiry alone does not invalidate a durable refresh session.
2. **P2 policy review — issued object links:** the recording presign path uses a 24-hour lifetime. A URL already issued remains a bearer capability until expiry even if membership changes. Consider shorter lifetimes or an authenticated proxy where immediate revocation is required. Existing direct-chat attachment links have a shorter lifetime, and group streaming rechecks membership.
3. **P2 hardening — six runtime identity flags:** validate effective processes/capabilities and introduce per-image restrictions, as described above. Do not confuse a Dockerfile-level warning with verified remote exploitability.
4. **Coverage gap:** no production TLS/headers, SMTP delivery, object-storage policy, TURN/network egress, full multi-user HTTP/browser workflow, long-duration soak, or cross-platform installer audit was executed. Authorization coverage is representative, not exhaustive.
5. **Coverage gap:** mocked browser media lifecycle tests and local stream fixtures do not establish stability under real camera/network conditions. The separate performance/memory reports describe the measured lifecycle cases and their limits.
6. **Supply-chain follow-up:** rebuild and publish approved artifacts through the release process, then repeat scans on their exact digests and architectures. Local fixes have not changed the running service or published installer.

## Reproduction and evidence

From the repository root, the following commands reproduce the targeted controls without contacting production:

```sh
go test -race -count=1 ./internal/app/auth ./internal/infrastructure/security ./internal/transport/http/middleware ./internal/usecase/auth
go test -count=1 -v ./internal/config -run TestDevelopmentPortsStayOnLoopback
bash scripts/audit-postgres.sh
AUDIT_GO_IN_DOCKER=true bash scripts/audit-postgres.sh
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
npm --prefix frontend run check
npm --prefix frontend audit --audit-level=high
```

The PostgreSQL script creates and removes its own disposable resources and uses only synthetic credentials. Do not point the integration helper at a nonempty or shared database. The optional browser tools require the documented local Playwright/Chromium setup; they block non-loopback traffic for their page fixtures.

Raw and redacted audit evidence is stored locally under `tmp/web-audit-20261026/`, not committed as production data. The new CI workflow reproduces core Go, PostgreSQL, frontend, and dependency checks; its constituent commands were exercised locally, but the workflow has not been represented as a completed remote GitHub run.
