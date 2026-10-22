#!/usr/bin/env python3
"""Download a published, checksum-verified GitHub Docker bundle and run its wizard."""

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import urllib.parse
import urllib.request

REPOSITORY = 'janickiy/meet-space'
VERSION = re.compile(r'v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?\Z')
SHA256 = re.compile(r'[a-f0-9]{64}\Z')
MAX_ARCHIVE = 2 * 1024**3


def require(condition, message):
    """Reject invalid release metadata or unsafe archive entries before execution."""
    if not condition:
        raise RuntimeError(message)


class SecureRedirect(urllib.request.HTTPRedirectHandler):
    """Allow GitHub's signed asset redirects only over HTTPS, without credentials."""

    def redirect_request(self, request, response, code, message, headers, target):
        parsed = urllib.parse.urlsplit(target)
        require(parsed.scheme == 'https' and parsed.hostname and not parsed.username and not parsed.password,
                'Загрузка перенаправлена на небезопасный адрес.')
        return super().redirect_request(request, response, code, message, headers, target)


def response(url):
    """Open an HTTPS resource with a bounded socket timeout and normal TLS validation."""
    require(url.startswith('https://'), 'Требуется HTTPS.')
    request = urllib.request.Request(url, headers={'User-Agent': 'MeetSpace-prebuilt-installer'})
    return urllib.request.build_opener(SecureRedirect()).open(request, timeout=60)


def read_json(url):
    """Read bounded JSON release metadata without evaluating its contents as code."""
    with response(url) as stream:
        raw = stream.read(4 * 1024**2 + 1)
    require(len(raw) <= 4 * 1024**2, 'Слишком большой манифест выпуска.')
    return json.loads(raw)


def select_archive(index, release, platform):
    """Validate the requested version, source commit, platform and immutable asset URL."""
    require(index.get('schemaVersion') == 1 and index.get('installationMode') == 'prebuilt'
            and index.get('repository') == REPOSITORY and index.get('version') == release
            and re.fullmatch(r'[a-f0-9]{40}', index.get('commit', '')),
            'Манифест готовых образов не соответствует выпуску.')
    artifact = index.get('platforms', {}).get(platform)
    require(isinstance(artifact, dict), 'В этом выпуске нет пакета для ' + platform)
    name = f'meetspace-{release}-{platform.replace("/", "-")}.tar.gz'
    expected = f'https://github.com/{REPOSITORY}/releases/download/{release}/{name}'
    require(artifact.get('url') == expected and SHA256.fullmatch(artifact.get('sha256', ''))
            and type(artifact.get('sizeBytes')) is int and 0 < artifact['sizeBytes'] <= MAX_ARCHIVE,
            'Некорректный адрес, размер или контрольная сумма пакета.')
    return artifact


def download(artifact, destination):
    """Stream a release asset to disk, reporting progress and checking its full checksum."""
    total, last, hasher = 0, -1, hashlib.sha256()
    with response(artifact['url']) as stream, destination.open('xb') as output:
        for block in iter(lambda: stream.read(1024**2), b''):
            total += len(block)
            require(total <= artifact['sizeBytes'], 'Получено больше данных, чем указано в манифесте.')
            output.write(block)
            hasher.update(block)
            percent = total * 100 // artifact['sizeBytes']
            if percent // 10 != last:
                print(f'Загрузка готовых образов: {percent}% ({total // 1024**2} МиБ)', flush=True)
                last = percent // 10
    require(total == artifact['sizeBytes'] and hasher.hexdigest() == artifact['sha256'],
            'Контрольная сумма/размер пакета не совпадают. Пакет не запущен.')


