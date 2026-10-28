"""Regression checks for opt-in live processing; never deploy real applications."""

import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('meetspace_wizard', ROOT / 'scripts/install/wizard.py')
wizard = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(wizard)


class LiveWorkerPolicyTest(unittest.TestCase):
    """Exercise saved configuration using only disposable local fixture files."""

    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory(prefix='meetspace-live-policy-')
        self.addCleanup(self.scratch.cleanup)
        self.root = Path(self.scratch.name)
        (self.root / 'config').mkdir()
        self.env = self.root / 'config/meetspace.env'
        self.env.write_text('LIVE_STT_ENABLED=false\nMEETING_ANALYTICS_ENABLED=false\n')
        (self.root / 'config/compose.json').write_text(json.dumps({'services': {
            'api': {'labels': {'fixture': 'keep'}}, 'live-worker': {'labels': {'fixture': 'keep'}},
            'prometheus': {'volumes': ['metrics-data:/prometheus']}}}))

    def test_default_and_boolean_flags(self):
        for example in ('.env.example', '.env.production.example', '.env.staging.example'):
            self.assertFalse(wizard.live_worker_enabled(ROOT / example), example)
        for flag in ('LIVE_STT_ENABLED', 'MEETING_ANALYTICS_ENABLED'):
            for value in ('true', 'True', 'TRUE', 't', 'T', '1', '"true"', "'true' # enabled"):
                self.env.write_text(f'export {flag}={value}\nUNRELATED_SECRET=private\n')
                self.assertTrue(wizard.live_worker_enabled(self.env), (flag, value))
        self.env.write_text('# disabled\nLIVE_STT_ENABLED=\nMEETING_ANALYTICS_ENABLED="false" # off\n')
        self.assertFalse(wizard.live_worker_enabled(self.env))

    def test_invalid_flags_do_not_execute_or_leak(self):
        for value in ('$(touch /not-executed)', '${OTHER_FLAG}', 'yes', 'true extra', 'private-secret'):
            self.env.write_text(f'LIVE_STT_ENABLED={value}\n')
            with self.assertRaises(RuntimeError) as result:
                wizard.live_worker_enabled(self.env)
            self.assertNotIn(value, str(result.exception))

    def test_policy_cli_and_release_runtime_selection(self):
        for enabled in (False, True):
            self.env.write_text(f'LIVE_STT_ENABLED={str(enabled).lower()}\n')
            output = subprocess.check_output(['bash', '-c', '''
set -euo pipefail
source "$1/scripts/release/common.sh"
RELEASE_ENV_FILE="$2"
configure_live_worker
printf '%s\n' "$RELEASE_LIVE_WORKER" "${RELEASE_APPLICATIONS[*]}" "${RELEASE_SERVICES[*]}" "$LIVE_METRICS_TARGETS_FILE"
''', 'test', str(ROOT), str(self.env)], text=True).splitlines()
            self.assertEqual(output[0], str(enabled).lower())
            self.assertEqual('live-worker' in output[1].split(), enabled)
            self.assertIn('live-worker', output[2].split(), 'Optional image still needs integrity/security verification')
            self.assertTrue(output[3].endswith('enabled.json' if enabled else 'disabled.json'))

    def test_compose_profile_and_old_bundle_overlay(self):
        manifest = {'deploymentLayout': 'hardened-shared-host'}
        state = {'project': 'meetspace-test'}
        for enabled in (False, True, False):
            self.env.write_text(f'MEETING_ANALYTICS_ENABLED={str(enabled).lower()}\n')
            self.assertEqual(wizard.configure_optional_services(self.root), enabled)
            overlay = json.loads((self.root / 'config/compose.json').read_text())
            self.assertEqual(overlay['services']['live-worker']['profiles'], ['live'])
            self.assertEqual(overlay['services']['api']['labels'], {'fixture': 'keep'})
            self.assertEqual(len(overlay['services']['prometheus']['volumes']), 2, 'Repeated resume must not duplicate config mounts')
            configuration = json.loads((self.root / 'config/prometheus.json').read_text())
            targets = [host for item in configuration['scrape_configs'][0]['static_configs'] for host in item['targets']]
            self.assertEqual('live-worker:8093' in targets, enabled)
            self.assertEqual('--profile' in wizard.compose_args(self.root, manifest, state), enabled)

    def test_saved_manager_does_not_revert_to_old_bundle_wizard(self):
        wizard.save_installer_runtime(self.root)
        saved = self.root / 'config/installer.py'
        self.assertEqual(saved.read_bytes(), (ROOT / 'scripts/install/wizard.py').read_bytes())
        self.assertEqual(saved.stat().st_mode & 0o777, 0o600)
        self.assertIn(str(saved), (self.root / 'manage.sh').read_text())

    def test_docker_environment_cannot_silently_enable_profile(self):
        with mock.patch.dict(os.environ, {'COMPOSE_PROFILES': 'live'}), mock.patch.object(wizard.subprocess, 'run') as run:
            wizard.run('docker', 'compose', 'config', '--quiet')
            self.assertEqual(run.call_args.kwargs['env']['COMPOSE_PROFILES'], '')

    def test_configuration_writes_reject_links_and_keep_bind_inode(self):
        target = self.root / 'config/generated.json'
        wizard.update_owned_configuration(target, '{}\n', 0o644)
        inode = target.stat().st_ino
        wizard.update_owned_configuration(target, '[]\n', 0o644)
        self.assertEqual(target.stat().st_ino, inode)
        self.assertEqual(target.read_text(), '[]\n')
        symlink = self.root / 'config/symlink.json'
        symlink.symlink_to(target)
        with self.assertRaises(OSError):
            wizard.update_owned_configuration(symlink, 'do not write')
        linked = self.root / 'config/hardlink.json'
        os.link(target, linked)
        with self.assertRaises(RuntimeError):
            wizard.update_owned_configuration(linked, 'do not write')
        self.assertEqual(target.read_text(), '[]\n')

    @unittest.skipUnless(shutil.which('docker'), 'Docker Compose CLI is unavailable')
    def test_fresh_configuration_and_old_bundle_render_without_live(self):
        fresh = self.root / 'fresh'
        for directory in ('config', 'secrets', 'storage', 'bundle/deployment'):
            (fresh / directory).mkdir(parents=True, exist_ok=True)
        deployment = fresh / 'bundle/deployment'
        # Simulate a verified legacy package that originally started live-worker
        # unconditionally, even if its example happened to enable live STT.
        for name in ('docker-compose.production.yml', 'docker-compose.hardened.yml', '.env.production.example'):
            source = (ROOT / name).read_text().replace('    profiles: [live]\n', '')
            if name.startswith('.env'):
                source = source.replace('LIVE_STT_ENABLED=false', 'LIVE_STT_ENABLED=true')
            (deployment / name).write_text(source)
        original = {name: path.read_bytes() for name, path in ((item.name, item) for item in deployment.iterdir())}
        services = ('api', 'media-worker', 'worker', 'product-worker', 'live-worker', 'frontend',
                    'minio', 'postgres', 'redis', 'rabbitmq', 'coturn', 'proxy', 'prometheus')
        manifest = {'version': 'v0.0.0-live-test', 'deploymentLayout': 'hardened-shared-host',
                    'images': {service: 'sha256:' + '1' * 64 for service in services}}
        state = {'root': str(fresh), 'domain': 'fixture.invalid', 'ip': '192.0.2.1',
                 'project': 'meetspace-test', 'subnet': '172.30.240.0/24', 'token': 'fixture'}
        original_run = wizard.run
        def run(*args, **kwargs):
            if args[:2] == ('docker', 'info'):
                return json.dumps({'NCPU': 2})
            return original_run(*args, **kwargs)
        with mock.patch.object(wizard, 'run', side_effect=run):
            wizard.make_configuration(fresh, manifest, state)
        self.assertFalse(wizard.live_worker_enabled(fresh / 'config/meetspace.env'))
        default = wizard.run(*wizard.compose_args(fresh, manifest, state), 'config', '--services', capture=True)
        self.assertNotIn('live-worker', default.splitlines())
        environment = fresh / 'config/meetspace.env'
        environment.write_text(environment.read_text().replace('MEETING_ANALYTICS_ENABLED=false', 'MEETING_ANALYTICS_ENABLED=true')
                               + '\nCOMPOSE_PROFILES=live\n')
        wizard.configure_optional_services(fresh)
        enabled = wizard.run(*wizard.compose_args(fresh, manifest, state), 'config', '--services', capture=True)
        self.assertIn('live-worker', enabled.splitlines())
        environment.write_text(environment.read_text().replace('MEETING_ANALYTICS_ENABLED=true', 'MEETING_ANALYTICS_ENABLED=false'))
        wizard.configure_optional_services(fresh)
        default = wizard.run(*wizard.compose_args(fresh, manifest, state), 'config', '--services', capture=True)
        self.assertNotIn('live-worker', default.splitlines(), 'Saved COMPOSE_PROFILES must not bypass feature flags')
        for name, data in original.items():
            self.assertEqual((deployment / name).read_bytes(), data, 'Immutable deployment files must not change')

    def test_stop_waits_for_drain(self):
        calls = []
        def run(*args, **kwargs):
            calls.append(args)
            if 'ps' in args:
                return 'a' * 64
            if 'inspect' in args:
                return 'true'
            if 'exec' in args:
                return json.dumps({'draining': True, 'active': 0})
        with mock.patch.object(wizard, 'run', side_effect=run):
            wizard.stop_live_worker(['docker', 'compose'])
        self.assertIn('stop', calls[-1])
        self.assertEqual(sum('exec' in call for call in calls), 2)

    def test_stop_never_forces_an_active_worker(self):
        calls = []
        def run(*args, **kwargs):
            calls.append(args)
            if 'ps' in args:
                return 'a' * 64
            if 'inspect' in args:
                return 'true'
            return json.dumps({'draining': True, 'active': 1})
        with mock.patch.object(wizard, 'run', side_effect=run), \
                mock.patch.object(wizard.time, 'monotonic', side_effect=[0, 0, 301]), \
                mock.patch.object(wizard.time, 'sleep'):
            with self.assertRaises(RuntimeError):
                wizard.stop_live_worker(['docker', 'compose'])
        self.assertFalse(any('stop' in call or 'kill' in call for call in calls))

    def test_release_disabled_worker_is_drained_before_stop(self):
        output = subprocess.check_output(['bash', '-c', '''
set -euo pipefail
source "$1/scripts/release/common.sh"
compose() { if [[ "$*" == *" ps "* ]]; then printf '%s' fixture; else printf 'compose:%s\n' "$*"; fi; }
docker() { printf true; }
drain_service() { printf 'drain:%s\n' "$*"; }
stop_disabled_live_worker
''', 'test', str(ROOT)], text=True).splitlines()
        self.assertEqual(output, ['drain:live-worker 8093', 'compose:--profile live stop live-worker'])

    @unittest.skipUnless(shutil.which('docker'), 'Docker Compose CLI is unavailable')
    def test_real_compose_excludes_live_by_default(self):
        # config only: never connect to the daemon, start containers or load the operator's .env.
        temporary = self.root / 'compose'
        temporary.mkdir()
        for filename in ('docker-compose.yml', 'docker-compose.production.yml', 'docker-compose.hardened.yml'):
            shutil.copyfile(ROOT / filename, temporary / filename)
        env = dict(os.environ)
        for key in list(env):
            if key not in ('PATH', 'HOME', 'LANG', 'LC_ALL', 'DOCKER_CONFIG'):
                env.pop(key)
        environment = temporary / '.env'
        values = {'RELEASE_ENV_FILE': str(environment), 'RECORDER_STORAGE_DIR': str(temporary / 'storage'),
                  'TLS_CERT_DIR': str(temporary / 'certs'), 'TURN_TLS_CERT_DIR': str(temporary / 'certs'),
                  'PUBLIC_FRONTEND_URL': 'https://fixture.invalid', 'MINIO_PUBLIC_ENDPOINT': 'https://fixture.invalid',
                  'POSTGRES_USER': 'fixture', 'POSTGRES_PASSWORD': 'synthetic-fixture',
                  'REDIS_PASSWORD': 'synthetic-fixture', 'RABBIT_MQ_USER': 'fixture',
                  'RABBIT_MQ_PASSWORD': 'synthetic-fixture', 'MINIO_ROOT_USER': 'fixture',
                  'MINIO_ROOT_PASSWORD': 'synthetic-fixture', 'LIVE_STT_ENABLED': 'false',
                  'MEETING_ANALYTICS_ENABLED': 'false'}
        for service in ('api', 'media-worker', 'worker', 'product-worker', 'live-worker', 'frontend',
                        'minio', 'postgres', 'redis', 'rabbitmq', 'coturn', 'proxy', 'prometheus'):
            values[service.upper().replace('-', '_') + '_IMAGE'] = 'fixture/image@sha256:' + '1' * 64
        values['OBJECT_STORAGE_IMAGE'] = values['MINIO_IMAGE']
        environment.write_text(''.join(f'{key}={value}\n' for key, value in values.items()))
        for compose in ('docker-compose.yml', 'docker-compose.production.yml'):
            args = ['docker', 'compose', '--project-name', 'meetspace-test', '--env-file', str(environment), '-f', str(temporary / compose)]
            default = subprocess.check_output([*args, 'config', '--services'], env=env, text=True)
            enabled = subprocess.check_output([*args, '--profile', 'live', 'config', '--services'], env=env, text=True)
            self.assertNotIn('live-worker', default.splitlines())
            self.assertIn('live-worker', enabled.splitlines())


if __name__ == '__main__':
    unittest.main()
