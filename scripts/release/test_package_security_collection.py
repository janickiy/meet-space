#!/usr/bin/env python3
"""Exercise package.sh security collection with stubbed images/scans, without Docker."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
SERVICES = ['api', 'media-worker', 'record-worker', 'product-worker', 'frontend', 'minio',
            'postgres', 'redis', 'rabbitmq', 'coturn', 'proxy', 'prometheus']
VERSION = 'v0.6.1-prebuilt.test'
COMMIT = 'a' * 40
SOURCE = 'b' * 64


@unittest.skipUnless(shutil.which('jq'), 'package.sh requires jq')
class PackageSecurityCollectionTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='package-security-test-')
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        scripts = self.root / 'scripts/release'
        scripts.mkdir(parents=True)
        for name in ('package.sh', 'common.sh'):
            shutil.copyfile(ROOT / 'scripts/release' / name, scripts / name)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        docker = self.bin / 'docker'
        docker.write_text('#!' + sys.executable + '\n' + r'''
import hashlib,json,os,sys
from pathlib import Path
args=sys.argv[1:]
with open(os.environ['TEST_DOCKER_LOG'],'a') as log: log.write(json.dumps(args)+'\n')
if args[:2] == ['image','inspect']:
    ref=args[-1]
    image='sha256:'+hashlib.sha256(ref.encode()).hexdigest()
    if '--format' in args:
        print(image)
    else:
        print(json.dumps([{'Id':image,'Config':{'Labels':{
            'org.opencontainers.image.version':os.environ['TEST_VERSION'],
            'org.opencontainers.image.revision':os.environ['TEST_COMMIT'],
            'io.go-recorder.source-sha256':os.environ['TEST_SOURCE']}}}]))
elif args[:2] == ['image','save']:
    Path(args[args.index('--output')+1]).write_bytes(b'fixture archive, not Docker images\n')
else:
    raise SystemExit('Unexpected Docker operation: '+repr(args))
''')
        docker.chmod(0o755)
        # package.sh invokes bash explicitly; this script preserves scan exit codes
        # and supplies real, checksummed report files for the package manifest.
        (scripts / 'scan-image.sh').write_text('''#!/usr/bin/env bash
set -euo pipefail
exec "$TEST_PYTHON" "$TEST_SCAN_STUB" "$@"
''')
        scan = self.root / 'scan.py'
        scan.write_text(r'''
import hashlib,json,os,sys
from pathlib import Path
reference,service,directory=sys.argv[1:4]
with open(os.environ['TEST_SCAN_LOG'],'a') as log: log.write(service+'\n')
failed=service in os.environ.get('TEST_FAILED_SERVICES','').split(',')
scan={'SchemaVersion':2,'Metadata':{'ImageID':'sha256:'+hashlib.sha256(reference.encode()).hexdigest(),
    'ImageConfig':{'architecture':'amd64'}},'Results':[{'Target':service,'Vulnerabilities':
    [{'VulnerabilityID':'TEST-HIGH','Severity':'HIGH'}] if failed else []}]}
Path(directory,service+'.scan.json').write_text(json.dumps(scan)+'\n')
Path(directory,service+'.sbom.json').write_text(json.dumps({'bomFormat':'CycloneDX'})+'\n')
raise SystemExit(7 if failed else 0)
''')
        self.build = self.root / 'build.json'
        self.build.write_text(json.dumps({'schemaVersion': 1, 'sourceDirty': False, 'version': VERSION,
            'commit': COMMIT, 'sourceSHA256': SOURCE, 'platform': 'linux/amd64',
            'deploymentLayout': 'hardened-shared-host',
            'images': {service: 'fixture/' + service + ':' + VERSION for service in SERVICES}}))
        self.environment = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ['PATH'],
            TEST_PYTHON=sys.executable, TEST_SCAN_STUB=str(scan), TEST_VERSION=VERSION,
            TEST_COMMIT=COMMIT, TEST_SOURCE=SOURCE,
            TRIVY_IMAGE='fixture/trivy@sha256:' + 'c' * 64)

    def run_package(self, name, *, collect=False, failed=(), mode='archive', extra=()):
        output = self.root / name
        docker_log, scan_log = self.root / (name + '-docker.log'), self.root / (name + '-scan.log')
        env = dict(self.environment, TEST_DOCKER_LOG=str(docker_log), TEST_SCAN_LOG=str(scan_log),
                   TEST_FAILED_SERVICES=','.join(failed))
        command = ['bash', str(self.root / 'scripts/release/package.sh'), '--build', str(self.build),
                   '--output', str(output), '--mode', mode]
        if collect:
            command.append('--collect-security-failures')
        process = subprocess.run(command + list(extra), env=env, text=True, capture_output=True, timeout=30)
        scans = scan_log.read_text().splitlines() if scan_log.exists() else []
        docker = [json.loads(line) for line in docker_log.read_text().splitlines()] if docker_log.exists() else []
        return process, output, scans, docker

    def assert_no_package(self, output, docker):
        for name in ('release.json', 'images.tar', 'SHA256SUMS'):
            self.assertFalse((output / name).exists(), name)
        self.assertFalse(any(args[:2] == ['image', 'save'] or args[:1] == ['push'] for args in docker))

    def test_collects_two_failures_and_scans_every_image_without_export(self):
        process, output, scans, docker = self.run_package('collect-failures', collect=True, failed=('api', 'frontend'))
        self.assertNotEqual(0, process.returncode)
        self.assertEqual(SERVICES, scans)
        self.assertIn('Image security gates failed for: api frontend', process.stdout + process.stderr)
        self.assert_no_package(output, docker)
        for service in SERVICES:
            self.assertTrue((output / (service + '.scan.json')).is_file())
            self.assertTrue((output / (service + '.sbom.json')).is_file())

    def test_default_keeps_fail_fast_and_original_scan_exit_code(self):
        process, output, scans, docker = self.run_package('fail-fast', failed=('api', 'frontend'))
        self.assertEqual(7, process.returncode)
        self.assertEqual(['api'], scans)
        self.assert_no_package(output, docker)

    def test_collection_rejects_nonarchive_modes_before_docker_or_scanning(self):
        for mode in ('registry', 'local'):
            with self.subTest(mode=mode):
                process, output, scans, docker = self.run_package('reject-' + mode, collect=True, mode=mode)
                self.assertNotEqual(0, process.returncode)
                self.assertIn('allowed only with --mode archive', process.stdout + process.stderr)
                self.assertEqual([], scans)
                self.assertEqual([], docker)
                self.assert_no_package(output, docker)

    def test_collection_cannot_skip_security_gates(self):
        process, output, scans, docker = self.run_package('reject-skip', collect=True, extra=('--skip-security-local',))
        self.assertNotEqual(0, process.returncode)
        self.assertIn('cannot skip security gates', process.stdout + process.stderr)
        self.assertEqual([], scans)
        self.assertEqual([], docker)
        self.assert_no_package(output, docker)

    def test_success_produces_identical_artifacts_with_and_without_collection(self):
        packages = []
        for name, collect in [('normal-success', False), ('collect-success', True)]:
            process, output, scans, docker = self.run_package(name, collect=collect)
            self.assertEqual(0, process.returncode, process.stdout + process.stderr)
            self.assertEqual(SERVICES, scans)
            self.assertEqual(1, sum(args[:2] == ['image', 'save'] for args in docker))
            manifest = json.loads((output / 'release.json').read_text())
            self.assertTrue(manifest['securityScanned'])
            self.assertEqual(set(SERVICES), set(manifest['securityEvidence']))
            self.assertEqual(hashlib.sha256((output / 'images.tar').read_bytes()).hexdigest(), manifest['archiveSHA256'])
            packages.append({name: (output / name).read_bytes() for name in ('release.json', 'images.tar', 'SHA256SUMS')})
        self.assertEqual(packages[0], packages[1])


if __name__ == '__main__':
    unittest.main()
