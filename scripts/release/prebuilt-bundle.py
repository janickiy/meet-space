#!/usr/bin/env python3
"""Package scanned images from a clean GitHub checkout for compilation-free installation."""

import argparse
import gzip
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile


def main():
    """Create an architecture-specific bundle, checksum and public release descriptor."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--package', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    source = Path(__file__).resolve().parents[2]
    spec = importlib.util.spec_from_file_location('source_installer', source / 'scripts/install/from-source.py')
    api = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(api)
    manifest = json.loads((args.package / 'release.json').read_text())
    commit = api.git(source, 'rev-parse', 'HEAD')
    api.require(manifest['commit'] == commit and api.git(source, 'status', '--porcelain') == '',
                'Prebuilt packages require this exact clean GitHub checkout.')
    api.require(manifest['platform'] in ('linux/amd64', 'linux/arm64')
                and re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?', manifest['version']),
                'Invalid package version/platform.')
    args.output.mkdir(mode=0o700, parents=True, exist_ok=False)
    bundle = args.output / 'bundle'
    api.make_bundle(source, args.package, bundle, manifest)
    name = f'meetspace-{manifest["version"]}-{manifest["platform"].replace("/", "-")}.tar.gz'
    archive = args.output / name
    with archive.open('xb') as raw, gzip.GzipFile(fileobj=raw, mode='wb', compresslevel=1, mtime=0) as compressed:
        with tarfile.open(fileobj=compressed, mode='w|') as output:
            output.add(bundle, arcname='bundle', recursive=True)
    api.require(archive.stat().st_size < 2 * 1024**3, 'GitHub release asset exceeds 2 GiB.')
    digest = api.digest(archive)
    (args.output / (name + '.sha256')).write_text(digest + '  ' + name + '\n')
    descriptor = dict(schemaVersion=1, version=manifest['version'], commit=commit,
                      platform=manifest['platform'], repository='janickiy/meet-space',
                      archive=dict(url=f'https://github.com/janickiy/meet-space/releases/download/{manifest["version"]}/{name}',
                                   sha256=digest, sizeBytes=archive.stat().st_size),
                      githubRunID=os.environ.get('GITHUB_RUN_ID', ''))
    (args.output / ('prebuilt-' + manifest['platform'].split('/')[1] + '.json')).write_text(json.dumps(descriptor, indent=2) + '\n')
    print('Prebuilt Docker image bundle:', archive)


if __name__ == '__main__':
    main()
