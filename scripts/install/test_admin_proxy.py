"""Opt-in isolated Nginx tests: RUN_ADMIN_PROXY_TESTS=1 python3 -m unittest discover -s scripts/install.

Uses an already present nginx:1.30.6-alpine image, ephemeral certificates and owned
containers; publishes no host ports and never reads the installation environment.
"""
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import time
import unittest
import uuid


ROOT = Path(__file__).resolve().parents[2]


@unittest.skipUnless(os.environ.get('RUN_ADMIN_PROXY_TESTS') == '1', 'opt-in Docker proxy test')
class AdminProxyTest(unittest.TestCase):
    def setUp(self):
        if not shutil.which('docker') or not shutil.which('openssl'):
            self.skipTest('docker and openssl are required')
        self.name = 'meetspace-admin-test-' + uuid.uuid4().hex[:12]
        self.scratch = tempfile.TemporaryDirectory(prefix='meetspace-admin-proxy-')
        self.addCleanup(self.scratch.cleanup)
        self.addCleanup(lambda: subprocess.run(['docker', 'rm', '-f', self.name],
                                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL))
        self.path = Path(self.scratch.name)
        subprocess.run(['openssl', 'req', '-x509', '-nodes', '-newkey', 'rsa:2048', '-days', '1',
                        '-subj', '/CN=localhost', '-keyout', str(self.path / 'privkey.pem'),
                        '-out', str(self.path / 'fullchain.pem')], check=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        (self.path / 'index.html').write_text('<h1>Admin fixture</h1>')

    def start_proxy(self, enabled):
        mounts = [
            (ROOT / 'dockers/production/nginx.conf', '/etc/nginx/conf.d/default.conf'),
            (self.path, '/run/certs'),
        ]
        packaged = os.environ.get('ADMIN_PROXY_TEST_IMAGE')
        if not packaged:
            mounts.extend([
            (ROOT / 'dockers/production/admin.conf.template', '/etc/meetspace/admin.conf.template'),
            (ROOT / 'dockers/production/25-admin-config.sh', '/docker-entrypoint.d/25-admin-config.sh'),
            (self.path / 'index.html', '/usr/share/nginx/admin/index.html'),
            ])
        args = ['docker', 'run', '-d', '--name', self.name, '--add-host', 'minio:127.0.0.1',
                '-e', 'ADMIN_ENABLED=' + enabled]
        for source, target in mounts:
            args.extend(['-v', f'{source}:{target}:ro'])
        args.append(packaged or 'nginx:1.30.6-alpine')
        subprocess.run(args, check=True, stdout=subprocess.DEVNULL)
        for _ in range(30):
            result = self.http_status(443, '/api/v1/admin/auth/me')
            if result == 404:
                return
            time.sleep(.1)
        logs = subprocess.run(['docker', 'logs', self.name], capture_output=True, text=True)
        self.fail('Nginx did not become ready: ' + logs.stdout + logs.stderr)

    def http_status(self, port, path):
        result = subprocess.run(['docker', 'exec', self.name, 'wget', '--no-check-certificate',
                                 '--server-response', '-T', '2', '-O', '/dev/null',
                                 f'https://127.0.0.1:{port}{path}'], capture_output=True, text=True)
        statuses = re.findall(r'HTTP/\d(?:\.\d)? (\d{3})', result.stderr)
        return int(statuses[-1]) if statuses else None

    def test_enabled_admin_is_separate_from_public_api(self):
        self.start_proxy('true')
        self.assertEqual(self.http_status(8081, '/'), 200)
        self.assertEqual(self.http_status(8081, '/users'), 200)
        self.assertEqual(self.http_status(8081, '/api/v1/auth/me'), 404)
        self.assertEqual(self.http_status(443, '/api/v1/admin'), 404)
        self.assertEqual(self.http_status(443, '/api/v1/admin/auth/me'), 404)
        for path in ['/api/v1/%61dmin/auth/me', '/api%2fv1%2fadmin%2fauth%2fme',
                     '/api/v1/admin%2fauth%2fme', '//api/v1//admin//auth/me',
                     '/api/v1/x/../admin/auth/me']:
            with self.subTest(path=path):
                self.assertEqual(self.http_status(443, path), 404)
        if os.environ.get('ADMIN_PROXY_TEST_IMAGE'):
            html = subprocess.check_output(['docker', 'exec', self.name, 'cat',
                                            '/usr/share/nginx/admin/index.html'], text=True)
            scripts = re.findall(r'src="(/assets/[^"]+\.js)"', html)
            self.assertTrue(scripts, 'Packaged index must reference its built JavaScript')
            for script in scripts:
                self.assertEqual(self.http_status(8081, script), 200)
        subprocess.run(['docker', 'exec', self.name, 'nginx', '-t'], check=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    def test_disabled_admin_has_no_listener_but_public_denial_remains(self):
        self.start_proxy('false')
        self.assertIsNone(self.http_status(8081, '/'))
        self.assertEqual(self.http_status(443, '/api/v1/admin/auth/me'), 404)
        result = subprocess.run(['docker', 'exec', self.name, 'test', '!', '-e',
                                 '/etc/nginx/conf.d/meetspace-admin.conf'])
        self.assertEqual(result.returncode, 0)


if __name__ == '__main__':
    unittest.main()
