#!/usr/bin/env python3
"""Opt-in P1 measurements. Operates only on its own disposable Docker fixtures."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import time

ROOT = Path(__file__).resolve().parents[2]
SOURCE = Path(__file__).resolve().parent / "harness"
PACKAGES = ("sfu", "postgres", "rabbitmq", "ffmpeg")


def execute(args, *, env=None, log=None, timeout=600):
    if log:
        with log.open("a") as output:
            subprocess.run(args, cwd=ROOT, env=env, stdout=output,
                           stderr=subprocess.STDOUT, timeout=timeout, check=True)
        return ""
    return subprocess.check_output(args, cwd=ROOT, env=env, text=True,
                                   stderr=subprocess.STDOUT, timeout=timeout).strip()


def save_private(path, value):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as output:
        json.dump(value, output, indent=2)
    path.chmod(0o600)


def inspect_owned(state, container):
    row = json.loads(execute(["docker", "inspect", container["id"]]))[0]
    key, value = state["label"].split("=", 1)
    if (row["Id"] != container["id"] or
            row["Name"] != "/" + container["name"] or
            row["Config"]["Labels"].get(key) != value):
        raise RuntimeError("Refusing to operate on a foreign container")
    return row


def services_up(out):
    path = out / "services.private.json"
    if path.exists():
        raise RuntimeError("Fixture state already exists; use services-down first")
    prefix = "meetspace-p1-" + secrets.token_hex(6)
    state = {"label": "meetspace.p1.audit=" + prefix, "network": prefix + "-net",
             "containers": {}, "password": secrets.token_urlsafe(24),
             "preexisting": execute(["docker", "ps", "-q"]).split()}
    save_private(path, state)
    execute(["docker", "network", "create", "--label", state["label"], state["network"]])
    fixtures = (
        ("postgres", "postgres:17-alpine", "5432", ["-e", "POSTGRES_USER=p1audit",
          "-e", "POSTGRES_PASSWORD=" + state["password"], "-e", "POSTGRES_DB=p1audit"]),
        ("redis", "redis:7-alpine", "6379", []),
        ("rabbit", "rabbitmq:4.2.9-management", "5672", ["-e", "RABBITMQ_DEFAULT_USER=p1audit",
          "-e", "RABBITMQ_DEFAULT_PASS=" + state["password"]]),
    )
    for name, image, port, extra in fixtures:
        cname = prefix + "-" + name
        cmd = ["docker", "run", "-d", "--name", cname, "--label", state["label"],
               "--network", state["network"], "-p", "127.0.0.1::" + port, *extra, image]
        if name == "redis":
            cmd += ["redis-server", "--save", "", "--appendonly", "no"]
        cid = execute(cmd)
        state["containers"][name] = {"id": cid, "name": cname, "image": image}
        save_private(path, state)
        row = inspect_owned(state, state["containers"][name])
        binding = row["NetworkSettings"]["Ports"][port + "/tcp"][0]
        if binding["HostIp"] != "127.0.0.1":
            raise RuntimeError("Loopback port binding required")
        state["containers"][name].update(port=int(binding["HostPort"]), imageID=row["Image"])
        save_private(path, state)
        print("Started isolated " + name, flush=True)
    probes = {"postgres": ["pg_isready", "-U", "p1audit", "-d", "p1audit"],
              "redis": ["redis-cli", "ping"], "rabbit": ["rabbitmq-diagnostics", "-q", "ping"]}
    deadline = time.monotonic() + 90
    while probes and time.monotonic() < deadline:
        for name, cmd in list(probes.items()):
            # Avoid creating a root-owned Erlang cookie during broker startup.
            user = ["--user", "rabbitmq"] if name == "rabbit" else []
            probe = subprocess.run(["docker", "exec", *user, state["containers"][name]["id"], *cmd],
                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10)
            if probe.returncode == 0:
                del probes[name]
        if probes:
            time.sleep(1)
    if probes:
        raise RuntimeError("Readiness failed; run services-down to remove this fixture")
    print("Disposable services ready")


def services_down(out):
    path = out / "services.private.json"
    state = json.loads(path.read_text())
    removed = []
    for container in state["containers"].values():
        # A previous interrupted cleanup may have already removed this exact ID.
        if not execute(["docker", "ps", "-aq", "--no-trunc", "--filter", "id=" + container["id"]]):
            continue
        inspect_owned(state, container)
        execute(["docker", "rm", "-fv", container["id"]])
        removed.append(container["id"])
    network = json.loads(execute(["docker", "network", "inspect", state["network"]]))[0]
    key, value = state["label"].split("=", 1)
    if network["Labels"].get(key) != value or network["Containers"]:
        raise RuntimeError("Refusing to remove a foreign/nonempty network")
    execute(["docker", "network", "rm", state["network"]])
    current = execute(["docker", "ps", "-q", "--no-trunc"]).split()
    missing = [cid for cid in state["preexisting"] if not any(x.startswith(cid) for x in current)]
    (out / "cleanup.json").write_text(json.dumps({"removed": removed,
        "preexisting_not_running_at_cleanup": missing}, indent=2))
    path.unlink()
    print("Removed only the recorded audit containers, their anonymous volumes and network")


def fixture_environment(out):
    state = json.loads((out / "services.private.json").read_text())
    for container in state["containers"].values():
        row = inspect_owned(state, container)
        if not row["State"]["Running"]:
            raise RuntimeError("Audit container is not running")
    services = state["containers"]
    return {**os.environ,
        "RECORDER_STAGE1_TEST_POSTGRES_DSN": "postgres://p1audit:" + state["password"] +
            "@127.0.0.1:" + str(services["postgres"]["port"]) + "/p1audit?sslmode=disable",
        "RECORDER_AUDIT_MIGRATIONS": str(ROOT / "database/migrations"),
        "RECORDER_STAGE2_TEST_REDIS_ADDR": "127.0.0.1:" + str(services["redis"]["port"]),
        "RECORDER_P1_REDIS_ADDR": "127.0.0.1:" + str(services["redis"]["port"]),
        "RABBITMQ_TEST_URL": "amqp://p1audit:" + state["password"] + "@127.0.0.1:" +
            str(services["rabbit"]["port"]) + "/%2F"}


def prepare(out, args):
    mapping = {}
    for source in SOURCE.rglob("*.go.txt"):
        relative = source.relative_to(SOURCE).with_suffix("")
        virtual = ROOT / relative
        if virtual.exists():
            raise RuntimeError("Refusing to hide an existing test: " + str(virtual))
        target = out / "harness" / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, target)
        mapping[str(virtual)] = str(target)
    overlay = out / "overlay.json"
    overlay.write_text(json.dumps({"Replace": mapping}, indent=2))
    env = {**os.environ, "GOCACHE": os.environ.get("GOCACHE", str(out / "go-cache"))}
    go = os.environ.get("GO_BIN", "go")
    for package in PACKAGES:
        for race in (False, True) if args.race and package != "ffmpeg" else (False,):
            suffix = "-race" if race else ""
            execute([go, "test", "-c", "-overlay=" + str(overlay), *(["-race"] if race else []),
                "-o", str(out / (package + suffix + ".test")),
                "./internal/infrastructure/" + package], env=env, log=out / "build.log")
    if args.baseline_group:
        baseline = Path(args.baseline_group).resolve()
        if not baseline.is_file():
            raise RuntimeError("Missing baseline source snapshot")
        before = {**mapping, str(ROOT / "internal/infrastructure/postgres/group_repository.go"): str(baseline)}
        before_overlay = out / "before-overlay.json"
        before_overlay.write_text(json.dumps({"Replace": before}, indent=2))
        execute([go, "test", "-c", "-overlay=" + str(before_overlay), "-o",
            str(out / "postgres-before.test"), "./internal/infrastructure/postgres"],
            env=env, log=out / "build.log")
    manifest = {"commit": execute(["git", "rev-parse", "HEAD"]),
                "go": execute([go, "version"]), "race_built": args.race,
                "baseline_group": str(Path(args.baseline_group).resolve()) if args.baseline_group else None,
                "group_source_sha256": hashlib.sha256((ROOT / "internal/infrastructure/postgres/group_repository.go").read_bytes()).hexdigest(),
                "harness_sha256": {str(p.relative_to(SOURCE)): hashlib.sha256(p.read_bytes()).hexdigest()
                                   for p in SOURCE.rglob("*.go.txt")}}
    if args.baseline_group:
        manifest["baseline_sha256"] = hashlib.sha256(Path(args.baseline_group).read_bytes()).hexdigest()
    (out / "source.json").write_text(json.dumps(manifest, indent=2))
    print("Audit binaries built; production files were not overwritten")


def test(out, package, pattern, env, *, race=False, log=None, extra=()):
    binary = out / (package + ("-race" if race else "") + ".test")
    execute([str(binary), "-test.run=" + pattern, "-test.v", "-test.timeout=6m", *extra],
            env=env, log=log or out / (package + ".log"), timeout=390)


def workload(out, args):
    env = dict(os.environ)
    if args.command in ("db", "groups-compare", "redis", "rabbit", "lifecycle"):
        env = fixture_environment(out)
    if args.command == "db":
        folder = out / ("data-" + args.phase)
        folder.mkdir(exist_ok=True)
        env.update(RECORDER_P1_DB_PERF="true", RECORDER_P1_GROUP_PERF="true",
                   RECORDER_P1_DB_PROFILE_DIR=str(folder), RECORDER_P1_DB_CONFERENCES="1000")
        binary = out / ("postgres-before.test" if args.phase == "before" else "postgres.test")
        execute([str(binary), "-test.run=^TestP1(DBWorkloads|GroupPerformance|GroupCreationContracts)$",
                 "-test.v", "-test.timeout=6m"], env=env, log=out / ("db-" + args.phase + ".log"))
    elif args.command == "groups-compare":
        env["RECORDER_P1_GROUP_PERF"] = "true"
        for phase in ("before", "after"):
            (out / ("groups-" + phase + ".txt")).write_text("")
        for sample in range(6):
            for phase in (("before", "after") if sample % 2 == 0 else ("after", "before")):
                binary = out / ("postgres-before.test" if phase == "before" else "postgres.test")
                execute([str(binary), "-test.run=^$", "-test.bench=^BenchmarkP1GroupCreation$",
                         "-test.benchmem", "-test.benchtime=30x", "-test.timeout=2m"],
                        env=env, log=out / ("groups-" + phase + ".txt"))
        if shutil.which("benchstat"):
            execute(["benchstat", str(out / "groups-before.txt"), str(out / "groups-after.txt")],
                    log=out / "benchstat.txt")
    elif args.command == "media":
        env.update(RECORDER_P1_MEDIA="true", RECORDER_P1_MEDIA_SECONDS="5",
                   RECORDER_P1_PROFILE_DIR=str(out / "media"))
        test(out, "sfu", "^TestP1MediaPerformance$", env)
        test(out, "sfu", "^$", env, log=out / "rtp-bench.txt", extra=(
             "-test.bench=^BenchmarkP1RecordPacket$", "-test.benchmem",
             "-test.benchtime=300ms", "-test.count=6"))
    elif args.command in ("redis", "rabbit"):
        folder = out / args.command
        folder.mkdir(exist_ok=True)
        env.update(RECORDER_P1_PROFILE_DIR=str(folder), RECORDER_P1_REDIS_LOAD="true")
        if args.command == "redis":
            package, pattern, bench = "postgres", "^TestP1(RedisReconciliationLoad|RedisRecoveryLatency|AccountPresenceBatches)$", "^BenchmarkP1PresencePage$"
        else:
            package, pattern, bench = "rabbitmq", "^Test(ConsumerReconnectRecoveryLatency|CommandRoundTrip)$", "^Benchmark(ConfirmedCommandSerial|ConfirmedCommandParallel|PublisherReconnect)$"
        test(out, package, pattern, env, log=out / (args.command + ".log"))
        test(out, package, "^$", env, log=out / (args.command + "-bench.txt"), extra=(
             "-test.bench=" + bench, "-test.benchmem", "-test.benchtime=300ms", "-test.count=3"))
    elif args.command == "lifecycle":
        env.update(RECORDER_P0_MEDIA_CYCLES=str(args.cycles), RECORDER_P0_WS_STRESS="true",
                   RECORDER_P1_GROUP_PERF="true", RECORDER_P0_PROFILE_DIR=str(out / "p0-ws"))
        test(out, "postgres", "^Test(P0WS|P1GroupCreationContracts)", env, race=True, log=out / "p0-ws.log")
        env["RECORDER_P0_PROFILE_DIR"] = str(out / "p0-sfu")
        test(out, "sfu", "^TestP0Media(Lifecycle|SourceChurn)$", env, race=True, log=out / "p0-sfu.log")
        test(out, "rabbitmq", "^Test(PublisherCancels|PublisherClose|AMQPRPC|AMQPHandshake)",
             env, race=True, log=out / "p0-rabbit.log", extra=("-test.count=3",))
    print(args.command + " complete; inspect " + str(out))


def recording(out, args):
    if not args.ffmpeg_image:
        raise RuntimeError("recording requires --ffmpeg-image with an existing trusted local image")
    arch = execute(["docker", "image", "inspect", "--format={{.Architecture}}", args.ffmpeg_image])
    if arch not in ("arm64", "amd64"):
        raise RuntimeError("Unsupported image architecture")
    image_id = execute(["docker", "image", "inspect", "--format={{.Id}}", args.ffmpeg_image])
    env = {**os.environ, "GOOS": "linux", "GOARCH": arch, "CGO_ENABLED": "0",
           "GOCACHE": os.environ.get("GOCACHE", str(out / "go-cache"))}
    binary = out / "ffmpeg-linux.test"
    execute([os.environ.get("GO_BIN", "go"), "test", "-c", "-overlay=" + str(out / "overlay.json"),
             "-o", str(binary), "./internal/infrastructure/ffmpeg"], env=env, log=out / "build.log")
    folder = out / "recording"
    folder.mkdir(exist_ok=True)
    (folder / "p0").mkdir(exist_ok=True)
    name = "meetspace-p1-ffmpeg-" + secrets.token_hex(6)
    try:
        execute(["docker", "run", "--rm", "--pull=never", "--name", name,
             "--network", "none", "--read-only", "--tmpfs", "/tmp:rw,exec,nosuid,size=512m",
             "-v", str(binary) + ":/audit.test:ro", "-v", str(folder) + ":/evidence",
             "-e", "RECORDER_P1_FINALIZATION=true", "-e", "RECORDER_P0_FFMPEG_STRESS=true",
             "-e", "RECORDER_P1_PROFILE_DIR=/evidence", "-e", "RECORDER_P0_EVIDENCE_DIR=/evidence/p0",
             "--entrypoint", "/audit.test", args.ffmpeg_image,
             "-test.run=^Test(P1FinalizationProfile|P0RealFFmpegResourceCycles)$",
             "-test.v", "-test.timeout=3m"], log=out / "recording.log", timeout=210)
    finally:
        subprocess.run(["docker", "rm", "-f", name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    (folder / "image.json").write_text(json.dumps({"image": args.ffmpeg_image, "id": image_id, "arch": arch}))
    print("Recording finalization and process lifecycle measurements complete")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("prepare", "services-up", "services-down", "db",
                        "groups-compare", "media", "redis", "rabbit", "lifecycle", "recording"))
    parser.add_argument("--output", required=True, help="New evidence directory under repository tmp/")
    parser.add_argument("--baseline-group")
    parser.add_argument("--race", action="store_true", help="Build race binaries for lifecycle")
    parser.add_argument("--phase", choices=("before", "after"), default="after")
    parser.add_argument("--cycles", type=int, default=25)
    parser.add_argument("--ffmpeg-image")
    args = parser.parse_args()
    out = Path(args.output).resolve()
    if not out.is_relative_to(ROOT / "tmp") or out == ROOT / "tmp":
        parser.error("output must be a dedicated directory inside repository tmp/")
    if not 1 <= args.cycles <= 1000:
        parser.error("cycles must be 1..1000")
    out.mkdir(parents=True, exist_ok=True)
    if args.command == "prepare":
        prepare(out, args)
    elif args.command == "services-up":
        services_up(out)
    elif args.command == "services-down":
        services_down(out)
    elif args.command == "recording":
        recording(out, args)
    else:
        workload(out, args)


if __name__ == "__main__":
    try:
        main()
    except subprocess.CalledProcessError as error:
        # Service commands can contain generated credentials: never echo argv/output.
        raise SystemExit("Audit subprocess failed (exit " + str(error.returncode) + "); inspect the local log")
    except subprocess.TimeoutExpired:
        raise SystemExit("Audit subprocess timed out; inspect the local log and run services-down")
