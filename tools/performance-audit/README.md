# P1 performance and lifecycle audit

Opt-in synthetic measurements for the existing SFU, repositories, Redis,
RabbitMQ and FFmpeg. These are audit tools, not a production load generator.
Python 3.10+, the Go version from `go.mod`, and Docker are required.

The `harness/**/*.go.txt` files are Go source templates. `prepare` uses Go's
`-overlay` option to add tests to the existing internal packages without writing
test files into the application tree. It refuses to mask an existing test.
All binaries, profiles, generated credentials and synthetic data go to a
dedicated ignored directory under `tmp/`.

## Run

Run timed suites **sequentially**, with other build/load jobs stopped. Existing
applications can still influence measurements; record the hardware and competing
workloads. The suite does not read `.env`, use deployment credentials, connect to
the remote host, or inspect the user's database.

```sh
export AUDIT_OUT="$PWD/tmp/p1-local-audit"
python3 tools/performance-audit/run.py prepare --output "$AUDIT_OUT" --race
python3 tools/performance-audit/run.py services-up --output "$AUDIT_OUT"
python3 tools/performance-audit/run.py db --output "$AUDIT_OUT"
python3 tools/performance-audit/run.py media --output "$AUDIT_OUT"
python3 tools/performance-audit/run.py redis --output "$AUDIT_OUT"
python3 tools/performance-audit/run.py rabbit --output "$AUDIT_OUT"
python3 tools/performance-audit/run.py lifecycle --output "$AUDIT_OUT" --cycles 25
python3 tools/performance-audit/run.py services-down --output "$AUDIT_OUT"
```

Use `GO_BIN=/path/to/go` and `GOCACHE=/path/to/writable/cache` if needed.
`services-up` creates three uniquely named/labelled disposable containers on a
dedicated bridge with random PostgreSQL/RabbitMQ credentials and ports bound
only to `127.0.0.1`. Image tags can move: preserve the image IDs in the private
state when recording a result. The state file is mode `0600`; never commit it.

The PostgreSQL helper accepts only the loopback `/p1audit` administrative DB,
creates a random `p1_<uuid>` database per test and drops it on cleanup. Fixtures
include 1000 meetings, 20k recordings, 400k recording segments, 1000 groups with
20 members, 19 direct conversations, 50,950 personal messages, and related
synthetic rows. Redis uses random namespaces (no `FLUSHDB`); broker tests own
their queues and exchanges. `lifecycle` deliberately kills only its named
Pub/Sub subscriptions / test AMQP connections.

Always run `services-down`, including after a failed suite. It validates the
recorded container ID, name and label before deletion, removes only those
containers and their anonymous volumes, and refuses a foreign/nonempty network.
It writes a receipt of any preexisting containers no longer running; external
user activity is not attributed to the audit. Never use `docker compose down`,
`docker system prune` or a shared database for these tools.

## Compare the group INSERT optimization

Save the **whole baseline source file** before editing, or extract it from a
known baseline commit. Supply that snapshot to `prepare`; it builds a second
PostgreSQL binary with only that file replaced. The working tree is not reset.

```sh
git show a604c0d49ce5651f481fe0c0a144e5a049759ff4:internal/infrastructure/postgres/group_repository.go > /tmp/p1-group-before.go.txt
python3 tools/performance-audit/run.py prepare --output "$AUDIT_OUT" --race --baseline-group /tmp/p1-group-before.go.txt
python3 tools/performance-audit/run.py db --output "$AUDIT_OUT" --phase before
python3 tools/performance-audit/run.py db --output "$AUDIT_OUT" --phase after
python3 tools/performance-audit/run.py groups-compare --output "$AUDIT_OUT"
```

`groups-compare` runs six alternating before/after samples, 30 creations per
group size per sample, with a fresh disposable DB for each sub-benchmark. If
`benchstat` is on PATH it also saves the comparison. The two-member case is a
control: do not claim a gain when significance is absent. More bytes/op can be a
tradeoff of the bounded batch even when latency and allocation count improve.
The contract suite checks owner/member roles, limits, idempotent retry, rejected
unregistered accounts, and rollback after an injected membership INSERT failure.

## Actual FFmpeg, without restarting a worker

Use a trusted **already installed** image containing `ffmpeg` and `ffprobe`, for
example the image used by the local recording worker. Inspect its exact ID first.
The tool cross-compiles the Go harness for that image's Linux architecture;
the image is not pulled, its normal entrypoint does not run, and no service
network or credentials are attached.

```sh
python3 tools/performance-audit/run.py recording --output "$AUDIT_OUT" --ffmpeg-image go-recorder-worker
```

This container has a read-only root and a 512 MiB executable `/tmp` tmpfs. It
measures two concurrent composite finalizers over existing 4-second H264/AAC
segments (640×360, 10fps, 2-second segments), then checks real FFmpeg process,
pipe, cancellation, error and temp-file lifecycles. Encoding the fixture is
outside the finalization timer. It does **not** measure SFU→capture→upload→DB
latency, physical disk throughput or production recorder capacity. Durable
recovery segments are intentionally retained until the fixture is removed.
The Linux FFmpeg binary uses `CGO_ENABLED=0`, so this particular suite is not a
race build; `lifecycle` covers Go WebRTC/WS/AMQP paths with the race detector.

## Interpret evidence

- DB: 5 warmups, 100 requests per read path, p50/p95/p99, exact SQL and
  `EXPLAIN (ANALYZE, BUFFERS)`, Go allocation deltas and pool stats. Timings are
  local repository/usecase calls. Only recordings HTTP includes in-process Gin
  and JSON; auth verification, TLS and MinIO signing are excluded.
- Media: real Pion clients and SFU in **one process**, synthetic RTP payloads
  160/1200 bytes, 50 packets/source/s, 22-second warmup and 5-second sample.
  CPU and heap include clients, SRTP and bounded retransmission histories.
  Zero application queue drops is not proof of zero WAN packet loss.
- CPU, heap, allocs, mutex, block and goroutine checkpoints are saved. Use
  `pprof -base before.allocs.pprof after.allocs.pprof` (and equivalent mutex/block
  checkpoints) for deltas. Cumulative block time includes concurrent goroutine
  waits and completed waits spanning warmup; it is not request latency or CPU.
  The standalone `github.com/google/pprof` tool can read these profiles if the
  Go distribution does not include `go tool pprof`.
- Redis recovery is `CLIENT KILL` of one owned Pub/Sub subscription while the
  Redis server is healthy. Rabbit recovery is a test channel/connection fault.
  Neither is a long outage or multi-replica thundering-herd SLA.
- FFmpeg child CPU uses `RUSAGE_CHILDREN`; Go CPU sampling does not capture
  subprocess CPU. `/proc/self/io` and tmpfs size are not physical disk iowait.
- Ordinary `go test ./...` currently has no permanent Go tests. A green empty
  run must not replace these explicitly selected, opt-in lifecycle checks.

See `docs/code-review/P1_PERFORMANCE_AUDIT.md` and `P1_BEFORE_AFTER.md` for
the measured results, remaining P1 findings and separate P2 follow-up.
