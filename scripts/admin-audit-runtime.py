#!/usr/bin/env python3
"""Bounded diagnostics for the explicitly named disposable admin-audit stack.

No production target is accepted. Login credentials arrive only as stdin JSON;
reports never contain credentials, cookies, CSRF tokens or response user data.
Container outages require a separate explicit flag and exact ownership labels.
"""
import argparse
import concurrent.futures
import http.client
import json
import re
import select
import socket
import ssl
import subprocess
import sys
import threading
import time
from pathlib import Path

PREFIX = 'meetspace-audit-'
ORIGIN = 'https://localhost:18081'
TLS = ssl._create_unverified_context()  # Fixed loopback synthetic fixture only.


def docker(*args, check=True, timeout=20):
    return subprocess.run(['docker', *args], check=check, capture_output=True, timeout=timeout)


def owned(name):
    if name not in {PREFIX + v for v in ('api', 'proxy', 'postgres', 'redis', 'rabbit', 'storage')}:
        raise RuntimeError('container is not in the fixed disposable allowlist')
    labels = json.loads(docker('inspect', '--format', '{{json .Config.Labels}}', name).stdout)
    if (labels or {}).get('codex.task') != 'admin-audit':
        raise RuntimeError('container ownership label does not match admin-audit')
    return labels


def connection(timeout=12):
    return http.client.HTTPSConnection('localhost', 18081, context=TLS, timeout=timeout)


def request(path, *, cookie='', credentials=None, csrf='', method='GET'):
    conn = connection()
    headers = {'Cookie': cookie} if cookie else {}
    body = None
    if method != 'GET':
        headers.update({'Origin': ORIGIN, 'Content-Type': 'application/json'})
        if csrf:
            headers['X-CSRF-Token'] = csrf
        body = json.dumps(credentials or {})
    start = time.monotonic()
    payload, session = {}, ''
    try:
        conn.request(method, '/api/v1/admin' + path, body, headers)
        response = conn.getresponse()
        raw = response.read()
        status = response.status
        session = response.getheader('Set-Cookie', '').split(';', 1)[0]
        if raw:
            payload = json.loads(raw)
    except (OSError, http.client.HTTPException, json.JSONDecodeError) as error:
        status = type(error).__name__
    finally:
        conn.close()
    result = {'status': status, 'ms': round((time.monotonic() - start) * 1000, 3)}
    if isinstance(payload, dict) and isinstance(payload.get('error'), str):
        result['error_code'] = payload['error']
    if path == '/system' and isinstance(payload, dict):
        result['services'] = {s['name']: s['status'] for s in payload.get('services', [])}
    return result, payload, session


def login(credentials):
    result, data, cookie = request('/auth/login', credentials=credentials, method='POST')
    if result['status'] != 200 or not cookie:
        raise RuntimeError('fixture login failed: ' + str(result['status']))
    return cookie, data['csrfToken']


def snapshot(prefix):
    owned(PREFIX + 'api')
    owned(PREFIX + 'redis')
    commands = {
        'heap.pb.gz': 'wget -qO- -T 8 "http://127.0.0.1:${PPROF_PORT}/debug/pprof/heap?gc=1"',
        'goroutines.txt': 'wget -qO- -T 8 "http://127.0.0.1:${PPROF_PORT}/debug/pprof/goroutine?debug=1"',
        'metrics.txt': 'wget -qO- -T 8 --header "Authorization: Bearer ${METRICS_SECRET}" http://127.0.0.1:8085/metrics',
    }
    for suffix, command in commands.items():
        Path(str(prefix) + '-' + suffix).write_bytes(docker('exec', PREFIX + 'api', 'sh', '-c', command).stdout)
    clients = docker('exec', PREFIX + 'redis', 'redis-cli', 'INFO', 'clients').stdout
    Path(str(prefix) + '-redis-clients.txt').write_bytes(clients)


def readiness():
    result = docker('exec', PREFIX + 'api', 'wget', '-S', '-O', '/dev/null', '-T', '4',
                    'http://127.0.0.1:8085/health/ready', check=False, timeout=8)
    statuses = re.findall(rb'HTTP/\S+\s+(\d+)', result.stderr)
    return int(statuses[-1]) if statuses else 'transport_error'


