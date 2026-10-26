# MeetSpace web memory and lifecycle audit

Date: 2026-10-26. Baseline checkout: `3ea9a4e83e82484c25402963b2e68a0642d986f6`.

## Conclusion and scope

One browser resource leak was reproduced and fixed: rejected notification-stream responses were not explicitly cancelled. An unfinished error response could keep its HTTP connection open after the caller had rejected it. Both synthetic stream tests and a loopback Chromium HTTP fixture distinguish the baseline failure from the corrected behavior.

The other lifecycle tests added in this audit passed against the existing media and worker implementation. No speculative production changes were made to Go signaling, SFU, RTP forwarding, or FFmpeg lifecycle code. Passing these bounded tests is **not** evidence that the complete application is leak-free.

Testing used local synthetic identities, mock browser devices, loopback HTTP fixtures, and isolated Go managers. There were no production calls, real account credentials, live participant media, or production deployment changes. Local Go tests ran on macOS/arm64; the initial sample used Go 1.26.9 and the final run used Go 1.26.10. Browser measurements used Chromium 156.0.8078.47.

## Lifecycle inventory

The following inventory separates static ownership review from executed runtime checks.

| Component and code | Ownership and existing cleanup | Evidence and boundary |
| --- | --- | --- |
| Notification fetch: `frontend/src/api.ts` | Caller owns a valid SSE response. Rejected, invalid-MIME, stale-account, and aborted responses now cancel their body before returning an error or recovering the session. | Seven targeted stream tests and loopback browser reproduction; confirmed fix described below. |
| Conference socket: `frontend/src/realtime.ts` | Effect cleanup aborts pending work, clears reconnect/chat timers, detaches socket callbacks, clears the socket reference, and closes the socket. Retry delay is capped. | Static review; authenticated reconnect storms and actual tab-close behavior were not exercised. |
| Browser media: `frontend/src/media.ts` | `stop()` clears timers and pending requests, closes the peer connection, disposes microphone processors, stops owned camera/screen tracks, and clears stream/ICE/publication references. Late capture completion checks disposal. | Five tests with mocked tracks and peer connections; no actual ICE negotiation, device, or codec test. |
| Audio activity: `frontend/src/audioActivity.ts` | Sampling interval exists only while observations exist. `stop()` disconnects nodes, removes track/document/context listeners, closes the audio context, and clears references. Borrowed tracks remain owned by the media client. | Static review; browser audio-device churn and audio-context retained objects were not profiled. |
| Attachment object URLs: `frontend/src/components/ChatPanel.tsx`, `GroupChats.tsx` | Object-URL creation and revocation paths exist for downloaded content, group avatars, and local previews. | Static inventory only; repeated large attachment/avatar replacement was not measured. |
| Meeting WebSocket: `internal/transport/websocket/handler.go` | Separate bounded critical/low-priority queues; size limits; read/write deadlines; writer completion is joined; unregister and socket shutdown have explicit ownership. | Three queue/size regression tests. Real TCP socket/FD counts were not sampled. |
| User event WebSocket: `internal/transport/websocket/user_handler.go` | Connection slots and workers are tracked; drain closes clients; run cancellation closes subscription/socket and joins owned work. | Static review; broker outage and repeated account switching were not exercised end to end. |
| Realtime hub: `internal/usecase/realtime/hub.go` | Bounded low-priority events; cancellable receive/janitor workers; unregister removes connection ownership; shutdown cancels and joins workers with a deadline. Broker retry uses bounded backoff. | Static review; Redis outage, cleanup latency, and sustained janitor cost remain unmeasured. |
| SFU manager/peer: `internal/infrastructure/sfu/manager.go`, `peer.go` | Registry mutation is locked; network cleanup occurs after detachment. Peer cancellation closes the peer connection before waiting for workers. Join/shutdown coordination tracks in-flight registration. | Repeated join/leave, concurrent join/shutdown, and identity-rebinding tests. Peers are real Pion objects but do not negotiate with remote peers. |
| RTP subscriptions: `internal/infrastructure/sfu/tracks.go` | Per-target queues and packet sizes are bounded; unsubscribe cancels work and stops the sender; keyframe requests are coalesced. | Static review; actual RTP/RTCP traffic, congestion, and weak-network behavior were not load-tested. |
| Recording egress: `internal/infrastructure/sfu/egress.go` | Queue overflow terminates the output instead of growing the buffer; close removes registry ownership and is idempotent. | Fifty forced queue-overflow/close cycles with a two-frame queue. |
| FFmpeg process/output: `internal/infrastructure/ffmpeg/bounded.go`, `segment_recorder.go` | Child cancellation uses termination followed by bounded kill escalation; `Wait()` has one owner. Captured diagnostic output is a concurrency-safe 64 KiB tail. | Real synthetic subprocess tests exercise `runBounded`, not an FFmpeg codec or `segmentRecorder.Stop()` end-to-end test. |
| Composite recording: `internal/infrastructure/composite/capture.go`, `internal/usecase/recorder/composite.go` | Bounded capture/compose queues; response-body closure; reader cancellation and joining; recording lease and inactivity deadline; publication and temporary-directory cleanup paths. | Static review; interrupted upload, disk-full, failed finalization, and orphan-directory recovery were not injected. |
| Worker audio/transcription and broker | Audio decoder/process deadlines, temporary-directory cleanup, worker cancellation, and broker publish confirmation/reconnect ownership are present. | Static review; real transcription providers, Redis/RabbitMQ outages, and process trees were not exercised. |

