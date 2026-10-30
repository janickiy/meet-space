"""Installer and upgrade regression checks; never deploy applications or contact Docker."""

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


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


wizard = load('wizard', 'scripts/install/wizard.py')
source = load('source_installer', 'scripts/install/from-source.py')
images = load('images_installer', 'scripts/install/from-images.py')
legacy = load('legacy_retirement', 'scripts/install/retire-legacy-services.py')
SERVICES = {'api', 'media-worker', 'record-worker', 'product-worker', 'frontend', 'minio',
            'postgres', 'redis', 'rabbitmq', 'coturn', 'proxy', 'prometheus'}


class ServiceInventoryTest(unittest.TestCase):
    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory(prefix='meetspace-inventory-')
        self.addCleanup(self.scratch.cleanup)
        self.root = Path(self.scratch.name)
        (self.root / 'config').mkdir()
        self.manifest = {'version': 'v0.0.0-test', 'deploymentLayout': 'hardened-shared-host',
                         'images': {name: 'sha256:' + '1' * 64 for name in SERVICES}}

    def test_inventories_match_source_and_release_layouts(self):
        self.assertEqual(source.SERVICES, SERVICES)
        wizard.validate_service_inventory(self.manifest)
        classic = {**self.manifest, 'deploymentLayout': 'classic',
                   'images': {**self.manifest['images'], 'grafana': 'sha256:' + '2' * 64}}
        wizard.validate_service_inventory(classic)
        for layout, expected in (('classic', SERVICES | {'grafana'}), ('hardened-shared-host', SERVICES)):
            result = subprocess.check_output(['bash', '-c', 'source "$1/scripts/release/common.sh"; '
                                              'select_release_services "$2"; printf "%s\\n" "${RELEASE_SERVICES[@]}"',
                                              'test', str(ROOT), layout], text=True)
            self.assertEqual(set(result.splitlines()), expected)

    def test_old_or_incomplete_package_is_not_relabelled(self):
        for old in ({name: ref for name, ref in self.manifest['images'].items() if name != 'record-worker'},
                    {**self.manifest['images'], 'worker': 'sha256:' + '3' * 64},
                    {**self.manifest['images'], 'live-worker': 'sha256:' + '4' * 64}):
            with self.assertRaisesRegex(RuntimeError, 'record-worker'):
                wizard.validate_service_inventory({**self.manifest, 'images': old})
        with self.assertRaises(RuntimeError):
            wizard.validate_service_inventory({**self.manifest, 'deploymentLayout': 'unexpected'})

    @unittest.skipUnless(shutil.which('jq'), 'jq is unavailable')
    def test_shell_inventory_check_rejects_extra_images(self):
        path = self.root / 'manifest.json'
        script = ('source "$1/scripts/release/common.sh"; select_release_services hardened-shared-host; '
                  'RELEASE_MANIFEST="$2"; verify_service_inventory')
        path.write_text(json.dumps(self.manifest))
        good = subprocess.run(['bash', '-c', script, 'test', str(ROOT), str(path)], capture_output=True)
        self.assertEqual(good.returncode, 0, good.stderr)
        path.write_text(json.dumps({**self.manifest, 'images': {**self.manifest['images'], 'worker': 'old'}}))
        bad = subprocess.run(['bash', '-c', script, 'test', str(ROOT), str(path)], capture_output=True)
        self.assertNotEqual(bad.returncode, 0)
        self.assertIn(b'record-worker', bad.stderr)

    def test_examples_have_no_removed_provider_configuration(self):
        for filename in ('.env.example', '.env.production.example', '.env.staging.example'):
            keys = [line.split('=', 1)[0] for line in (ROOT / filename).read_text().splitlines() if '=' in line]
            for key in keys:
                self.assertFalse(key.startswith(('LIVE_', 'TRANSCRIPTION_', 'STT_', 'AI_', 'EMBEDDING')),
                                 (filename, key))
                self.assertNotIn(key, ('MEETING_ANALYTICS_ENABLED', 'PRODUCT_TEMP_PATH',
                                      'PRODUCT_MAX_REPROCESS', 'PRODUCT_REPROCESS_COOLDOWN',
                                      'WORKER_INTERNAL_URL', 'WORKER_INTERNAL_SECRET',
                                      'WEBRTC_UDP_PORT', 'WEBRTC_TCP_PORT', 'WEBRTC_NAT_IPS'))
            self.assertIn('WEBRTC_ICE_SERVERS_JSON', keys)
        self.assertNotIn('ffmpeg', (ROOT / 'dockers/product-worker/Dockerfile').read_text().lower())
        self.assertIn('./cmd/record-worker', (ROOT / 'dockers/record-worker/Dockerfile').read_text())
        self.assertFalse((ROOT / 'dockers/live-worker/Dockerfile').exists())
        self.assertFalse((ROOT / 'docker-compose.vector.yml').exists())
        self.assertIn('scripts/install/retire-legacy-services.py', source.DEPLOYMENT_FILES)

    def test_monitoring_is_idempotent_and_keeps_operator_mounts(self):
        path = self.root / 'config/compose.json'
        path.write_text(json.dumps({'services': {'api': {'labels': {'fixture': 'keep'}},
                                                'prometheus': {'volumes': ['metrics-data:/prometheus']}}}))
        for _ in range(2):
            wizard.configure_monitoring(self.root)
        overlay = json.loads(path.read_text())
        self.assertEqual(overlay['services']['api']['labels'], {'fixture': 'keep'})
        self.assertEqual(len(overlay['services']['prometheus']['volumes']), 2)
        config = json.loads((self.root / 'config/prometheus.json').read_text())
        targets = {host for item in config['scrape_configs'][0]['static_configs'] for host in item['targets']}
        self.assertEqual(targets, {'api:8085', 'media-worker:8091', 'record-worker:8090', 'product-worker:8092'})
        self.assertNotIn('--profile', wizard.compose_args(self.root, self.manifest, {'project': 'meetspace-test'}))

    def test_saved_manager_is_current_private_and_not_redirected(self):
        wizard.save_installer_runtime(self.root)
        saved = self.root / 'config/installer.py'
        self.assertEqual(saved.read_bytes(), (ROOT / 'scripts/install/wizard.py').read_bytes())
        self.assertEqual(saved.stat().st_mode & 0o777, 0o600)
        self.assertIn(str(saved), (self.root / 'manage.sh').read_text())
        saved.unlink()
        saved.symlink_to(ROOT / 'scripts/install/wizard.py')
        with self.assertRaises(RuntimeError):
            wizard.save_installer_runtime(self.root)

    def test_configuration_writes_reject_links_and_keep_bind_inode(self):
        target = self.root / 'config/generated.json'
        wizard.update_owned_configuration(target, '{}\n', 0o644)
        inode = target.stat().st_ino
        wizard.update_owned_configuration(target, '[]\n', 0o644)
        self.assertEqual(target.stat().st_ino, inode)
        linked = self.root / 'config/symlink.json'
        linked.symlink_to(target)
        with self.assertRaises(OSError):
            wizard.update_owned_configuration(linked, 'do not write')
        os.link(target, self.root / 'config/hardlink.json')
        with self.assertRaises(RuntimeError):
            wizard.update_owned_configuration(target, 'do not write')
        self.assertEqual(target.read_text(), '[]\n')

    def test_profiles_cannot_be_injected_by_parent_environment(self):
        with mock.patch.dict(os.environ, {'COMPOSE_PROFILES': 'unexpected'}), mock.patch.object(wizard.subprocess, 'run') as run:
            wizard.run('docker', 'compose', 'config', '--quiet')
            self.assertEqual(run.call_args.kwargs['env']['COMPOSE_PROFILES'], '')

    @unittest.skipUnless(shutil.which('docker'), 'Docker Compose CLI is unavailable')
    def test_fresh_configuration_renders_without_mutating_release(self):
        for directory in ('secrets', 'storage', 'bundle/deployment'):
            (self.root / directory).mkdir(parents=True, exist_ok=True)
        deployment = self.root / 'bundle/deployment'
        for name in ('docker-compose.production.yml', 'docker-compose.hardened.yml', '.env.production.example'):
            shutil.copyfile(ROOT / name, deployment / name)
        original = {path.name: path.read_bytes() for path in deployment.iterdir()}
        state = {'root': str(self.root), 'domain': 'fixture.invalid', 'ip': '192.0.2.1',
                 'project': 'meetspace-test', 'subnet': '172.30.240.0/24', 'token': 'fixture'}
        original_run = wizard.run
        def render(*args, **kwargs):
            if args[:2] == ('docker', 'info'):
                return json.dumps({'NCPU': 2})
            self.assertEqual(args[:2], ('docker', 'compose'))
            self.assertIn('config', args)
            return original_run(*args, **kwargs)
        with mock.patch.object(wizard, 'run', side_effect=render):
            wizard.make_configuration(self.root, self.manifest, state)
            actual = wizard.run(*wizard.compose_args(self.root, self.manifest, state), '--profile', '*',
                                'config', '--services', capture=True)
        self.assertEqual(set(actual.splitlines()), SERVICES | {'migrate'})
        for name, data in original.items():
            self.assertEqual((deployment / name).read_bytes(), data)
        env = (self.root / 'config/meetspace.env').read_text()
        self.assertIn('RECORD_WORKER_IMAGE=', env)
        self.assertNotIn('LIVE_', env)
        keys = {line.split('=', 1)[0] for line in env.splitlines() if '=' in line}
        self.assertTrue({'MEDIA_INTERNAL_SECRET', 'METRICS_SECRET', 'WEBRTC_ICE_SERVERS_JSON'} <= keys)
        self.assertTrue(keys.isdisjoint({'WORKER_INTERNAL_URL', 'WORKER_INTERNAL_SECRET',
                                        'WEBRTC_UDP_PORT', 'WEBRTC_TCP_PORT', 'WEBRTC_NAT_IPS'}))

    @unittest.skipUnless(shutil.which('docker'), 'Docker Compose CLI is unavailable')
    def test_local_and_production_compose_inventories(self):
        # Render copied templates only: do not read the operator's .env or contact a daemon.
        for path in ROOT.glob('docker-compose*.yml'):
            shutil.copyfile(path, self.root / path.name)
        environment = self.root / '.env'
        values = {'RELEASE_ENV_FILE': str(environment), 'RECORDER_STORAGE_DIR': str(self.root / 'storage'),
                  'TLS_CERT_DIR': str(self.root / 'certs'), 'TURN_TLS_CERT_DIR': str(self.root / 'certs'),
                  'PUBLIC_FRONTEND_URL': 'https://fixture.invalid', 'MINIO_PUBLIC_ENDPOINT': 'https://fixture.invalid',
                  'POSTGRES_USER': 'fixture', 'POSTGRES_PASSWORD': 'synthetic-fixture',
                  'REDIS_PASSWORD': 'synthetic-fixture', 'RABBIT_MQ_USER': 'fixture',
                  'RABBIT_MQ_PASSWORD': 'synthetic-fixture', 'MINIO_ROOT_USER': 'fixture',
                  'MINIO_ROOT_PASSWORD': 'synthetic-fixture'}
        values.update({name.upper().replace('-', '_') + '_IMAGE': 'fixture/image@sha256:' + '1' * 64
                       for name in SERVICES | {'grafana'}})
        values['OBJECT_STORAGE_IMAGE'] = values['MINIO_IMAGE']
        environment.write_text(''.join(f'{key}={value}\n' for key, value in values.items()))
        env = {key: value for key, value in os.environ.items() if key in ('PATH', 'HOME', 'LANG', 'LC_ALL')}
        variants = (
            ('docker-compose.yml',),
            ('docker-compose.yml', 'docker-compose.smtp.yml'),
            ('docker-compose.yml', 'docker-compose.media-range.yml'),
            ('docker-compose.yml', 'docker-compose.media-udp.yml'),
            ('docker-compose.production.yml',),
            ('docker-compose.production.yml', 'docker-compose.grafana.yml'),
            ('docker-compose.production.yml', 'docker-compose.hardened.yml', 'docker-compose.shared-host.yml'),
            ('docker-compose.production.yml', 'docker-compose.grafana.yml', 'docker-compose.release-local.yml',
             'docker-compose.release-local-grafana.yml'),
            ('docker-compose.production.yml', 'docker-compose.hardened.yml', 'docker-compose.release-local.yml',
             'docker-compose.release-local-hardened.yml'),
        )
        # SMTP's private env_file is a fixture too, not an operator credential file.
        (self.root / '.env.smtp').write_text('SMTP_HOST=smtp.fixture.invalid\n')
        values['SMTP_ENV_FILE'] = str(self.root / '.env.smtp')
        environment.write_text(''.join(f'{key}={value}\n' for key, value in values.items()))
        for variant in variants:
            args = ['docker', 'compose', '--project-name', 'meetspace-test', '--env-file', str(environment),
                    *[argument for filename in variant for argument in ('-f', str(self.root / filename))],
                    '--profile', '*', 'config', '--format', 'json']
            config = json.loads(subprocess.check_output(args, env=env, text=True))
            self.assertIn('record-worker', config['services'])
            self.assertNotIn('worker', config['services'])
            self.assertNotIn('live-worker', config['services'])
            for service in ('record-worker', 'record-worker-debug'):
                for port in config['services'].get(service, {}).get('ports', []):
                    self.assertIn(port['target'], (8090, 2346))
                    self.assertEqual(port.get('host_ip'), '127.0.0.1')
            if variant[0] == 'docker-compose.production.yml':
                self.assertEqual(config['services']['record-worker']['entrypoint'], ['/app/record-worker'])

    def test_old_prebuilt_index_fails_before_archive_download(self):
        with self.assertRaisesRegex(RuntimeError, 'record-worker'):
            images.select_archive({'schemaVersion': 1}, 'v1.0.0', 'linux/amd64')

    def test_make_startup_and_explicit_migration_paths_are_guarded(self):
        makefile = (ROOT / 'Makefile').read_text()
        for target in ('up', 'restart', 'migrate-up', 'debug-api', 'debug-record-worker', 'debug-both'):
            body = makefile.split('\n' + target + ':\n', 1)[1].split('\n\n', 1)[0]
            self.assertIn('retire-legacy-services.py --check-migration', body, target)
            commands = body.splitlines()
            guard = next(i for i, line in enumerate(commands) if 'retire-legacy-services.py' in line)
            self.assertFalse(any(' up ' in line or ' go run ' in line for line in commands[:guard]), target)

    def test_latest_skips_incompatible_indexes_without_download(self):
        release = {'tag_name': 'v1.0.0', 'published_at': '2026-10-20T00:00:00Z'}
        assets = {'prebuilt.json': {'browser_download_url': 'https://fixture.invalid/prebuilt.json'}}
        with mock.patch.object(images, 'read_json', side_effect=[[release], {'schemaVersion': 1}]), \
                mock.patch.object(images, 'release_assets', return_value=assets), \
                mock.patch.object(images, 'download') as download:
            with self.assertRaisesRegex(RuntimeError, 'совместимого'):
                images.resolve_release('latest', 'linux/amd64')
            download.assert_not_called()

    @unittest.skipUnless(shutil.which('node'), 'Node is unavailable')
    def test_archive_tool_rejects_legacy_before_creating_output(self):
        package = self.root / 'package'
        package.mkdir()
        (package / 'release.json').write_text(json.dumps({
            'artifactMode': 'archive', 'securityScanned': True, 'sourceDirty': False,
            'sourceFingerprintVerified': True, 'images': {'worker': 'old'}}))
        destination = self.root / 'must-not-exist'
        result = subprocess.run(['node', str(ROOT / 'scripts/release/prepare-archive.mjs'),
                                 str(package), str(destination)], capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Incompatible inventory', result.stderr)
        self.assertFalse(destination.exists())


class LegacyRetirementTest(unittest.TestCase):
    def fake_docker(self, *, active=0, running=True, wrong_labels=False, drain_error=False):
        self.calls = []
        container = 'a' * 64
        def run(*args):
            self.calls.append(args)
            if args[1] == 'ps':
                return container if args[-1].endswith('=worker') else ''
            if args[1] == 'inspect':
                if '--format' in args:
                    return 'false'
                return json.dumps([{'Id': container, 'State': {'Running': running},
                                    'Config': {'Labels': {'com.docker.compose.project': 'other' if wrong_labels else 'meetspace-test',
                                                          'com.docker.compose.service': 'worker'}}}])
            if args[1] == 'exec':
                if drain_error:
                    raise subprocess.CalledProcessError(1, ['docker'])
                return json.dumps({'draining': True, 'active': active})
            return ''
        return run

    def test_old_recorder_is_drained_then_disabled_and_stopped(self):
        with mock.patch.object(legacy, 'run', side_effect=self.fake_docker()):
            legacy.retire('meetspace-test')
        actions = [call[1] for call in self.calls]
        self.assertLess(actions.index('exec'), actions.index('update'))
        self.assertLess(actions.index('update'), actions.index('stop'))
        self.assertEqual(actions.count('exec'), 2)
        self.assertTrue(any('--restart=no' in call for call in self.calls))
        self.assertFalse(any('rm' in call or 'kill' in call for call in self.calls))

    def test_active_or_failed_drain_never_stops_or_changes_restart_policy(self):
        for settings in ({'active': 1}, {'drain_error': True}):
            with mock.patch.object(legacy, 'run', side_effect=self.fake_docker(**settings)), \
                    mock.patch.object(legacy.time, 'monotonic', side_effect=[0, 301]):
                with self.assertRaises((RuntimeError, subprocess.CalledProcessError)):
                    legacy.retire('meetspace-test')
            self.assertFalse(any(call[1] in ('stop', 'update') for call in self.calls))

    def test_foreign_labels_fail_closed(self):
        with mock.patch.object(legacy, 'run', side_effect=self.fake_docker(wrong_labels=True)):
            with self.assertRaises(RuntimeError):
                legacy.retire('meetspace-test')
        self.assertFalse(any(call[1] in ('stop', 'exec', 'update') for call in self.calls))

    def test_stopped_legacy_container_stays_stopped_after_daemon_restart(self):
        with mock.patch.object(legacy, 'run', side_effect=self.fake_docker(running=False)):
            legacy.retire('meetspace-test')
        self.assertTrue(any(call[1] == 'update' for call in self.calls))
        self.assertFalse(any(call[1] in ('exec', 'stop') for call in self.calls))

    def migration_docker(self, *, applied=False, active_service=None):
        def run(*args):
            if args[1] == 'ps':
                service = args[-1].split('=')[-1]
                return 'b' * 64 if service in ('postgres', active_service) else ''
            if 'to_regclass' in args[-1]:
                return 't'
            if '000037_remove_speech_features.up.sql' in args[-1]:
                return 't' if applied else 'f'
            raise AssertionError(args)
        return run

    def test_pending_cleanup_requires_all_applications_stopped(self):
        for service in legacy.APPLICATION_SERVICES:
            with mock.patch.object(legacy, 'run', side_effect=self.migration_docker(active_service=service)):
                with self.assertRaisesRegex(RuntimeError, 'maintenance window'):
                    legacy.require_migration_quiescence('meetspace-test')

    def test_applied_cleanup_does_not_block_later_normal_upgrades(self):
        with mock.patch.object(legacy, 'run', side_effect=self.migration_docker(applied=True, active_service='api')):
            legacy.require_migration_quiescence('meetspace-test')

    def test_stopped_applications_allow_pending_cleanup(self):
        with mock.patch.object(legacy, 'run', side_effect=self.migration_docker()):
            legacy.require_migration_quiescence('meetspace-test')

    def test_missing_database_or_ledger_is_safe_only_with_stopped_apps(self):
        for database in (False, True):
            for active in (False, True):
                def run(*args):
                    if args[1] == 'ps':
                        if args[-1].endswith('=postgres') and database:
                            return 'b' * 64
                        return 'a' * 64 if args[-1].endswith('=api') and active else ''
                    return 'f'  # A running PostgreSQL has no ledger yet.
                with mock.patch.object(legacy, 'run', side_effect=run):
                    if active:
                        with self.assertRaisesRegex(RuntimeError, 'maintenance window'):
                            legacy.require_migration_quiescence('meetspace-test')
                    else:
                        legacy.require_migration_quiescence('meetspace-test')

    def test_ambiguous_or_failed_database_inspection_blocks_transition(self):
        with mock.patch.object(legacy, 'run', return_value='a' * 64 + '\n' + 'b' * 64):
            with self.assertRaisesRegex(RuntimeError, 'Ambiguous'):
                legacy.require_migration_quiescence('meetspace-test')
        with mock.patch.object(legacy, 'run', side_effect=['b' * 64, 'unexpected']):
            with self.assertRaisesRegex(RuntimeError, 'ledger'):
                legacy.require_migration_quiescence('meetspace-test')
        with mock.patch.object(legacy, 'run', side_effect=['b' * 64, subprocess.CalledProcessError(1, ['docker'])]):
            with self.assertRaises(subprocess.CalledProcessError):
                legacy.require_migration_quiescence('meetspace-test')


if __name__ == '__main__':
    unittest.main()
