# MeetSpace Web Performance Report

> Исторический отчёт: отражает состояние проекта на дату проверки. После удаления распознавания речи и переименования сервиса записи его перечень компонентов не является описанием текущей версии. Актуальные сведения: [архитектура](../../ARCHITECTURE.md).

Date: 2026-10-26. Baseline commit: `3ea9a4e83e82484c25402963b2e68a0642d986f6`.

Later repository integration is recorded in [the publication follow-up](WEB_AUDIT_SUMMARY.md#repository-publication-integration-2026-10-26). Bundle, browser and profile numbers below belong to the original audit builds, not the subsequently integrated upstream landing page.

## Scope and interpretation

This audit used the existing dirty local checkout, not a clean release image or a production deployment. Existing changes to `frontend/src/components/RealtimePanel.tsx` and `frontend/src/pages/conference.css` were preserved in both frontend builds. Tests used loopback servers, mocked browser API responses, and synthetic backend fixtures. No production traffic, credentials, accounts, recordings, or real meetings were used.

The confirmed improvements are bounded resource ownership and overload behavior, not a demonstrated increase in application throughput. No performance tuning was applied without evidence. Unmeasured metrics below must not be interpreted as passing acceptance thresholds.

| Environment | Value |
| --- | --- |
| Host | macOS 27, ARM64, 18 logical CPUs, 48 GiB RAM |
| Node.js | 24.19.0 |
| Frontend dependency installation | Locked `npm ci --ignore-scripts` |
| Vite | 8.3.5 |
| Browser | Headless Chromium/Google Chrome 156.0.8078.47 |
| Go | 1.26.10 |
| Browser viewports | Desktop 1440 × 1000; mobile-sized 390 × 844 |
| Browser network/device preset | Local loopback; no CPU or network throttling; reduced motion |

An earlier build used an already-installed Vite 8.3.1 and an unsupported global Node version. It was excluded from the comparison. The measured baseline was rebuilt with the locked dependencies and supported Node runtime before the frontend source fix.

## Frontend build and transfer-size baseline

Production builds completed with TypeScript checking. Baseline output is `tmp/web-audit-20261026/locked-baseline-dist`; patched output is `tmp/web-audit-20261026/frontend-after-dist`.

The following counts sum all emitted `assets/*.js` files, using Node's `gzipSync` consistently for both builds. They are not initial-route transfer totals: lazy chunks are included. CSS, fonts, public images, response headers, cache reuse, and Brotli compression are excluded.

| Metric | Baseline | Patched | Difference |
| --- | ---: | ---: | ---: |
| JavaScript chunks | 36 | 36 | 0 |
| Total JavaScript, raw bytes | 833,591 | 833,870 | +279 |
| Total JavaScript, gzip bytes | 259,174 | 259,225 | +51 |
| Largest `index` chunk, raw bytes | 376,170 | 376,170 | 0 |
| Largest `index` chunk, gzip bytes | 110,398 | 110,392 | -6 |
| Shared `ui` chunk, raw bytes | 112,708 | 112,987 | +279 |
| Shared `ui` chunk, gzip bytes | 36,521 | 36,566 | +45 |
| Conference page chunk, raw bytes | 58,618 | 58,618 | 0 |
| Conference page chunk, gzip bytes | 16,348 | 16,353 | +5 |

The small increase comes from notification response cleanup and stale-response checks. Changes to content hashes and imports can change gzip size even when a chunk's raw length is unchanged. This is not a bundle optimization or a measured user-visible speed improvement.

Lazy page modules already exist. Shared layout and media-related dependencies remain reachable from the public application entry. Splitting them further was deferred because initial-route waterfall, render cost, and user impact were not measured. No blanket memoization, dependency replacement, or speculative caching was introduced.

## Browser resource measurements

### Rejected notification stream connections

`tools/audit/notification-stream-browser.mjs` serves the baseline and patched API module on a local HTTP server and supplies unfinished response bodies. Five sequential requests per scenario stay below the usual six-connection HTTP/1 browser limit. After requests reject, the script waits, forces garbage collection, and counts server-side open responses.

| Scenario | Baseline open responses after GC | Patched open responses after GC |
| --- | ---: | ---: |
| Five HTTP 503 responses | 5 / 5 | 0 / 5 |
| Five HTTP 200 responses with invalid SSE content type | 5 / 5 | 0 / 5 |

This confirms that explicit response-body cancellation releases connections that previously survived garbage collection. It prevents accumulation during notification reconnection errors. It does not establish production request latency, RSS savings, or throughput. The companion unit regression observed 0 of 50 rejected streams canceled before the fix and 50 of 50 afterward; all seven notification tests now pass.

The final repeated browser proof is saved in `tmp/web-audit-20261026/evidence/notification-stream-browser-final.log`.

### Repeated anonymous navigation

`tools/audit/frontend-browser-smoke.mjs` exercised actual static production builds with every API request mocked as unauthenticated. Each desktop cycle navigated landing → login → registration → landing via the application links. Baseline and patched builds each ran 50 warmup cycles and 100 measured cycles. Samples were taken after a cooldown and two forced garbage collections. Desktop and mobile smoke checks also covered empty-login validation, protected-route redirect and destination preservation, horizontal overflow, and uncaught exceptions: 40 of 40 checks passed across the two builds and two viewports.

| Metric after warmup / after 100 cycles | Baseline | Patched |
| --- | ---: | ---: |
| Used JavaScript heap, bytes | 4,149,388 → 4,432,484 | 4,114,524 → 4,396,160 |
| Used JavaScript heap change, bytes | +283,096 | +281,636 |
| DOM nodes | 693 → 693 | 693 → 693 |
| JavaScript event listeners | 179 → 179 | 179 → 179 |
| Documents reported by CDP | 4 → 4 | 4 → 4 |

The small heap growth in both variants was investigated rather than described as a leak-free result. A separate patched-build run captured full heap snapshots after the same 50 warmup and 100 measured cycles. Its used JavaScript heap changed from 4,112,860 to 4,397,456 bytes (+284,596). Grouped snapshot **shallow-size** differences were:

| Snapshot group | Net shallow-size difference |
| --- | ---: |
| Compiled code | +262,196 bytes |
| Native objects | +125,836 bytes |
| JavaScript objects | 0 bytes |
| Closures | 0 bytes |
| Strings | 0 bytes |
| Arrays | +76 bytes |
| Numbers | +48 bytes |

The largest code increases were V8 instruction streams (+155,520 bytes) and trusted byte arrays (+63,792 bytes). Native increases included Chromium network-resource inspector data (+91,584 bytes) and buffered layout/resource timing objects. Native snapshot accounting is not equivalent to `Runtime.getHeapUsage.usedSize`; the columns should not be summed into an asserted JavaScript heap total. Grouping is not a dominator-tree retained-size analysis, and net totals can hide individual object replacement.

No growing application-object, DOM-node, or listener group was observed in this bounded anonymous-navigation scenario. This does not certify authenticated navigation, long-running calls, chat history, all browser engines, or the absence of leaks generally. Heap snapshots and the comparison are in `tmp/web-audit-20261026/browser-snapshots/`; the comparable baseline/patched navigation results are in `tmp/web-audit-20261026/browser-warmed/frontend-smoke.json`.

### Media lifecycle coverage

Five additional Node tests exercise the actual conference-media client with synthetic browser primitives. They cover 100 camera join/leave cycles, capture completing after stop, constraints completing after stop, screen capture completing after leave, and joining without devices. No owned tracks or peers remain live after the repeated stop cycles. These are resource-ownership regressions, not real WebRTC performance measurements; no camera, microphone, RTP stream, remote participant, or network impairment was involved.

## Backend password-work bound

The baseline authentication service had no shared bound on concurrently active password derivations. A controlled blocking password-provider fixture reproduced a peak of three active operations when the intended limit was two. It does not establish an observed production concurrency peak.

The fix shares two immediately acquired work slots across registration, login, and password-confirmed deletion. Saturated requests return the existing rate-limit error, mapped to HTTP 429, rather than adding an unbounded waiting queue. Canceled requests do not start password work, and success/failure paths release capacity.

The configured Argon2 working set is 64 MiB per derivation. Two active derivations therefore imply a **theoretical 128 MiB active Argon2 working-set bound per authentication-service instance**. This is not a measured total-process RSS cap: runtime overhead, other requests, already allocated or not-yet-collected memory, and multiple application instances are outside it. The regression uses a blocking synthetic provider, not real Argon2 allocations. Password-strength parameters were not weakened.

The intentional overload tradeoff is more HTTP 429 responses under concurrent authentication pressure instead of uncontrolled active hashing. Normal login p50/p95/p99, sustained throughput, total RSS, CPU contention, and an optimal deployment-specific slot count were not benchmarked. Do not describe the fixed slot count as universally throughput-optimal.

## Go/SFU profiling and database coverage

A short local SFU lifecycle test produced CPU and heap profiles under `tmp/web-audit-20261026/evidence/sfu-*`:

- CPU profile duration: **201.89 ms**; total CPU samples: **10 ms**. The samples landed in certificate-generation cryptography. This sample is too small and the scenario too narrow to support any bottleneck or optimization claim.
- Sampled in-use heap: **2,051.01 KiB**, attributed to runtime thread/timer allocations. This is not whole-server RSS, a representative media-session retained heap, or a before/after performance result.
- The test does not establish audio/video packet throughput, retransmission behavior, TURN performance, jitter, latency, or encoding/recording cost.

Relevant evidence files are `sfu-profile.log`, `sfu-cpu-top.txt`, `sfu-heap-top.txt`, `sfu-cpu.pprof`, and `sfu-heap.pprof` in that evidence directory. No tuning was based on these sparse profiles.

PostgreSQL authorization integration tests used isolated synthetic data. They validate access control, not database performance. Representative query plans, `EXPLAIN (ANALYZE, BUFFERS)`, query counts, pool wait time, index efficiency, N+1 impact, saturation, and transaction-latency percentiles were **not measured**. No database performance changes were made on that basis.

## Explicit measurement gaps

| Requested measurement | Status / reason |
| --- | --- |
| LCP, INP, CLS and Lighthouse scores | Not measured; no controlled Web Vitals/Lighthouse run was executed |
| React Profiler renders and commit durations | Not measured; no React DevTools profiler capture |
| Real weak-network / CPU-throttled calls | Not measured; no isolated multi-user media environment was exercised |
| API p50/p95/p99, throughput and error rate under load | Not measured; no representative service load benchmark |
| Database plans, query latency and pool saturation | Not measured; synthetic authorization tests are not a load benchmark |
| WebRTC packet loss, jitter, bitrate and TURN fallback cost | Not measured; lifecycle fixtures carry no real media traffic |
| Recording throughput, FFmpeg CPU/RSS and finalization latency | Not measured; no representative recording workload |
| Redis/RabbitMQ outage recovery throughput and queue depth | Not measured; no workload-driven dependency-outage test |
| Production transfer/cache behavior | Not measured; frontend served by a local fixture, not the deployed proxy/CDN |
| Desktop/mobile native-client performance | Out of scope for this web audit; not measured |

Consequently, “no meaningful performance regression in video calls, messaging, or recording” remains **unverified**, rather than passed.

## Reproduction and follow-up

From the repository root, with Node 24.19.0 on `PATH`, run the regression suite:

```sh
node --experimental-transform-types --test frontend/tests/*.test.mjs
```

Build the current frontend into a separate output directory from `frontend/`:

```sh
npm run build -- --outDir ../tmp/web-audit-20261026/frontend-after-dist
```

For browser scripts, set `PLAYWRIGHT_MODULE_PATH` to the installed Playwright entry point and `CHROME_EXECUTABLE_PATH` to the chosen Chromium executable when they are not available through normal package/browser discovery. This run used the bundled runtime's `dependencies/node/node_modules/playwright/index.mjs` and `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`.

```sh
node tools/audit/notification-stream-browser.mjs
AUDIT_NAVIGATION_WARMUP=50 AUDIT_NAVIGATION_CYCLES=100 \
  node tools/audit/frontend-browser-smoke.mjs \
  tmp/web-audit-20261026/locked-baseline-dist \
  tmp/web-audit-20261026/frontend-after-dist
AUDIT_NAVIGATION_WARMUP=50 AUDIT_NAVIGATION_CYCLES=100 \
  AUDIT_HEAP_SNAPSHOTS=1 \
  node tools/audit/frontend-browser-smoke.mjs \
  tmp/web-audit-20261026/frontend-after-dist
node tools/audit/compare-heap-snapshots.mjs \
  tmp/web-audit-browser/frontend-after-dist-heap-0.heapsnapshot \
  tmp/web-audit-browser/frontend-after-dist-heap-100.heapsnapshot
```

The SSE script reads its baseline module from `HEAD`. After committing the fix, a separate baseline revision is needed to reproduce the before/after comparison. The browser script defaults to `tmp/web-audit-browser`; set `AUDIT_OUTPUT_DIR` to keep different runs separate. Generated heap snapshots, build artifacts, and local evidence are not intended as production assets.

Prioritized next measurements, before further optimization:

1. Establish a bounded, isolated multi-user staging workload for authenticated navigation, messaging, conference join/leave, real media, and recording; record latency, CPU, retained heap, goroutines, FDs, and packet metrics.
2. Measure cold/warm initial-route waterfalls and Web Vitals with fixed device/network presets before changing eager shared dependencies or image-loading behavior.
3. Profile long chat histories: loaded pages currently are not capped/virtualized, and a 15-second refetch interval may scale work with loaded history. This is a code-inspection hypothesis, not a measured bottleneck.
4. Exercise long-lived caption and call sessions to quantify lifetime-bound deduplication sets/maps, before changing their retention policy.
5. Use representative synthetic database volume and query plans before introducing indexes, cache changes, pool changes, or pagination tuning.

The verified connection cleanup and password-work cap are suitable narrow resilience fixes. Overall production performance readiness requires the missing representative measurements; this report does not certify it.
