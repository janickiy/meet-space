# Architecture inventory and P2 contracts

`go run ./tools/architecture-audit` emits the production `internal/` package
inventory as JSON: imports, inbound dependencies, public declarations, fields,
interfaces and function source spans/branch nodes. Branch nodes include closures;
they are not cyclomatic complexity. Tests and `cmd/` inbound edges are excluded.

`check.py` runs the opt-in Go contracts in `harness/**/*.go.txt` through Go's
overlay support. It refuses to mask an existing test file. It does not modify
application source, load `.env`, contact production or use deployment secrets.
Generated copies, hashes, logs and results go under the specified repository
`tmp/` directory. Python 3.10+ and the Go version from `go.mod` are required.

```sh
export GOCACHE=/private/tmp/meetspace-go-cache
go run ./tools/architecture-audit > /tmp/meetspace-architecture.json
python3 tools/architecture-audit/check.py --output tmp/p2-contracts
python3 tools/architecture-audit/check.py --output tmp/p2-contracts --race --count 3
```

These cover application/HTTP/media/WS error contracts, group member presence,
real Pion rejected-join/close ownership, notification SSE admission and legacy
recording no-media/rollback behavior. Without Redis, SSE delivery/stream cleanup
tests explicitly skip; admission and failed-subscription tests still execute.
The ordinary `go test ./...` does not automatically include these templates.

## Real SSE and lifecycle regression

Reuse only the disposable services created by the P1 audit runner. `--services`
validates their exact IDs/names/labels before using their loopback addresses. The
SSE tests create isolated UUID channels and close clients/subscriptions; no
`FLUSHDB`, production accounts or shared data are used.

```sh
python3 tools/performance-audit/run.py prepare --output tmp/p2-fixtures --race
python3 tools/performance-audit/run.py services-up --output tmp/p2-fixtures
python3 tools/architecture-audit/check.py --output tmp/p2-contracts-redis \
  --services tmp/p2-fixtures --race --count 3
python3 tools/performance-audit/run.py lifecycle --output tmp/p2-fixtures
python3 tools/performance-audit/run.py services-down --output tmp/p2-fixtures
```

Always clean up with `services-down`, including after a failed test. See
`../performance-audit/README.md` for the DB/media/FFmpeg measurements and their
limitations. Keep before/after evidence in separate directories. Do not run timed
measurements concurrently with builds, race tests or other audit workloads.

For targeted checks, pass `--packages internal/app/personal
internal/usecase/personal`. Every requested package must have a harness template.
`--bench --count 6 --packages internal/app/notifications` runs the small rejected
SSE request benchmark (including the request fixture), without a race build.
It is not a sustained SSE throughput benchmark.

See `docs/code-review/P2_ARCHITECTURE_AUDIT.md` and `P2_BEFORE_AFTER.md` for the
current audit, compatibility decisions and remaining work.