The inventory does not assert exhaustive coverage of every observer, cache, listener, database handle, or HTTP response in the repository.

## Confirmed finding: rejected notification streams retained connections

### Reproduction and impact

The baseline `notificationEvents()` rejected non-success HTTP status codes and successful responses with the wrong MIME type without cancelling the unread response body. When the server kept that body open, the browser still retained the HTTP/1.1 connection. Reconnect attempts could consume the per-origin connection pool even though the API promises had already rejected.

The local HTTP fixture deliberately returns unfinished responses. Each scenario makes five sequential requests, below Chromium's six-connection HTTP/1 pool limit. Requests remain local; no deployed MeetSpace endpoint is contacted.

| Scenario | Baseline | Corrected code |
| --- | --- | --- |
| Synthetic rejected response bodies explicitly cancelled | 0 / 50 | 50 / 50 |
| Browser HTTP 503 unfinished responses still open after five rejected calls | 5 / 5 | 0 / 5 |
| Browser HTTP 200 with invalid SSE MIME, unfinished responses still open after five rejected calls | 5 / 5 | 0 / 5 |

The browser fixture waits for closure and also samples after forced GC. Baseline connections remained open; corrected connections closed without relying on GC. The final repeat confirmed these results and its raw output is saved locally at `tmp/web-audit-20261026/evidence/notification-stream-browser-final.log`. The fixture is included at `tools/audit/notification-stream-browser.mjs` for reproduction. Its small heap samples include module warmup and are not retained-heap leak measurements.

### Fix and regression coverage

`frontend/src/api.ts` now cancels unread response bodies on HTTP failure, invalid SSE content type, account change, and abort observed after response acquisition. A rejected 401 body is released before session recovery, including recovery failures. A valid SSE response is left open for the caller to consume and close.

`frontend/tests/notification-stream.test.mjs` covers all seven cases. These checks verify stream ownership directly; they do not infer a specific number of bytes saved or claim lower whole-application heap usage.

## Browser retained-heap observations

### Matched unauthenticated navigation runs

The baseline and corrected static production builds used the locked frontend dependencies, a 1440 × 1000 viewport, 50 warmup cycles, and 100 measured SPA navigation cycles through landing, login, registration, and landing again. API responses were mocked as unauthenticated. Measurements were taken after explicit GC; the browser script includes cooldown. Mobile smoke used 390 × 844 but did not collect a repeated-navigation heap series.

| Build | Used JS heap at measured cycle 0 | Used JS heap at cycle 100 | Delta | DOM nodes | JS event listeners | Documents |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Locked baseline | 4,149,388 B | 4,432,484 B | +283,096 B | 693 → 693 | 179 → 179 | 4 → 4 |
| Corrected | 4,114,524 B | 4,396,160 B | +281,636 B | 693 → 693 | 179 → 179 | 4 → 4 |

Source: local `tmp/web-audit-20261026/browser-warmed/frontend-smoke.json`.

Both finite runs show residual heap growth of a similar size. This is not a measured heap improvement from the SSE fix: these navigation scenarios are unauthenticated and do not exercise the actual authenticated notification stream. Stable node/listener/document counts narrow the observation but do not prove absence of detached trees or all retained closures.

### Separate corrected-build heap-snapshot run

A separate run collected snapshots at cycles 0 and 100. Its runtime used-heap samples were **4,112,860 B → 4,397,456 B**. Do not substitute these numbers into the matched-run table above.

