#!/usr/bin/env python3
"""Download a published, checksum-verified GitHub Docker bundle and run its wizard."""

import argparse
import datetime
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
import urllib.error
import urllib.request

REPOSITORY = 'janickiy/meet-space'
VERSION = re.compile(r'v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?\Z')
SHA256 = re.compile(r'[a-f0-9]{64}\Z')
MAX_ARCHIVE = 2 * 1024**3
MAX_RELEASE_PAGES = 10


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
    headers = {'User-Agent': 'MeetSpace-prebuilt-installer'}
    if urllib.parse.urlsplit(url).hostname == 'api.github.com':
        headers.update(Accept='application/vnd.github+json')
        headers['X-GitHub-Api-Version'] = '2022-11-28'
    request = urllib.request.Request(url, headers=headers)
    return urllib.request.build_opener(SecureRedirect()).open(request, timeout=60)


def read_json(url):
    """Read bounded JSON release metadata without evaluating its contents as code."""
    with response(url) as stream:
        raw = stream.read(4 * 1024**2 + 1)
    require(len(raw) <= 4 * 1024**2, 'Слишком большой манифест выпуска.')
    return json.loads(raw)


def select_archive(index, release, platform):
    """Validate the requested version, source commit, platform and immutable asset URL."""
    require(isinstance(index, dict), 'Манифест готовых образов должен быть JSON-объектом.')
    require(type(index.get('schemaVersion')) is int and index['schemaVersion'] == 1
            and index.get('installationMode') == 'prebuilt'
            and index.get('repository') == REPOSITORY and index.get('version') == release
            and isinstance(index.get('commit'), str) and re.fullmatch(r'[a-f0-9]{40}', index['commit']),
            'Манифест готовых образов не соответствует выпуску.')
    require(isinstance(index.get('platforms'), dict), 'Некорректный список платформ выпуска.')
    artifact = index['platforms'].get(platform)
    require(isinstance(artifact, dict), 'В этом выпуске нет пакета для ' + platform)
    name = f'meetspace-{release}-{platform.replace("/", "-")}.tar.gz'
    expected = f'https://github.com/{REPOSITORY}/releases/download/{release}/{name}'
    require(artifact.get('url') == expected and isinstance(artifact.get('sha256'), str)
            and SHA256.fullmatch(artifact['sha256'])
            and type(artifact.get('sizeBytes')) is int and 0 < artifact['sizeBytes'] <= MAX_ARCHIVE,
            'Некорректный адрес, размер или контрольная сумма пакета.')
    return artifact


def release_assets(release, platform):
    """Return a published release's complete installer assets for platform, or None.

    Args:
        release: One release object from the public GitHub API, never executable input.
        platform: The requested Linux architecture (linux/amd64 or linux/arm64).

    Drafts, source-only releases, duplicate names and unfinished uploads are not
    installable. Prereleases are eligible because MeetSpace server builds use them.
    """
    if not isinstance(release, dict) or release.get('draft') is not False:
        return None
    tag, published = release.get('tag_name'), release.get('published_at')
    if not isinstance(tag, str) or not VERSION.fullmatch(tag) or not isinstance(published, str):
        return None
    if not re.fullmatch(r'[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z', published):
        return None
    try:
        datetime.datetime.fromisoformat(published.replace('Z', '+00:00'))
    except ValueError:
        return None
    names = ('prebuilt.json', f'meetspace-{tag}-{platform.replace("/", "-")}.tar.gz')
    assets = release.get('assets')
    if not isinstance(assets, list):
        return None
    selected = {}
    for name in names:
        matches = [asset for asset in assets if isinstance(asset, dict) and asset.get('name') == name]
        if len(matches) != 1:
            return None
        asset = matches[0]
        expected = f'https://github.com/{REPOSITORY}/releases/download/{tag}/{name}'
        limit = 4 * 1024**2 if name == 'prebuilt.json' else MAX_ARCHIVE
        if (asset.get('state') != 'uploaded' or asset.get('browser_download_url') != expected
                or type(asset.get('size')) is not int or not 0 < asset['size'] <= limit):
            return None
        selected[name] = asset
    return selected


