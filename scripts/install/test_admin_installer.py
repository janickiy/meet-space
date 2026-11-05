"""Admin bootstrap/recovery boundary tests without a real database or secret files."""
import importlib.util
import json
from pathlib import Path
import subprocess
import unittest
from unittest import mock


spec = importlib.util.spec_from_file_location('admin_wizard', Path(__file__).with_name('wizard.py'))
wizard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(wizard)


class AdministratorInstallerTest(unittest.TestCase):
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
        fake_password = 'Test only password 42'
        with mock.patch.object(wizard, 'run', side_effect=[
                '{"exists":false}', '{"exists":true,"username":"admin"}']) as run, \
                mock.patch.object(wizard, 'ask', return_value='Admin'), \
                mock.patch.object(wizard, 'administrator_password', return_value=fake_password):
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

    def test_hidden_input_rejects_weak_and_mismatching_passwords(self):
        with mock.patch.object(wizard.getpass, 'getpass', side_effect=[
                'short1', 'short1', 'Long enough 42', 'Different 42',
                'Long enough 42', 'Long enough 42']):
            self.assertEqual(wizard.administrator_password(), 'Long enough 42')

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