Grouped snapshot **shallow-size**, not retained-size, differences were:

| Snapshot category | Net shallow-size change |
| --- | ---: |
| V8 code | +262,196 B |
| Native objects | +125,836 B |
| Arrays | +76 B |
| Numbers | +48 B |
| JavaScript objects | 0 B |
| Closures | 0 B |
| Strings | 0 B, with a net count change of +3 |

Large named groups include V8 instruction/byte-array structures, inspector network-resource data, layout-shift records, and performance entries. This is consistent with substantial runtime/instrumentation contribution. It is **not** a dominator/retaining-path analysis and does not establish that all growth is benign. Snapshot category totals and runtime used-heap samples are different metrics and should not be added or directly compared.

Local artifacts: `tmp/web-audit-20261026/browser-snapshots/frontend-after-dist-heap-0.heapsnapshot`, `frontend-after-dist-heap-100.heapsnapshot`, `frontend-smoke.json`, and `heap-comparison.json`. The comparison utility is `tools/audit/compare-heap-snapshots.mjs`.

### Mock media ownership checks

`frontend/tests/media-lifecycle.test.mjs` executes the real `ConferenceMediaClient` against synthetic browser media objects:

- 100 camera join/leave cycles: zero live owned mock tracks and zero unclosed mock peer connections after every cycle.
- A camera capture resolving after stop has its track stopped and cannot send a join request.
- Camera constraints resolving after stop cannot recreate a peer connection.
- A screen-capture request resolving after leave has its track stopped and cannot retain a local screen stream.
- Joining without devices never requests camera/microphone access.

All five media and seven SSE tests passed in the final frontend check. These are lifecycle regressions, not browser WebRTC E2E, actual hardware-resource counts, or real media heap measurements.

## Go lifecycle measurements

### SFU join/leave samples

`TestRepeatedJoinLeaveReleasesResources` performs four batches of 20 local join/leave cycles. Each leave must cancel the peer context, close its Pion peer connection, and return owned room/peer/track/subscription registry counts to zero. After each batch, the test forces GC, waits 25 ms, forces GC again, and logs process-wide runtime counters. There is no hard heap-size assertion.

Initial local run, Go 1.26.9; recorded in executed tool output:

| Completed cycles | HeapAlloc | HeapObjects | Goroutines |
| ---: | ---: | ---: | ---: |
| 20 | 489,728 B | 1,200 | 2 |
| 40 | 501,472 B | 1,244 | 2 |
| 60 | 507,424 B | 1,268 | 2 |
| 80 | 513,664 B | 1,281 | 2 |

Final independent run, Go 1.26.10; saved in `tmp/web-audit-20261026/evidence/lifecycle-final.log`:

| Completed cycles | HeapAlloc | HeapObjects | Goroutines |
| ---: | ---: | ---: | ---: |
| 20 | 474,568 B | 1,158 | 2 |
| 40 | 501,088 B | 1,234 | 2 |
| 60 | 511,704 B | 1,261 | 2 |
| 80 | 521,784 B | 1,286 | 2 |

Goroutine counts were stable and ownership assertions passed. HeapAlloc and HeapObjects grew modestly in both runs. No lifecycle production patch separates these two samples, and toolchain/process warmup differ; this is **not** a before/after optimization benchmark. Longer steady-state runs and allocation/retaining-path analysis are needed before classifying the heap growth. No claim of zero retained-heap growth is made.

A separately captured Go heap profile is available locally as `tmp/web-audit-20261026/evidence/sfu-heap.pprof` with `sfu-heap-top.txt` in the same directory. Its sampled `inuse_space` summary attributes approximately 2,051 KiB to runtime allocation, timer, and OS-thread setup paths. It is a short synthetic-test profile, not a paired production heap diff, and cannot exclude smaller retained application objects.

### Bounded queues and shutdown

| Test | Executed result | Limitation |
| --- | --- | --- |
| Concurrent join/shutdown | 20 start-gated join workers complete; shutdown rejects later joins; registry returns to zero. Also passed under race detection. | Does not force every possible interleaving or exercise remote negotiated peers. |
| Recording egress overflow | 50 outputs terminate when a two-frame queue overflows; queue remains at two frames; repeated close is safe; room/output ownership returns to zero; drop count is 50. | Artificially small queue, no recorder network traffic. |
| Critical WebSocket queue overflow | Third offer to a two-slot critical queue stops the client with `slow_client`; queue stays bounded. | Queue test, not a slow real socket. |
| Recoverable event overflow | 1,000 low-priority offers fill only eight low-priority slots; critical queue remains empty and media client is not stopped. | Does not verify subsequent DB resynchronization. |
| Oversized outbound event | A 1,025-byte event under a 1,024-byte configured cap stops the client with `outbound_too_large`; nothing is enqueued. | Synthetic payload boundary. |

