"""Certificate renewal hook regression tests without Docker or host changes."""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


HOOK = Path(__file__).with_name('renew-meeting-certificate.sh')
LINEAGE = '/etc/letsencrypt/live/meeting.janickiy.com'

MOCK = r'''
import json, os, pathlib, sys
command = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
with open(os.environ['HOOK_TEST_LOG'], 'a') as log:
    log.write(json.dumps([command, *args]) + '\n')
if command == 'docker':
    if args[0] == 'ps':
        project = next(a.split('=', 2)[2] for a in args if a.startswith('label=com.docker.compose.project='))
        service = next(a.split('=', 2)[2] for a in args if a.startswith('label=com.docker.compose.service='))
        print('\n'.join(json.loads(os.environ['HOOK_TEST_CONTAINERS']).get(project + '/' + service, [])))
    elif args[0] == 'exec':
        phase = 'test' if args[-1] == '-t' else 'reload'
        if os.environ.get('HOOK_TEST_FAIL') == phase:
            print('mock nginx ' + phase + ' failed', file=sys.stderr)
            sys.exit(17)
'''


class RenewalHookTests(unittest.TestCase):
    def run_hook(self, containers=None, failure='', lineage=LINEAGE):
        with tempfile.TemporaryDirectory(prefix='meetspace-renew-test-') as directory:
            root = Path(directory)
            command = root / 'mock-command'
            command.write_text('#!' + sys.executable + '\n' + MOCK)
            command.chmod(0o755)
            for name in ('install', 'apache2ctl', 'systemctl', 'docker'):
                (root / name).symlink_to(command)
            log = root / 'calls.jsonl'
            environment = dict(os.environ, PATH=str(root) + os.pathsep + os.environ['PATH'],
                               RENEWED_LINEAGE=lineage, HOOK_TEST_LOG=str(log),
                               HOOK_TEST_CONTAINERS=json.dumps(containers or {}), HOOK_TEST_FAIL=failure)
            result = subprocess.run(['bash', str(HOOK)], env=environment, capture_output=True, text=True)
            calls = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
            return result, calls

    def test_reloads_each_running_proxy_after_copy_and_validation(self):
        result, calls = self.run_hook({
            'recorder-staging-meeting/proxy': ['proxy-staging-1', 'proxy-staging-2'],
            'recorder-production-meeting/proxy': ['proxy-production'],
            'recorder-staging-meeting/coturn': ['turn-staging'],
        })
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[0] for call in calls[:4]], ['install', 'install', 'apache2ctl', 'systemctl'])
        self.assertEqual([call for call in calls if call[:2] == ['docker', 'exec']], [
            ['docker', 'exec', identifier, 'nginx', *arguments]
            for identifier in ('proxy-staging-1', 'proxy-staging-2', 'proxy-production')
            for arguments in (['-t'], ['-s', 'reload'])
        ])
        self.assertIn(['docker', 'restart', 'turn-staging'], calls)

    def test_absent_proxy_preserves_apache_and_coturn_renewal(self):
        result, calls = self.run_hook({'recorder-production-meeting/coturn': ['turn-production']})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any(call[:2] == ['docker', 'exec'] for call in calls))
        self.assertIn(['systemctl', 'reload', 'apache2'], calls)
        self.assertIn(['docker', 'restart', 'turn-production'], calls)

    def test_nginx_failure_is_visible_and_stops_hook(self):
        for phase in ('test', 'reload'):
            with self.subTest(phase=phase):
                result, calls = self.run_hook({'recorder-staging-meeting/proxy': ['proxy-staging']}, failure=phase)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('mock nginx ' + phase + ' failed', result.stderr)
                if phase == 'test':
                    self.assertNotIn(['docker', 'exec', 'proxy-staging', 'nginx', '-s', 'reload'], calls)
                self.assertFalse(any(call[:2] == ['docker', 'restart'] for call in calls))

    def test_other_certificate_is_untouched(self):
        result, calls = self.run_hook(lineage='/etc/letsencrypt/live/unrelated.example')
        self.assertEqual(result.returncode, 0)
        self.assertEqual(calls, [])


if __name__ == '__main__':
    unittest.main()