def resolve_release(requested, platform):
    """Resolve latest once, or validate an explicit tag, without starting an installation.

    Args:
        requested: 'latest' or a validated immutable server release tag.
        platform: The native Linux architecture whose ready-made bundle is required.

    Only published releases with both a manifest and the requested archive qualify.
    Sort by publication time rather than tag spelling or GitHub's latest badge.
    Network/rate-limit/integrity errors never cause an implicit source build.
    """
    require(requested == 'latest' or isinstance(requested, str) and VERSION.fullmatch(requested),
            'Укажите latest или корректную версию выпуска.')
    if requested != 'latest':
        try:
            index = read_json(f'https://github.com/{REPOSITORY}/releases/download/{requested}/prebuilt.json')
        except urllib.error.HTTPError as error:
            if error.code != 404:
                raise
            raise RuntimeError(f'Релиз {requested} или его prebuilt.json недоступен (HTTP 404). '
                               'Попробуйте --release latest. Для сборки из main: --mode source --ref main.') from None
        select_archive(index, requested, platform)
        return index
    releases = []
    for page in range(1, MAX_RELEASE_PAGES + 1):
        batch = read_json(f'https://api.github.com/repos/{REPOSITORY}/releases?per_page=100&page={page}')
        require(isinstance(batch, list) and len(batch) <= 100, 'GitHub вернул некорректный список выпусков.')
        releases.extend(batch)
        if len(batch) < 100:
            break
    else:
        raise RuntimeError('Слишком большой список выпусков. Укажите точную версию через --release VERSION.')
    candidates = [(release, assets) for release in releases
                  if (assets := release_assets(release, platform)) is not None]
    candidates.sort(key=lambda item: (item[0]['published_at'], item[0]['tag_name']), reverse=True)
    for release, assets in candidates:
        tag = release['tag_name']
        try:
            index = read_json(assets['prebuilt.json']['browser_download_url'])
        except urllib.error.HTTPError as error:
            if error.code != 404:
                raise
            # A release can disappear between discovery and manifest retrieval.
            continue
        artifact = select_archive(index, tag, platform)
        archive = assets[f'meetspace-{tag}-{platform.replace("/", "-")}.tar.gz']
        require(archive['size'] == artifact['sizeBytes'], 'Размер пакета отличается от метаданных GitHub.')
        checksum = archive.get('digest')
        require(checksum is None or checksum == 'sha256:' + artifact['sha256'],
                'Контрольная сумма пакета отличается от метаданных GitHub.')
        return index
    raise RuntimeError(f'В GitHub Releases нет опубликованного пакета готовых образов для {platform}. '
                       'Релизы только с исходниками не подходят. Для сборки актуального main '
                       'запустите: sudo bash install.sh --mode source --ref main. '
                       'Автоматическая сборка не запускалась.')


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
    parser.add_argument('--release', default='latest', help='latest or an exact published server tag')
    parser.add_argument('--platform', required=True, choices=('linux/amd64', 'linux/arm64'))
    parser.add_argument('--resolve-only', action='store_true', help='check release availability without installing')
    args = parser.parse_args()
    require(args.release == 'latest' or VERSION.fullmatch(args.release), 'Некорректная версия выпуска.')
    if args.resolve_only:
        index = resolve_release(args.release, args.platform)
        print(json.dumps(dict(version=index['version'], commit=index['commit'], platform=args.platform,
                              archive=select_archive(index, index['version'], args.platform)), indent=2))
        return
    require(os.geteuid() == 0 and sys.platform == 'linux', 'Нужен запуск от root на Linux.')
    os.umask(0o077)
    index = resolve_release(args.release, args.platform)
    artifact = select_archive(index, index['version'], args.platform)
    work = Path(tempfile.mkdtemp(prefix='meetspace-images.', dir='/var/tmp'))
    print(f'Готовые образы: {index["version"]}\nИсходники GitHub: {index["commit"]}\n'
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
    except urllib.error.HTTPError as error:
        hint = ('GitHub отклонил запрос или ограничил его частоту; повторите позже.' if error.code in (403, 429)
                else 'Проверьте доступность опубликованных файлов в GitHub Releases и повторите запуск.')
        print(f'Установка готовых образов остановлена: HTTP {error.code}. {hint} '
              'Сборка из исходников автоматически не запускалась.', file=sys.stderr)
        sys.exit(1)
    except (RuntimeError, OSError, ValueError, KeyError, subprocess.CalledProcessError) as error:
        print('Установка готовых образов остановлена: ' + str(error), file=sys.stderr)
        sys.exit(1)