### Worker process lifecycle

`internal/infrastructure/ffmpeg/bounded_test.go` launches the test executable as a child process. It does not require installed FFmpeg and deliberately does not claim codec/recording correctness.

- A child producing 256 KiB of output leaves exactly the final **64 KiB** in the diagnostic buffer.
- Four concurrent writers, each making 100 writes of 1 KiB, keep the shared tail within 64 KiB under race detection.
- A readiness marker ensures the helper ignores SIGTERM before cancellation. Cancellation escalates to kill, and `runBounded` returns only after waiting for the child. The initial race-enabled run measured **3.002833125 s**; the final non-race run measured **3.002566542 s**. These are separate observations around the existing three-second escalation deadline, not an optimization comparison.

Real segment recording, all FFmpeg grandchildren/process groups, pipe-drain failures, crash recovery, temporary-directory accumulation, and disk-pressure cleanup remain untested.

## Reproduction and evidence

Run from the repository root unless stated otherwise. These commands do not contact a deployed MeetSpace application:

```sh
go test -count=1 -v ./internal/infrastructure/sfu ./internal/infrastructure/ffmpeg ./internal/transport/websocket
go test -race -count=5 ./internal/infrastructure/sfu
go test -race -count=1 -v ./internal/infrastructure/ffmpeg ./internal/transport/websocket
npm --prefix frontend test
```

All commands above were executed successfully during the audit. The final frontend run additionally passed lint, typechecking, and production build through `npm run check`; its log is `tmp/web-audit-20261026/evidence/frontend-check-final.log`. Global race-test evidence is in `evidence/go-race-final.log`.

For the local browser fixture, use Node with TypeScript transformation support and an available Playwright installation/browser:

```sh
node tools/audit/notification-stream-browser.mjs
```

`PLAYWRIGHT_MODULE_PATH` and `CHROME_EXECUTABLE_PATH` may point to an existing local installation. The browser fixture compares `git show HEAD:frontend/src/api.ts` with the working tree; after committing the fix, preserve or explicitly select the original baseline to reproduce the before/after comparison. Comparing a fixed HEAD with the fixed working tree is not a historical baseline.

To repeat navigation sampling against already-built local assets:

```sh
AUDIT_NAVIGATION_WARMUP=50 AUDIT_NAVIGATION_CYCLES=100 \
  node tools/audit/frontend-browser-smoke.mjs \
  tmp/web-audit-20261026/locked-baseline-dist \
  tmp/web-audit-20261026/frontend-after-dist
```

Raw build directories, snapshots, and logs under `tmp/` are local audit artifacts, not guaranteed tracked repository contents. See the test report for the complete environment and wider suite results.

## Unmeasured areas and acceptance boundary

- **File descriptors:** not measured before or after. Socket/process ownership assertions are not a substitute for FD counts.
- **Authenticated browser lifecycles:** repeated successful login/logout, account switching through the UI, live chat/recording navigation, and notification reconnect over a real backend were not exercised.
- **Real media:** two-to-five users, TURN/NAT fallback, actual RTP/RTCP, microphone changes, screen-share cycles, reconnect storms, tab termination, weak networks, and CPU throttling were not tested.
- **Long-lived services:** no hours-long goroutine/heap/FD soak, broker outage, PostgreSQL pool saturation, storage interruption, or full worker restart/finalization recovery test was run.
- **Retained memory:** browser category diffs are shallow-size summaries, not retained-size dominators. Go samples are short-process, unnegotiated-peer counters. Neither establishes an application-wide steady-state plateau.
- **Authentication work budget:** the new two-slot Argon2 semaphore bounds concurrent 64 MiB derivations, not total process RSS or post-GC heap. Other allocations and GC-retained buffers remain outside that concurrency budget.

The confirmed notification-response leak is regression-tested and fixed. The broader acceptance criterion of no monotonically growing retained heap/goroutines/FDs across normal authenticated media workloads is **not yet established**. The next validation should use an isolated, resource-limited staging environment with synthetic users, matched pre/post workloads, cooldown and GC samples, FD/goroutine counts, and retained-heap paths before any further lifecycle optimization.
