#!/usr/bin/env python3
"""Run opt-in P2 contracts through a Go overlay, without editing application files."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import runpy
import shutil
import subprocess
import time

ROOT = Path(__file__).resolve().parents[2]
HARNESS = Path(__file__).resolve().parent / "harness"


def audit_directory(value):
    path = Path(value).resolve()
    if not path.is_relative_to(ROOT / "tmp") or path == ROOT / "tmp":
        raise argparse.ArgumentTypeError("use a dedicated directory under repository tmp/")
    return path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=audit_directory)
    parser.add_argument("--services", type=audit_directory,
                        help="P1 runner's owned disposable services directory")
    parser.add_argument("--race", action="store_true")
    parser.add_argument("--packages", nargs="*", help="internal/... package paths; default all templates")
    parser.add_argument("--count", type=int, default=1)
    parser.add_argument("--bench", action="store_true", help="run benchmarks only (never with --race)")
    args = parser.parse_args()
    if args.count < 1 or args.count > 100 or (args.bench and args.race):
        parser.error("count must be 1..100; benchmarks cannot use race")
    args.output.mkdir(parents=True, exist_ok=True)
    overlay = {}
    packages = set()
    hashes = {}
    for source in sorted(HARNESS.rglob("*_test.go.txt")):
        relative = source.relative_to(HARNESS).with_suffix("")
        package = relative.parent.as_posix()
        if args.packages and package not in args.packages:
            continue
        virtual = ROOT / relative
        if virtual.exists():
            raise RuntimeError("refusing to hide an existing test: " + str(virtual))
        target = args.output / "harness" / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, target)
        overlay[str(virtual)] = str(target)
        packages.add(package)
        hashes[relative.as_posix()] = hashlib.sha256(source.read_bytes()).hexdigest()
    if not packages or (args.packages and set(args.packages) != packages):
        parser.error("no templates found for one or more requested packages")
    overlay_path = args.output / "overlay.json"
    overlay_path.write_text(json.dumps({"Replace": overlay}, indent=2))
    env = dict(os.environ)
    if args.services:
        fixture = runpy.run_path(str(ROOT / "tools/performance-audit/run.py"))
        env = fixture["fixture_environment"](args.services)
        env["RECORDER_TEST_REDIS_ADDR"] = env["RECORDER_STAGE2_TEST_REDIS_ADDR"]
    env.setdefault("GOCACHE", str(args.output / "go-cache"))
    cmd = [env.get("GO_BIN", "go"), "test", "-overlay=" + str(overlay_path),
           "-count=" + str(args.count), "-timeout=120s", "-v"]
    if args.race:
        cmd.append("-race")
    if args.bench:
        cmd += ["-run=^$", "-bench=^BenchmarkP2", "-benchmem", "-benchtime=300ms"]
    cmd += ["./" + package for package in sorted(packages)]
    name = "bench" if args.bench else "race" if args.race else "test"
    started = time.monotonic()
    with (args.output / (name + ".log")).open("w") as log:
        result = subprocess.run(cmd, cwd=ROOT, env=env, stdout=log, stderr=subprocess.STDOUT)
    summary = {"exit": result.returncode, "seconds": round(time.monotonic() - started, 2),
               "packages": sorted(packages), "race": args.race, "count": args.count,
               "redis_fixture": bool(args.services), "harness_sha256": hashes}
    (args.output / (name + ".json")).write_text(json.dumps(summary, indent=2))
    print(f"P2 {name}: exit {result.returncode}; {len(packages)} packages; {summary['seconds']}s; {args.output}")
    raise SystemExit(result.returncode)


if __name__ == "__main__":
    main()