def extract(archive, destination):
    """Extract only regular bundle files/directories, rejecting traversal and every link."""
    with tarfile.open(archive, 'r:gz') as source:
        seen, total, members = set(), 0, []
        for member in source:
            parts = PurePosixPath(member.name).parts
            require(parts and parts[0] == 'bundle' and not member.name.startswith('/')
                    and '..' not in parts and '\\' not in member.name
                    and not any(ord(c) < 32 for c in member.name)
                    and (member.isfile() or member.isdir()) and member.name not in seen,
                    'Небезопасный файл в установочном пакете.')
            require(member.mode & 0o7000 == 0, 'Специальные права в установочном пакете.')
            seen.add(member.name)
            total += member.size
            require(len(seen) <= 50000 and total <= 12 * 1024**3, 'Пакет превышает допустимый размер.')
            members.append(member)
        for member in members:
            path = destination.joinpath(*PurePosixPath(member.name).parts)
            if member.isdir():
                path.mkdir(parents=True, exist_ok=True, mode=0o700)
            else:
                path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
                with source.extractfile(member) as incoming, path.open('xb') as output:
                    shutil.copyfileobj(incoming, output, 1024**2)
                path.chmod(0o644)
    return destination / 'bundle'


def validate_bundle(bundle, index, platform):
    """Check that the extracted wizard and image bundle match the selected GitHub release."""
    info = json.loads((bundle / 'bundle.json').read_text())
    manifest_path = bundle / 'release/release.json'
    require(hashlib.sha256(manifest_path.read_bytes()).hexdigest() == info.get('manifestSHA256'),
            'Манифест пакета изменён.')
    manifest = json.loads(manifest_path.read_text())
    require(manifest.get('commit') == index['commit'] and manifest.get('version') == index['version']
            and manifest.get('platform') == platform and manifest.get('securityScanned') is True
            and manifest.get('sourceDirty') is False and manifest.get('artifactMode') == 'archive',
            'Происхождение/платформа готовых образов не совпадают.')
    for name in ('scripts/install/wizard.py', 'scripts/install/verify-release.sh', 'scripts/release/common.sh'):
        path = bundle / 'deployment' / name
        require(path.is_file() and hashlib.sha256(path.read_bytes()).hexdigest() == info['deploymentSHA256'].get(name),
                'Контрольная сумма инструментов установки не совпадает: ' + name)


def main():
    """Fetch the chosen architecture's published bundle; never silently start a source build."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--release', required=True)
    parser.add_argument('--platform', required=True, choices=('linux/amd64', 'linux/arm64'))
    args = parser.parse_args()
    require(VERSION.fullmatch(args.release), 'Некорректная версия выпуска.')
    require(os.geteuid() == 0 and sys.platform == 'linux', 'Нужен запуск от root на Linux.')
    os.umask(0o077)
    index = read_json(f'https://github.com/{REPOSITORY}/releases/download/{args.release}/prebuilt.json')
    artifact = select_archive(index, args.release, args.platform)
    work = Path(tempfile.mkdtemp(prefix='meetspace-images.', dir='/var/tmp'))
    print(f'Готовые образы: {args.release}\nИсходники GitHub: {index["commit"]}\n'
          f'Платформа: {args.platform}\nПакет: {artifact["sizeBytes"] // 1024**2} МиБ\n'
          'Компиляция и сканирование выполнены в GitHub Actions.', flush=True)
    try:
        archive = work / 'images.tar.gz'
        download(artifact, archive)
        bundle = extract(archive, work)
        validate_bundle(bundle, index, args.platform)
        archive.unlink()
        subprocess.run(['python3', '-I', str(bundle / 'deployment/scripts/install/wizard.py'), str(bundle)], check=True)
    except BaseException:
        print(f'\nПакет сохранён в {work}. Если проверка пакета прошла и мастер создал каталог установки, '
              'используйте напечатанную им команду продолжения.', file=sys.stderr)
        raise
    else:
        shutil.rmtree(work)


if __name__ == '__main__':
    try:
        main()
    except (KeyboardInterrupt, EOFError):
        print('Установка прервана.', file=sys.stderr)
        sys.exit(130)
    except (RuntimeError, OSError, ValueError, KeyError, subprocess.CalledProcessError) as error:
        print('Установка готовых образов остановлена: ' + str(error), file=sys.stderr)
        sys.exit(1)