def outages(credentials, output, dependencies):
    for service in ('api', 'postgres', 'redis', 'rabbit'):
        owned(PREFIX + service)
    cookie, csrf = login(credentials)
    report = {}
    try:
        for dependency in dependencies:
            name = PREFIX + dependency
            print('Testing isolated outage:', dependency, flush=True)
            started = time.monotonic()
            try:
                docker('stop', '--time', '3', name, timeout=10)
                time.sleep(11)  # Let the fixed aggregate/probe cache expire.
                paths = ('/system', '/dashboard', '/users?pageSize=20', '/audit?pageSize=20')
                with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
                    samples = dict(zip(paths, pool.map(lambda p: request(p, cookie=cookie)[0], paths)))
                report[dependency] = {'during': samples, 'readiness_during': readiness()}
            finally:
                docker('start', name, timeout=15)
                report.setdefault(dependency, {})['stopped_seconds'] = round(time.monotonic() - started, 3)
            deadline = time.monotonic() + 35
            while time.monotonic() < deadline:
                ready = readiness()
                if ready == 200:
                    break
                time.sleep(1)
            report[dependency]['readiness_after'] = ready
            report[dependency]['api_after'] = request('/users?pageSize=20', cookie=cookie)[0]
            Path(output).write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
            print('Restored:', dependency, 'readiness:', ready, flush=True)
            if ready != 200:
                break
    finally:
        request('/auth/logout', cookie=cookie, csrf=csrf, method='POST')
    return report


def cancellation(credentials):
    """Cancel a real blocked read, holding a lock only in audit_preview."""
    owned(PREFIX + 'postgres')
    cookie, csrf = login(credentials)
    command = ['docker', 'exec', '-i', PREFIX + 'postgres', 'psql', '-X', '-qAt',
               '-U', 'postgres', '-d', 'audit_preview', '-v', 'ON_ERROR_STOP=1']
    holder = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    conn = connection()
    query = ("SELECT count(*) FROM pg_stat_activity WHERE datname='audit_preview' "
             "AND wait_event_type='Lock' AND query ILIKE '%users%'")
    def waiters():
        return int(subprocess.run(command + ['-c', query], capture_output=True, check=True, timeout=5).stdout)
    try:
        holder.stdin.write(b"SET lock_timeout='2s'; SET idle_in_transaction_session_timeout='15s'; BEGIN; LOCK users IN ACCESS EXCLUSIVE MODE; SELECT 'locked';\n")
        holder.stdin.flush()
        if not select.select([holder.stdout], [], [], 5)[0] or holder.stdout.readline().strip() != b'locked':
            raise RuntimeError('synthetic cancellation fixture did not acquire its lock')
        conn.request('GET', '/api/v1/admin/users?pageSize=20', headers={'Cookie': cookie})
        before = 0
        for _ in range(15):
            before = waiters()
            if before:
                break
            time.sleep(.1)
        started = time.monotonic()
        if conn.sock:
            conn.sock.shutdown(socket.SHUT_RDWR)
        conn.close()
        after = before
        for _ in range(20):
            after = waiters()
            if after == 0:
                break
            time.sleep(.1)
        result = {'lock_waiters_before_close': before, 'lock_waiters_after_close': after,
                  'waiter_release_ms': round((time.monotonic() - started) * 1000, 3),
                  'cancelled_active_query': before > 0 and after == 0}
    finally:
        conn.close()
        try:
            holder.communicate(b'ROLLBACK;\n\\q\n', timeout=5)
        except subprocess.TimeoutExpired:
            holder.kill()
            holder.communicate(timeout=5)
    result['recovery'] = request('/users?pageSize=20', cookie=cookie)[0]
    request('/auth/logout', cookie=cookie, csrf=csrf, method='POST')
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=('snapshot', 'login-burst', 'outages', 'cancellation'))
    parser.add_argument('--output', required=True)
    parser.add_argument('--allow-outages', action='store_true')
    parser.add_argument('--dependency', choices=('redis', 'rabbit', 'postgres'))
    args = parser.parse_args()
    owned(PREFIX + 'api')
    if args.mode == 'snapshot':
        snapshot(args.output)
        return
    credentials = json.load(sys.stdin)
    if args.mode == 'outages':
        if not args.allow_outages:
            parser.error('outages require --allow-outages after coordinating the test window')
        dependencies = [args.dependency] if args.dependency else ('redis', 'rabbit', 'postgres')
        result = outages(credentials, args.output, dependencies)
    elif args.mode == 'cancellation':
        result = cancellation(credentials)
    else:
        barrier = threading.Barrier(8)
        sessions = []
        def one(_):
            barrier.wait(timeout=5)
            result, data, cookie = request('/auth/login', credentials=credentials, method='POST')
            if cookie and data.get('csrfToken'):
                sessions.append((cookie, data['csrfToken']))
            return result
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            result = list(pool.map(one, range(8)))
        for cookie, csrf in sessions:
            request('/auth/logout', cookie=cookie, csrf=csrf, method='POST')
    credentials.clear()
    Path(args.output).write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(result, ensure_ascii=False))


if __name__ == '__main__':
    try:
        main()
    except (RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(str(error))
