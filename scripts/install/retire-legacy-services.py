#!/usr/bin/env python3
"""Drain and stop retired Compose services before installing their replacements.

Containers and volumes are retained. Only containers with both exact project and
service labels are selected; errors leave the deployment stopped before startup.
"""

import argparse
import json
import re
import subprocess
import sys
import time


LEGACY_SERVICES = {'worker': 8090, 'worker-debug': 8090, 'live-worker': 8093}
APPLICATION_SERVICES = ('api', 'api-debug', 'media-worker', 'record-worker',
                        'record-worker-debug', 'product-worker', 'frontend', *LEGACY_SERVICES)


def run(*args):
    """Capture only Docker diagnostics; never print environment or secret values."""
    return subprocess.run(args, check=True, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                          text=True).stdout.strip()


def retire(project, timeout=300):
    """Retire exact legacy containers in project after authenticated zero-active drain."""
    if not isinstance(project, str) or not re.fullmatch(r'[a-z0-9][a-z0-9_-]*', project):
        raise RuntimeError('Invalid Compose project name')
    for service, port in LEGACY_SERVICES.items():
        containers = run('docker', 'ps', '--all', '--quiet', '--no-trunc',
                         '--filter', 'label=com.docker.compose.project=' + project,
                         '--filter', 'label=com.docker.compose.service=' + service).splitlines()
        for container in containers:
            if not re.fullmatch(r'[a-f0-9]{64}', container):
                raise RuntimeError('Cannot identify a legacy container safely')
            details = json.loads(run('docker', 'inspect', container))
            if len(details) != 1 or details[0].get('Id') != container:
                raise RuntimeError('Ambiguous legacy container identity')
            labels = details[0].get('Config', {}).get('Labels', {}) or {}
            if (labels.get('com.docker.compose.project') != project
                    or labels.get('com.docker.compose.service') != service):
                raise RuntimeError('Legacy container labels changed; deployment aborted')
            state = details[0].get('State', {})
            if state.get('Paused') or state.get('Restarting') or state.get('Dead'):
                raise RuntimeError(service + ' requires operator inspection before replacement')
            if state.get('Running') is True:
                request = ('test -n "$METRICS_SECRET" && wget -qO- '
                           '--header="Authorization: Bearer $METRICS_SECRET" '
                           f'http://127.0.0.1:{port}/operations/drain')
                run('docker', 'exec', container, 'sh', '-ec',
                    request.replace('wget -qO-', 'wget -qO- --post-data=""'))
                deadline = time.monotonic() + timeout
                while True:
                    drained = json.loads(run('docker', 'exec', container, 'sh', '-ec', request))
                    if (drained.get('draining') is True and type(drained.get('active')) is int
                            and drained['active'] == 0):
                        break
                    if time.monotonic() >= deadline:
                        raise RuntimeError(service + ' still has active work; replacement was not started')
                    time.sleep(2)
                # Prevent an old recorder from returning after a Docker daemon restart.
                run('docker', 'update', '--restart=no', container)
                run('docker', 'stop', '--time', '45', container)
            elif state.get('Running') is False:
                run('docker', 'update', '--restart=no', container)
            else:
                raise RuntimeError('Unknown legacy container state')
            if run('docker', 'inspect', '--format', '{{.State.Running}}', container) != 'false':
                raise RuntimeError(service + ' did not stop; replacement was not started')
            print(service + ': drained/stopped; container retained with restart disabled')


def require_migration_quiescence(project):
    """Block the one-time destructive schema transition while an application is running.

    A missing ledger (including a fresh database) needs the same check. Once the
    exact cleanup migration is recorded, normal future release behavior resumes.
    No database content, container or volume is changed by this preflight.
    """
    if not isinstance(project, str) or not re.fullmatch(r'[a-z0-9][a-z0-9_-]*', project):
        raise RuntimeError('Invalid Compose project name')
    postgres = run('docker', 'ps', '--quiet', '--no-trunc',
                   '--filter', 'label=com.docker.compose.project=' + project,
                   '--filter', 'label=com.docker.compose.service=postgres').splitlines()
    if len(postgres) > 1:
        raise RuntimeError('Ambiguous PostgreSQL container; inspect the selected project')
    if postgres:
        container = postgres[0]
        if not re.fullmatch(r'[a-f0-9]{64}', container):
            raise RuntimeError('Cannot identify PostgreSQL safely')
        query = ('PGPASSWORD="$POSTGRES_PASSWORD" exec psql -XqAt -v ON_ERROR_STOP=1 '
                 '-U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$1"')
        ledger = run('docker', 'exec', container, 'sh', '-ec', query, 'sh',
                     "SELECT to_regclass('public.release_schema_migrations') IS NOT NULL")
        if ledger not in ('t', 'f'):
            raise RuntimeError('Cannot determine the migration ledger state')
        if ledger == 't':
            applied = run('docker', 'exec', container, 'sh', '-ec', query, 'sh',
                          "SELECT EXISTS (SELECT 1 FROM public.release_schema_migrations "
                          "WHERE name = '000037_remove_speech_features.up.sql')")
            if applied == 't':
                return
            if applied != 'f':
                raise RuntimeError('Cannot determine the speech cleanup migration state')
    for service in APPLICATION_SERVICES:
        running = run('docker', 'ps', '--quiet', '--no-trunc',
                      '--filter', 'label=com.docker.compose.project=' + project,
                      '--filter', 'label=com.docker.compose.service=' + service)
        if running:
            raise RuntimeError('Migration 000037 requires a maintenance window: freeze new work, '
                               'drain and stop ALL application services, and verify a database/object '
                               'backup before retrying. Still running: ' + service + '. '
                               'Old images cannot be rolled back after cleanup without restoring the backup.')


def main():
    """Accept an explicit project or obtain its name from a read-only Compose render."""
    parser = argparse.ArgumentParser(description=__doc__)
    selection = parser.add_mutually_exclusive_group(required=True)
    selection.add_argument('--project')
    selection.add_argument('--compose', nargs=argparse.REMAINDER)
    parser.add_argument('--check-migration', action='store_true')
    args = parser.parse_args()
    project = args.project
    if args.compose is not None:
        if args.compose[:2] != ['docker', 'compose']:
            raise RuntimeError('--compose requires docker compose; select a Docker context through DOCKER_CONTEXT for both commands')
        project = json.loads(run(*args.compose, 'config', '--format', 'json')).get('name', '')
    retire(project)
    if args.check_migration:
        require_migration_quiescence(project)


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, RuntimeError, subprocess.CalledProcessError) as error:
        message = str(error) if not isinstance(error, subprocess.CalledProcessError) else 'Docker command failed'
        print('Legacy service retirement stopped: ' + message, file=sys.stderr)
        sys.exit(1)
