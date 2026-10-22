#!/usr/bin/env python3
"""Combine both scanned GitHub build descriptors into one public installer index."""

import importlib.util
import json
from pathlib import Path
import sys


def main():
    """Validate both architectures against the release commit before publishing the index."""
    root, version, commit = Path(sys.argv[1]), sys.argv[2], sys.argv[3]
    helper = Path(__file__).resolve().parents[1] / 'install/from-images.py'
    spec = importlib.util.spec_from_file_location('images', helper)
    api = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(api)
    index = dict(schemaVersion=1, installationMode='prebuilt', repository=api.REPOSITORY,
                 version=version, commit=commit, platforms={})
    for arch in ('amd64', 'arm64'):
        item = json.loads((root / ('prebuilt-' + arch + '.json')).read_text())
        api.require(item.get('repository') == api.REPOSITORY and item.get('version') == version
                    and item.get('commit') == commit and item.get('platform') == 'linux/' + arch,
                    'Architecture descriptor differs from release source.')
        index['platforms']['linux/' + arch] = item['archive']
        api.select_archive(index, version, 'linux/' + arch)
    (root / 'prebuilt.json').write_text(json.dumps(index, indent=2) + '\n')


if __name__ == '__main__':
    main()
