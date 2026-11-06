#!/usr/bin/env python3
"""Bounded, read-only API load for an explicitly disposable loopback HTTPS stack.

Credentials are read as JSON from stdin and never written to results. The report
contains only operation names, status counts and latency percentiles.
"""
import argparse
import collections
import concurrent.futures
import http.client
import json
import math
import ssl
import sys
import threading
import time
import urllib.parse
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--url', default='https://localhost:18081')
    parser.add_argument('--requests', type=int, default=1200)
    parser.add_argument('--concurrency', type=int, default=8)
    parser.add_argument('--rate', type=int, default=40)
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    target = urllib.parse.urlsplit(args.url)
    if (target.scheme != 'https' or target.hostname not in ('localhost', '127.0.0.1')
            or target.username or target.password or target.path not in ('', '/')
            or target.query or target.fragment or target.port in (None, 443)):
        parser.error('only an explicit non-443 loopback HTTPS test port is allowed')
    if not (1 <= args.requests <= 5000 and 1 <= args.concurrency <= 16 and 1 <= args.rate <= 50):
        parser.error('bounds: requests 1..5000, concurrency 1..16, rate 1..50/s')
    credentials = json.load(sys.stdin)
    tls = ssl._create_unverified_context()  # Only synthetic, loopback test TLS.
    def connect():
        return http.client.HTTPSConnection(target.hostname, target.port, context=tls, timeout=10)
    login = connect()
    try:
        login.request('POST', '/api/v1/admin/auth/login', json.dumps(credentials),
                      {'Origin': args.url.rstrip('/'), 'Content-Type': 'application/json'})
        response = login.getresponse()
        response.read()
        if response.status != 200:
            raise RuntimeError(f'fixture login returned {response.status}')
        cookie = response.getheader('Set-Cookie', '').split(';', 1)[0]
        if not cookie:
            raise RuntimeError('fixture login did not set a session')
    finally:
        login.close()
        credentials.clear()
    routes = {
        'dashboard': '/dashboard',
        'users_search': '/users?q=audit-199&sort=createdAt&order=desc&pageSize=20',
        'audit': '/audit?pageSize=20&sort=createdAt&order=desc',
        'conferences': '/conferences?pageSize=20',
    }
    local = threading.local()
    connections = []
    lock = threading.Lock()
    started = time.monotonic()
    def request(index):
        due = started + index / args.rate
        delay = due - time.monotonic()
        if delay > 0:
            time.sleep(delay)
        if not hasattr(local, 'connection'):
            local.connection = connect()
            with lock:
                connections.append(local.connection)
        name = list(routes)[index % len(routes)]
        begin = time.monotonic()
        try:
            local.connection.request('GET', '/api/v1/admin' + routes[name], headers={'Cookie': cookie})
            result = local.connection.getresponse()
            result.read()
            status = str(result.status)
        except (OSError, http.client.HTTPException):
            local.connection.close()
            status = 'transport_error'
        return name, (time.monotonic() - begin) * 1000, status
    try:
        with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as pool:
            results = list(pool.map(request, range(args.requests)))
    finally:
        for connection in connections:
            connection.close()
    elapsed = time.monotonic() - started
    report = {'requests': args.requests, 'concurrency': args.concurrency,
              'rate_limit_rps': args.rate, 'elapsed_seconds': round(elapsed, 3),
              'actual_rps': round(args.requests / elapsed, 3), 'routes': {}}
    for name in routes:
        subset = [(ms, status) for route, ms, status in results if route == name]
        times = sorted(ms for ms, _ in subset)
        def percentile(p):
            return round(times[max(0, math.ceil(len(times) * p) - 1)], 3) if times else None
        report['routes'][name] = {'count': len(subset), 'p50_ms': percentile(.5),
                                 'p95_ms': percentile(.95), 'p99_ms': percentile(.99),
                                 'statuses': dict(collections.Counter(status for _, status in subset))}
    Path(args.output).write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(report, ensure_ascii=False))
    return 0 if all(status == '200' for _, _, status in results) else 1


if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except (RuntimeError, json.JSONDecodeError) as error:
        raise SystemExit(str(error))
