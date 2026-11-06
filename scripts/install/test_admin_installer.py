"""Admin bootstrap/recovery boundary tests without a real database or secret files."""
import importlib.util
import base64
import io
import json
from contextlib import redirect_stdout
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock


spec = importlib.util.spec_from_file_location('admin_wizard', Path(__file__).with_name('wizard.py'))
wizard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(wizard)


class AdministratorInstallerTest(unittest.TestCase):
    def test_fresh_provider_key_is_independent_private_and_never_overwritten(self):
        with tempfile.TemporaryDirectory(prefix='meetspace-admin-key-') as scratch:
            root = Path(scratch)
            for folder in ('config', 'secrets', 'bundle/deployment'):
                (root / folder).mkdir(parents=True, exist_ok=True)
            (root / 'bundle/deployment/.env.production.example').write_text(
                'JWT_SECRET=\nPROVIDER_TOKEN_ENCRYPTION_KEY=\n')
            manifest = {'version': 'test', 'images': {
                name: 'sha256:' + '1' * 64 for name in ('api', 'product-worker', 'proxy', 'minio')}}
            state = {'domain': 'meet.example.invalid', 'ip': '192.0.2.1',
                     'subnet': '172.30.0.0/24', 'token': 'fixture', 'project': 'fixture'}
            output = io.StringIO()
            with mock.patch.object(wizard, 'run', side_effect=lambda *args, **kwargs:
                                   '{"NCPU":4}' if args[:2] == ('docker', 'info') else '{"services":{}}'), \
                    mock.patch.object(wizard, 'save_installer_runtime'), \
                    mock.patch.object(wizard, 'configure_monitoring'), redirect_stdout(output):
                wizard.make_configuration(root, manifest, state)
                env_path = root / 'config/meetspace.env'
                before = env_path.read_text()
                values = dict(line.split('=', 1) for line in before.splitlines())
                key = values['PROVIDER_TOKEN_ENCRYPTION_KEY']
                decoded = base64.b64decode(key, validate=True)
                self.assertEqual(len(decoded), 32)
                self.assertNotEqual(key, values['JWT_SECRET'])
                self.assertNotEqual(decoded, values['JWT_SECRET'].encode())
                self.assertEqual(env_path.stat().st_mode & 0o777, 0o600)
                with self.assertRaises(FileExistsError):
                    wizard.make_configuration(root, manifest, state)
                self.assertEqual(env_path.read_text(), before)
                self.assertNotIn(key, output.getvalue())

    def test_existing_administrators_are_never_prompted_or_reset(self):
        with mock.patch.object(wizard, 'administrator_command', return_value={
                'exists': True, 'username': 'existing.admin'}) as command, \
                mock.patch.object(wizard, 'ask') as ask, \
                mock.patch.object(wizard, 'administrator_password') as password:
            self.assertEqual(wizard.bootstrap_administrator(['docker', 'compose']), 'existing.admin')
        command.assert_called_once_with(['docker', 'compose'], 'status')
        ask.assert_not_called()
        password.assert_not_called()

    def test_password_is_only_passed_on_stdin(self):
        fake_password = '13579024'  # Synthetic eight-digit fixture, never a real credential.
        with mock.patch.object(wizard, 'run', side_effect=[
                '{"exists":false}', '{"exists":true,"username":"admin"}']) as run, \
                mock.patch.object(wizard, 'ask', return_value='Admin'), \
                mock.patch.object(wizard.getpass, 'getpass', side_effect=[fake_password, fake_password]):
            self.assertEqual(wizard.bootstrap_administrator(['docker', 'compose']), 'admin')
        first, second = run.call_args_list
        self.assertIsNone(first.kwargs['data'])
        self.assertNotIn(fake_password, repr(second.args))
        self.assertEqual(second.args[-1], 'bootstrap')
        self.assertIn('-T', second.args)
        self.assertEqual(json.loads(second.kwargs['data']), {
            'username': 'admin', 'password': fake_password})

    def test_status_error_aborts_before_reading_password(self):
        with mock.patch.object(wizard, 'administrator_command', side_effect=
                               subprocess.CalledProcessError(1, ['admin', 'status'])), \
                mock.patch.object(wizard, 'administrator_password') as password:
            with self.assertRaises(subprocess.CalledProcessError):
                wizard.bootstrap_administrator([])
        password.assert_not_called()

    def test_ambiguous_status_does_not_bootstrap(self):
        for status in ({}, {'exists': 'false'}, {'exists': True}):
            with self.subTest(status=status), \
                    mock.patch.object(wizard, 'administrator_command', return_value=status) as command:
                with self.assertRaises(RuntimeError):
                    wizard.bootstrap_administrator([])
                command.assert_called_once()

    def test_missing_bootstrap_confirmation_is_a_failure(self):
        with mock.patch.object(wizard, 'administrator_command', side_effect=[{'exists': False}, {}]), \
                mock.patch.object(wizard, 'ask', return_value='admin'), \
                mock.patch.object(wizard, 'administrator_password', return_value='Test password 42'):
            with self.assertRaisesRegex(RuntimeError, 'не подтверждено'):
                wizard.bootstrap_administrator([])

    def test_hidden_input_rejects_seven_characters_and_mismatching_passwords(self):
        with mock.patch.object(wizard.getpass, 'getpass', side_effect=[
                '1357902', '1357902', '13579024', '13579025',
                '13579024', '13579024']) as getpass:
            self.assertEqual(wizard.administrator_password(), '13579024')
        self.assertEqual(getpass.call_count, 6)

    def test_eight_character_password_does_not_require_letters_or_digits(self):
        for password in ('13579024', 'abcdefgh', '🔒' * 8):
            with self.subTest(password_kind='numeric' if password.isdecimal() else 'non-numeric'), \
                    mock.patch.object(wizard.getpass, 'getpass', side_effect=[password, password]) as getpass:
                self.assertEqual(wizard.administrator_password(), password)
                self.assertEqual(getpass.call_count, 2)

    def test_unicode_character_and_utf8_upper_bound(self):
        limit = '🔒' * 128
        with mock.patch.object(wizard.getpass, 'getpass', side_effect=[
                '🔒' * 7, '🔒' * 7, limit + '🔒', limit + '🔒', limit, limit]) as getpass:
            self.assertEqual(wizard.administrator_password(), limit)
        self.assertEqual(len(limit.encode('utf-8')), 512)
        self.assertEqual(getpass.call_count, 6)

    def test_reset_accepts_numeric_password_and_keeps_secret_on_stdin(self):
        fake_password = '13579024'
        output = io.StringIO()
        with mock.patch.object(wizard, 'require_local_docker'), \
                mock.patch.object(Path, 'read_text', return_value='{}'), \
                mock.patch.object(wizard, 'metadata', return_value={}), \
                mock.patch.object(wizard, 'require_image_store'), \
                mock.patch.object(wizard, 'compose_args', return_value=['docker', 'compose']), \
                mock.patch.object(wizard, 'ask', return_value='Admin'), \
                mock.patch.object(wizard.getpass, 'getpass', side_effect=[fake_password, fake_password]), \
                mock.patch.object(wizard, 'run', return_value='{"username":"admin","reset":true}') as run, \
                redirect_stdout(output):
            wizard.manage(Path('/unused-admin-test-fixture'), 'reset-admin-password', [])
        run.assert_called_once()
        call = run.call_args
        self.assertEqual(call.args[-1], 'reset-password')
        self.assertIn('-T', call.args)
        self.assertNotIn(fake_password, repr(call.args))
        self.assertNotIn(fake_password, output.getvalue())
        self.assertEqual(json.loads(call.kwargs['data']), {'username': 'admin', 'password': fake_password})

    def test_echo_fallback_is_never_allowed(self):
        with mock.patch.object(wizard.getpass, 'getpass', side_effect=wizard.getpass.GetPassWarning):
            with self.assertRaisesRegex(RuntimeError, 'интерактивный терминал'):
                wizard.administrator_password()

    def test_https_checks_real_hostname_on_loopback_and_public_denial(self):
        with mock.patch.object(wizard, 'run', side_effect=['200', '401', '404']) as run:
            wizard.verify_administrator_https({'domain': 'meet.example.com', 'adminHTTPSPort': 9443})
        calls = run.call_args_list
        self.assertIn('meet.example.com:9443:127.0.0.1', calls[0].args)
        self.assertEqual(calls[1].args[-1], 'https://meet.example.com:9443/api/v1/admin/auth/me')
        self.assertEqual(calls[2].args[-1], 'https://meet.example.com:443/api/v1/admin/auth/me')
        for call in calls:
            self.assertNotIn('--insecure', call.args)
            self.assertNotIn('-k', call.args)

    def test_public_api_leak_fails_installation_verification(self):
        with mock.patch.object(wizard, 'run', side_effect=['200', '401', '200'] * 12), \
                mock.patch.object(wizard.time, 'sleep'):
            with self.assertRaisesRegex(RuntimeError, 'Установка не завершена'):
                wizard.verify_administrator_https({'domain': 'meet.example.com', 'adminHTTPSPort': 8081})


if __name__ == '__main__':
    unittest.main()
