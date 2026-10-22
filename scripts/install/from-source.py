#!/usr/bin/env python3
"""Build a clean public GitHub checkout on the target host, then run the installer."""

import argparse
import contextlib
import fcntl
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import platform
import re
import secrets
import shlex
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile


REPOSITORY = 'https://github.com/janickiy/meet-space.git'
SCANNER = 'aquasec/trivy@sha256:af6acf9a6b85dfe389a1941505c0ce9efef52a4719635e1a962f022a3d855daa'
BUILDKIT = 'moby/buildkit@sha256:cec9f139f45e93c5c69c60f8b07cfad9f43f4ef6b6a6cd917527fea5ff2e3dea'
BUILDKIT_CONFIG = '[worker.oci]\n  max-parallelism = 1\n'
SERVICES = frozenset(('api', 'media-worker', 'worker', 'product-worker', 'live-worker',
                      'frontend', 'minio', 'postgres', 'redis', 'rabbitmq', 'coturn', 'proxy', 'prometheus'))
DEPLOYMENT_FILES = (
    'docker-compose.production.yml', 'docker-compose.hardened.yml', 'docker-compose.grafana.yml',
    '.env.production.example', 'scripts/release/common.sh', 'dockers/production/routes.conf',
    'scripts/install/wizard.py', 'scripts/install/verify-release.sh',
    'dockers/turn/entrypoint.sh', 'dockers/turn/turnserver.conf',
    'dockers/observability/prometheus.yml', 'dockers/observability/alerts.yml',
    'dockers/observability/dashboard.json',
    'dockers/observability/provisioning/dashboards/recorder.yml',
    'dockers/observability/provisioning/datasources/recorder.yml',
)
DIGEST = re.compile(r'sha256:[a-f0-9]{64}\Z')


def require(condition, message):
    """Raise a safe diagnostic when the supplied invariant does not hold."""
    if not condition:
        raise RuntimeError(message)


def digest(path):
    """Return the SHA-256 of a regular file at path using bounded memory."""
    result = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            result.update(block)
    return result.hexdigest()


def environment(work=None):
    """Return a scrubbed command environment; work optionally selects private build scratch."""
    result = {key: os.environ[key] for key in ('PATH', 'HOME', 'TERM', 'LANG', 'LC_ALL') if key in os.environ}
    result.update(GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL='/dev/null',
                  GIT_TERMINAL_PROMPT='0', PYTHONDONTWRITEBYTECODE='1')
    if work is not None:
        result.update(TMPDIR=str(work / 'scratch'), TRIVY_IMAGE=SCANNER)
    return result


def command(*args, capture=False, work=None):
    """Execute explicit arguments with sanitized environment; optionally return standard output."""
    result = subprocess.run(args, check=True, stdin=subprocess.DEVNULL,
                            stdout=subprocess.PIPE if capture else None, text=True,
                            env=environment(work))
    return result.stdout.strip() if capture else None


def git(source, *args):
    """Run read-only or explicitly requested Git arguments with hooks and external filters disabled."""
    return command('git', '-c', 'core.hooksPath=/dev/null', '-c', 'core.fsmonitor=false',
                   '-C', str(source), *args, capture=True)


def regular_file(root, relative):
    """Resolve a tracked relative file while rejecting symlinks and traversal at every level."""
    parts = Path(relative).parts
    require(parts and not Path(relative).is_absolute() and all(part not in ('.', '..') for part in parts)
            and '\n' not in relative and '\r' not in relative, 'Недопустимый путь в исходниках.')
    current = root
    for part in parts:
        current = current / part
        require(not current.is_symlink(), 'Символьные ссылки в исходниках не поддерживаются.')
    require(current.is_file(), 'Не найден обычный файл исходников: ' + relative)
    return current


def tracked_files(source):
    """Return tracked regular files from source; submodules and symbolic links are unsupported."""
    records = git(source, 'ls-files', '--stage', '-z').split('\0')
    files = []
    for record in filter(None, records):
        header, relative = record.split('\t', 1)
        mode, _, stage = header.split()
        require(mode in ('100644', '100755') and stage == '0', 'Нужны обычные файлы без конфликтов и submodules.')
        regular_file(source, relative)
        files.append(relative)
    require(files and len(files) == len(set(files)), 'Пустой или неоднозначный набор исходников.')
    return sorted(files)


def verify_checkout(source, commit=None):
    """Require an absolute clean checkout of the exact public repository and optional commit."""
    require(source.is_absolute() and source.resolve() == source and source.is_dir()
            and not source.is_symlink(), 'Укажите абсолютный обычный каталог исходников.')
    require((source / '.git').is_dir() and not (source / '.git').is_symlink(), 'Нужен самостоятельный Git checkout.')
    require(git(source, 'rev-parse', '--show-toplevel') == str(source), 'Каталог не является корнем Git checkout.')
    require(git(source, 'remote', 'get-url', 'origin') == REPOSITORY, 'Origin должен указывать на публичный репозиторий MeetSpace.')
    head = git(source, 'rev-parse', 'HEAD')
    require(re.fullmatch(r'[a-f0-9]{40}', head) and (commit is None or head == commit), 'Коммит исходников не совпадает.')
    require(not git(source, 'status', '--porcelain', '--untracked-files=all', '--ignored'),
            'Исходники изменены или содержат посторонние файлы. Нужен чистый checkout GitHub.')
    tracked_files(source)
    for relative in (*DEPLOYMENT_FILES, 'scripts/release/build.sh', 'scripts/release/package.sh',
                     'scripts/install/from-source.py'):
        regular_file(source, relative)
    return head


def normalize_source_permissions(source, commit):
    """Make verified build inputs readable without exposing the private workspace.

    Args:
        source: The installer-owned source directory inside a private workspace.
        commit: The exact Git commit that must remain clean and unchanged.

    Only tracked regular files and their ancestor directories are normalized.
    Git executable bits are preserved; the workspace, Git metadata and sibling
    secrets retain their private modes. Validate every target before any chmod.
    """
    work = source.parent
    attributes = work.stat()
    require(source.name == 'source' and work.is_dir() and not work.is_symlink()
            and attributes.st_uid == os.geteuid() and stat.S_IMODE(attributes.st_mode) == 0o700,
            'Исходники должны находиться в приватном каталоге сборки с правами 0700.')
    verify_checkout(source, commit)
    files, directories = {}, {source}
    for record in filter(None, git(source, 'ls-files', '--stage', '-z').split('\0')):
        header, relative = record.split('\t', 1)
        mode, _, stage = header.split()
        require(mode in ('100644', '100755') and stage == '0' and '.git' not in Path(relative).parts,
                'Нужны обычные отслеживаемые файлы без конфликтов и служебных каталогов.')
        path = regular_file(source, relative)
        require(path not in files, 'Неоднозначный набор исходников.')
        files[path] = 0o755 if mode == '100755' else 0o644
        parent = path.parent
        while parent != source:
            directories.add(parent)
            parent = parent.parent
    require(files, 'Пустой набор исходников.')
    for directory in sorted(directories, key=lambda path: len(path.parts)):
        directory.chmod(0o755)
    for path, mode in files.items():
        path.chmod(mode)
    verify_checkout(source, commit)


def load_wizard(source):
    """Load installation APIs from the verified checkout without generating bytecode inside it."""
    sys.dont_write_bytecode = True
    spec = importlib.util.spec_from_file_location('meetspace_source_wizard', source / 'scripts/install/wizard.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


@contextlib.contextmanager
def installation_lock():
    """Serialize build/setup with other installers; release the lock before starting the wizard."""
    descriptor = os.open('/run/meetspace-installer.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    attributes = os.fstat(descriptor)
    if not stat.S_ISREG(attributes.st_mode) or attributes.st_uid != 0 or attributes.st_mode & 0o077:
        os.close(descriptor)
        raise RuntimeError('Файл блокировки установки должен принадлежать root с правами 0600.')
    with os.fdopen(descriptor, 'r+') as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise RuntimeError('Другой мастер установки уже запущен.') from None
        yield


def prerequisites(wizard):
    """Offer missing build tools through apt and initialize a local rootful Docker engine."""
    packages = {'git': 'git', 'bash': 'bash', 'curl': 'curl', 'jq': 'jq', 'openssl': 'openssl',
                'rg': 'ripgrep', 'tar': 'tar'}
    missing = [package for executable, package in packages.items() if not shutil.which(executable)]
    if missing:
        if not wizard.yes('Установить недостающие инструменты сборки: ' + ', '.join(missing) + '?'):
            raise RuntimeError('Инструменты сборки обязательны.')
        wizard.apt(*missing)
    wizard.docker_setup()
    command('docker', 'buildx', 'version', capture=True)


def resource_check(wizard, work, building=True):
    """Check native Docker and return its platform/info; disk capacity remains advisory."""
    architecture = {'x86_64': 'amd64', 'amd64': 'amd64', 'aarch64': 'arm64', 'arm64': 'arm64'}.get(platform.machine())
    require(architecture, 'Сборка поддерживается только на Linux amd64 или arm64.')
    info = json.loads(command('docker', 'info', '--format', '{{json .}}', capture=True))
    docker_arch = {'x86_64': 'amd64', 'amd64': 'amd64', 'aarch64': 'arm64', 'arm64': 'arm64'}.get(info.get('Architecture'))
    require(docker_arch == architecture, 'Нужен локальный Docker той же архитектуры, что и сервер.')
    if info.get('NCPU', 0) < 4 or info.get('MemTotal', 0) < 8 * 1024**3:
        print('Для сборки рекомендуются не менее 4 CPU и 8 ГБ RAM.')
    recommended_gib = 35 if building else 15
    for location in (work, Path(info['DockerRootDir'])):
        wizard.warn_disk_space(location, recommended_gib * 1024**3)
    return 'linux/' + architecture, info


def available_memory_mib():
    """Read currently available Linux RAM in MiB without counting swap as free memory."""
    for line in Path('/proc/meminfo').read_text().splitlines():
        if line.startswith('MemAvailable:'):
            return int(line.split()[1]) // 1024
    raise RuntimeError('Не удалось определить доступную оперативную память.')


def select_build_mode(wizard, state, requested, info):
    """Choose and persist a mode using the requested value, saved state and Docker resources."""
    mode = state.get('buildMode', 'auto') if requested == 'auto' else requested
    require(mode in ('auto', 'standard', 'low-memory'), 'Неизвестный режим сборки.')
    if mode == 'auto':
        small = (info.get('NCPU', 0) < 4 or info.get('MemTotal', 0) < 8 * 1024**3
                 or available_memory_mib() < 4096)
        print('\nРежим сборки:\n  1 — экономный: меньше памяти, сборка медленнее.\n'
              '  2 — обычный: быстрее, без отдельного ограничения памяти сборки.')
        choice = wizard.ask('Выберите режим', '1' if small else '2', lambda value: value in ('1', '2'))
        mode = 'low-memory' if choice == '1' else 'standard'
    state['buildMode'] = mode
    if mode == 'low-memory':
        require(info.get('MemoryLimit') and info.get('SwapLimit'),
                'Экономному режиму нужна поддержка ограничений памяти и swap в Docker.')
        print('Экономный режим: один CPU, последовательная сборка, не более 2 ГиБ RAM; '
              'swap сервера не изменяется. Проверка образов — не более 1 ГиБ RAM.')
    return mode


def builder_container(name):
    """Inspect the single Docker container belonging to the supplied installer builder name."""
    return json.loads(command('docker', 'inspect', 'buildx_buildkit_' + name + '0', capture=True))[0]


def verify_builder(name, memory_mib, expected_id=None):
    """Verify the named builder's identity and applied hard RAM/swap/CPU limits before use."""
    container = builder_container(name)
    settings = container['HostConfig']
    require(expected_id is None or container['Id'] == expected_id, 'Контейнер сборщика был заменён.')
    require(container['Config']['Image'] == BUILDKIT
            and settings['Memory'] == memory_mib * 1024**2
            and settings['MemorySwap'] == settings['Memory']
            and settings['CpuPeriod'] == 100000 and settings['CpuQuota'] == 100000
            and settings['RestartPolicy']['Name'] == 'no',
            'Docker не применил ожидаемые ограничения сборки. Неограниченная сборка не запускается.')
    return container['Id']


@contextlib.contextmanager
def low_memory_builder(work, state, info):
    """Run an owned, bounded BuildKit for work/state; stop it and retain cache on every exit."""
    # Keep headroom for existing services and the Docker daemon. This is a snapshot,
    # not a reservation: concurrent host workloads can still exhaust their own RAM.
    available = available_memory_mib()
    budget = max(128, min(2048, int(info['MemTotal']) // 1024**2 - 1536, available - 768))
    budget = max(128, budget // 128 * 128)
    saved = state.get('builder')
    if saved is None:
        name = 'meetspace-' + secrets.token_hex(12)
        saved = dict(name=name, memoryMiB=budget)
        state['builder'] = saved
        save_state(work, state)
    name, memory_mib = saved['name'], saved['memoryMiB']
    require(re.fullmatch(r'meetspace-[a-f0-9]{24}', name) and type(memory_mib) is int
            and 128 <= memory_mib <= 2048, 'Неверные параметры сохранённого сборщика.')
    if memory_mib > budget:
        print('Предупреждение: сейчас свободно меньше памяти, чем при предыдущем запуске сборки.')
        require(available >= memory_mib + 256,
                'Для продолжения освободите оперативную память или повторите позже; службы не остановлены.')
    config = work / 'buildkit-low-memory.toml'
    if config.exists():
        require(not config.is_symlink() and config.read_text() == BUILDKIT_CONFIG, 'Настройки сборщика изменены.')
    else:
        config.write_text(BUILDKIT_CONFIG)
    stop = False
    try:
        if saved.get('created'):
            if saved.get('containerID'):
                verify_builder(name, memory_mib, saved['containerID'])
            stop = True
            command('docker', 'buildx', 'inspect', '--bootstrap', name, capture=True)
        else:
            command('docker', 'buildx', 'create', '--name', name, '--driver', 'docker-container',
                    '--buildkitd-config', str(config), '--driver-opt', f'image={BUILDKIT}',
                    '--driver-opt', f'memory={memory_mib}m,memory-swap={memory_mib}m',
                    '--driver-opt', 'cpu-period=100000,cpu-quota=100000,restart-policy=no',
                    '--driver-opt', 'env.GOMAXPROCS=1,env.GOMEMLIMIT=256MiB,env.GOGC=20', capture=True)
            saved['created'] = True
            save_state(work, state)
            stop = True
            command('docker', 'buildx', 'inspect', '--bootstrap', name, capture=True)
        saved['containerID'] = verify_builder(name, memory_mib, saved.get('containerID'))
        save_state(work, state)
        print(f'Сборщик ограничен {memory_mib} МиБ RAM и одним CPU. Кеш сохраняется для продолжения.')
        yield name
    finally:
        if stop:
            # Never prune shared caches, alter swap, or stop application containers.
            result = subprocess.run(['docker', 'buildx', 'stop', name], env=environment(), check=False)
            if result.returncode:
                print('Не удалось остановить сборщик: docker buildx stop ' + name, file=sys.stderr)


def save_state(work, state):
    """Atomically persist non-secret build progress in the private work directory."""
    pending = work / ('state-' + secrets.token_hex(6) + '.tmp')
    with pending.open('x') as stream:
        json.dump(state, stream, indent=2)
        stream.write('\n')
    pending.chmod(0o600)
    pending.replace(work / 'source-build.json')


def clone_for_build(source, work, commit):
    """Create an independent local clone so temporary bootstrap files may be removed safely."""
    destination = work / 'source'
    command('git', '-c', 'core.hooksPath=/dev/null', '-c', 'protocol.file.allow=always',
            'clone', '--no-hardlinks', '--no-checkout', '--single-branch', str(source), str(destination))
    git(destination, 'remote', 'set-url', 'origin', REPOSITORY)
    git(destination, '-c', 'advice.detachedHead=false', 'checkout', '--detach', commit)
    verify_checkout(destination, commit)


def json_member(archive, members, name, limit=4 * 1024**2):
    """Read one regular JSON member with the supplied byte limit, without extracting it."""
    member = members.get(name)
    require(member is not None and member.isfile() and member.size <= limit,
            'Отсутствует допустимый JSON в архиве Docker.')
    with archive.extractfile(member) as stream:
        return stream.read()


def verify_attestation(archive, members, descriptor, image_manifest, config):
    """Verify non-runtime OCI provenance/SBOM bytes and return their bound runtime subject."""
    annotations = descriptor.get('annotations') or {}
    subject = annotations.get('io.containerd.manifest.subject')
    if not subject and annotations.get('vnd.docker.reference.type') == 'attestation-manifest':
        subject = annotations.get('vnd.docker.reference.digest')
    require(isinstance(subject, str) and DIGEST.fullmatch(subject)
            and config.get('architecture') == 'unknown' and config.get('os') == 'unknown',
            'В архиве обнаружен посторонний OCI-объект.')
    layers = image_manifest.get('layers')
    require(isinstance(layers, list) and layers, 'Пустой OCI attestation.')
    for layer in layers:
        checksum = layer.get('digest', '')
        require(layer.get('mediaType') == 'application/vnd.in-toto+json'
                and isinstance(checksum, str) and DIGEST.fullmatch(checksum),
                'Недопустимый слой OCI attestation.')
        raw = json_member(archive, members, 'blobs/sha256/' + checksum[7:], limit=32 * 1024**2)
        require('sha256:' + hashlib.sha256(raw).hexdigest() == checksum, 'Контрольная сумма attestation не совпадает.')
        statement = json.loads(raw)
        subjects = statement.get('subject')
        require(str(statement.get('_type', '')).startswith('https://in-toto.io/Statement/')
                and isinstance(subjects, list) and subjects
                and all(item.get('digest', {}).get('sha256') == subject[7:] for item in subjects),
                'Attestation не связан с образом выпуска.')
    return subject


def archive_inventory(archive_path, expected_architecture):
    """Bind runtime image digests while allowing only checksum-bound in-toto attestations."""
    with tarfile.open(archive_path, 'r:') as archive:
        members = {}
        for member in archive:
            name = member.name
            require(not name.startswith('/') and '..' not in Path(name).parts
                    and (member.isfile() or member.isdir()) and name not in members,
                    'Недопустимый или повторный элемент архива Docker.')
            members[name] = member
        saved, attestations, seen = [], [], set()
        is_oci = 'index.json' in members
        inventory = json.loads(json_member(archive, members, 'index.json' if is_oci else 'manifest.json'))
        descriptors = inventory['manifests'] if is_oci else inventory
        require(isinstance(descriptors, list) and descriptors, 'Пустой архив Docker.')
        for descriptor in descriptors:
            if is_oci:
                image_id = descriptor['digest']
                require(DIGEST.fullmatch(image_id), 'Недопустимый OCI digest.')
                raw = json_member(archive, members, 'blobs/sha256/' + image_id[7:])
                require('sha256:' + hashlib.sha256(raw).hexdigest() == image_id, 'Контрольная сумма OCI manifest не совпадает.')
                image_manifest = json.loads(raw)
                config_id = image_manifest['config']['digest']
                require(DIGEST.fullmatch(config_id), 'Недопустимый OCI config digest.')
                config_name = 'blobs/sha256/' + config_id[7:]
            else:
                config_name = descriptor['Config']
                require(re.fullmatch(r'(?:[a-f0-9]{64}\.json|blobs/sha256/[a-f0-9]{64})', config_name),
                        'Недопустимый путь конфигурации Docker.')
                config_file = Path(config_name).name
                config_id = 'sha256:' + (config_file[:-5] if config_file.endswith('.json') else config_file)
                image_id = config_id
            raw = json_member(archive, members, config_name)
            require('sha256:' + hashlib.sha256(raw).hexdigest() == config_id, 'Контрольная сумма Docker config не совпадает.')
            config = json.loads(raw)
            require(image_id not in seen, 'Повторный образ в архиве Docker.')
            seen.add(image_id)
            if is_oci and config.get('architecture') == 'unknown' and config.get('os') == 'unknown':
                attestations.append(verify_attestation(archive, members, descriptor, image_manifest, config))
                continue
            require(config.get('architecture') == expected_architecture and config.get('os') == 'linux',
                    'Архитектура образа не соответствует серверу.')
            saved.append((image_id, config_id))
        require(len(saved) == len(set(saved)), 'Повторный образ в архиве Docker.')
        require(set(attestations) <= {image for image, _ in saved}, 'Attestation ссылается на посторонний образ.')
        return is_oci, saved


def normalize_release(package, output, commit, version, target_platform):
    """Create a fully checked delivery manifest from a fresh, scanned package without Node.js."""
    require(not output.exists(), 'Каталог проверенного выпуска уже существует.')
    manifest = json.loads((package / 'release.json').read_text())
    require(manifest.get('schemaVersion') == 1 and manifest.get('artifactMode') == 'archive'
            and manifest.get('sourceDirty') is False and manifest.get('sourceFingerprintVerified') is True
            and manifest.get('securityScanned') is True and not manifest.get('infrastructureReused')
            and manifest.get('commit') == commit and manifest.get('version') == version
            and manifest.get('platform') == target_platform
            and manifest.get('deploymentLayout') == 'hardened-shared-host'
            and re.fullmatch(r'[a-f0-9]{64}', manifest.get('sourceSHA256', ''))
            and set(manifest.get('images', {})) == SERVICES, 'Нужен полный проверенный выпуск из чистых исходников.')
    require(digest(package / 'images.tar') == manifest.get('archiveSHA256'), 'Контрольная сумма архива образов не совпадает.')
    architecture = target_platform.split('/')[1]
    is_oci, saved = archive_inventory(package / 'images.tar', architecture)
    original = dict(manifest['images'])
    configs = {}
    for service in sorted(SERVICES):
        evidence = manifest['securityEvidence'][service]
        scan_path, sbom_path = package / (service + '.scan.json'), package / (service + '.sbom.json')
        require(digest(scan_path) == evidence['scanSHA256'] and digest(sbom_path) == evidence['sbomSHA256'],
                'Изменены результаты проверки безопасности: ' + service)
        scan, sbom = json.loads(scan_path.read_text()), json.loads(sbom_path.read_text())
        require('SchemaVersion' in scan and isinstance(scan.get('Results'), list)
                and sbom.get('bomFormat') == 'CycloneDX', 'Отсутствует полный отчёт безопасности: ' + service)
        require(not any(item.get('Severity') in ('HIGH', 'CRITICAL') for result in scan['Results']
                        for item in (result.get('Vulnerabilities') or [])),
                'Найдены HIGH/CRITICAL уязвимости; установка остановлена: ' + service)
        require(scan['Metadata']['ImageConfig']['architecture'] == architecture, 'Архитектура отчёта не совпадает.')
        config_id = scan['Metadata']['ImageID']
        require(isinstance(config_id, str) and DIGEST.fullmatch(config_id), 'Недопустимый ImageID отчёта.')
        matches = [image_id for image_id, saved_config in saved if saved_config == config_id]
        require(len(matches) == 1, 'Образ отчёта отсутствует или неоднозначен: ' + service)
        # Docker's containerd store can report an image-index ID rather than either
        # exported platform manifest or config digest. Trivy binds the saved bytes.
        require(isinstance(original[service], str) and DIGEST.fullmatch(original[service]),
                'Недопустимый исходный ImageID выпуска: ' + service)
        manifest['images'][service], configs[service] = matches[0], config_id
    require({image for image, _ in saved} == set(manifest['images'].values()), 'Архив содержит посторонние образы.')
    manifest['originalBuildImageIDs'] = original
    manifest['archiveBinding'] = 'OCI manifest -> config digest -> Trivy report' if is_oci else 'Docker config digest -> Trivy report'
    if is_oci:
        manifest['imageConfigIDs'] = configs
    else:
        manifest.pop('imageConfigIDs', None)
    output.mkdir(mode=0o700)
    # A same-filesystem hard link avoids duplicating the multi-gigabyte immutable archive.
    os.link(package / 'images.tar', output / 'images.tar')
    for service in sorted(SERVICES):
        for suffix in ('scan.json', 'sbom.json'):
            shutil.copyfile(package / (service + '.' + suffix), output / (service + '.' + suffix))
    (output / 'release.json').write_text(json.dumps(manifest, indent=2) + '\n')
    return manifest


def make_bundle(source, package, destination, state):
    """Assemble allowlisted deployment files and tracked public sources with individual checksums."""
    destination.mkdir(mode=0o700)
    manifest = normalize_release(package, destination / 'release', state['commit'], state['version'], state['platform'])
    deployment_hashes, source_hashes = {}, {}
    for relative in DEPLOYMENT_FILES:
        path = destination / 'deployment' / relative
        path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        shutil.copyfile(regular_file(source, relative), path)
        deployment_hashes[relative] = digest(path)
    for relative in tracked_files(source):
        path = destination / 'source' / relative
        path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        shutil.copyfile(regular_file(source, relative), path)
        source_hashes[relative] = digest(path)
    metadata = dict(schemaVersion=1, version=manifest['version'], platform=manifest['platform'],
                    manifestSHA256=digest(destination / 'release/release.json'), deploymentSHA256=deployment_hashes,
                    source=dict(repository=REPOSITORY, commit=state['commit'], filesSHA256=source_hashes))
    (destination / 'bundle.json').write_text(json.dumps(metadata, indent=2) + '\n')
    command('bash', str(destination / 'deployment/scripts/install/verify-release.sh'), str(destination))


def validate_work(path):
    """Accept only an existing private root-owned workspace created by this installer."""
    require(path.is_absolute() and path.resolve() == path and path.parent == Path('/var/tmp')
            and re.fullmatch(r'meetspace-source-build\.[a-zA-Z0-9_-]+', path.name)
            and path.is_dir() and not path.is_symlink(), 'Неверный каталог продолжения сборки.')
    attributes = path.stat()
    require(attributes.st_uid == 0 and not attributes.st_mode & 0o077, 'Каталог сборки должен принадлежать root с правами 0700.')
    state = json.loads(regular_file(path, 'source-build.json').read_text())
    require(state.get('schemaVersion') == 1 and state.get('repository') == REPOSITORY
            and state.get('work') == str(path) and re.fullmatch(r'[a-f0-9]{40}', state.get('commit', '')),
            'Неверные параметры продолжения сборки.')
    verify_checkout(path / 'source', state['commit'])
    return state


def existing_installation(wizard):
    """Offer the default saved installation before creating or rebuilding a workspace.

    Args:
        wizard: Installation APIs loaded from the verified source checkout.

    Returns:
        The confirmed existing root, or None for a separate fresh installation.
        An unrelated existing directory is never adopted or overwritten.
    """
    root = Path('/opt/meetspace')
    try:
        saved = wizard.inspect_existing_install(root)
    except RuntimeError as error:
        print(str(error))
        require(wizard.yes('Продолжить сборку для новой установки в другом каталоге?'),
                'Сборка не запущена. Существующий каталог не изменён.')
        return None
    if saved is None:
        return None
    if wizard.offer_existing_install(root) is not None:
        return root
    require(wizard.yes('Собрать отдельную новую установку в другом каталоге?'),
            'Установка отменена. Сохранённые файлы и данные не изменены.')
    return None


def build_and_bundle(work, state, wizard, requested_mode='auto'):
    """Build once, scan all images and resume only completed stages recorded in private state."""
    source = work / 'source'
    verify_checkout(source, state['commit'])
    prerequisites(wizard)
    target, info = resource_check(wizard, work, building=not state.get('built'))
    mode = select_build_mode(wizard, state, requested_mode, info)
    require(state.get('platform', target) == target, 'Архитектура сохранённой сборки отличается от сервера.')
    state['platform'] = target
    print(f'\nИсходники: {REPOSITORY}\nКоммит: {state["commit"]}\nПлатформа: {target}\n'
          'Backend, frontend и служебные образы будут собраны на этом сервере.\n'
          'Зависимости загружаются из публичных реестров. Все образы проходят проверку HIGH/CRITICAL.\n'
          'Сборка может занять продолжительное время; рекомендуется запускать мастер в tmux/screen.')
    require(wizard.yes('Начать или продолжить сборку из этих исходников?'), 'Сборка отменена.')
    if not state.get('built'):
        normalize_source_permissions(source, state['commit'])
        run_id = secrets.token_hex(6)
        state.update(version=f'v0.0.0-source.{state["commit"][:12]}.{run_id}',
                     registry='meetspace-source-' + run_id, build='build-' + run_id, built=False)
        save_state(work, state)
        print('\n[1/3] Сборка контейнеров из исходников GitHub')
        arguments = ['bash', str(source / 'scripts/release/build.sh'), '--version', state['version'],
                     '--registry', state['registry'], '--commit', state['commit'],
                     '--output', str(work / state['build']), '--layout', 'hardened-shared-host',
                     '--platform', target]
        if mode == 'low-memory':
            with low_memory_builder(work, state, info) as builder:
                command(*arguments, '--builder', builder, '--low-memory', work=work)
        else:
            command(*arguments, work=work)
        verify_checkout(source, state['commit'])
        state['built'] = True
        save_state(work, state)
    require(re.fullmatch(r'build-[a-f0-9]{12}', state.get('build', ''))
            and re.fullmatch(r'v0\.0\.0-source\.[a-f0-9]{12}\.[a-f0-9]{12}', state.get('version', '')),
            'Неверные параметры завершённой сборки.')
    build = json.loads(regular_file(work, state['build'] + '/build.json').read_text())
    require(build.get('commit') == state['commit'] and build.get('version') == state['version']
            and build.get('platform') == target and build.get('sourceDirty') is False,
            'Метаданные сохранённой сборки не совпадают.')
    if not state.get('packaged'):
        state['package'] = 'package-' + secrets.token_hex(6)
        save_state(work, state)
        print('\n[2/3] Проверка безопасности и сохранение собранных образов')
        command('bash', str(source / 'scripts/release/package.sh'), '--build', str(work / state['build'] / 'build.json'),
                '--output', str(work / state['package']), '--mode', 'archive',
                *(['--low-memory'] if mode == 'low-memory' else []), work=work)
        state['packaged'] = True
        save_state(work, state)
    require(re.fullmatch(r'package-[a-f0-9]{12}', state.get('package', '')), 'Неверный каталог пакета.')
    if not state.get('bundled'):
        state['bundle'] = 'bundle-' + secrets.token_hex(6)
        save_state(work, state)
        print('\n[3/3] Проверка контрольных сумм и подготовка мастера установки')
        verify_checkout(source, state['commit'])
        make_bundle(source, work / state['package'], work / state['bundle'], state)
        state['bundled'] = True
        save_state(work, state)
    require(re.fullmatch(r'bundle-[a-f0-9]{12}', state.get('bundle', '')), 'Неверный каталог установки.')
    bundle = work / state['bundle']
    wizard.metadata(bundle)
    command('bash', str(bundle / 'deployment/scripts/install/verify-release.sh'), str(bundle))
    return bundle


def main():
    """Dispatch a clean source checkout or retained work directory to the source-build workflow."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', nargs='?')
    parser.add_argument('--resume', metavar='WORK_DIRECTORY')
    parser.add_argument('--build-mode', choices=('auto', 'low-memory', 'standard'), default='auto')
    arguments = parser.parse_args()
    os.umask(0o077)
    require(os.geteuid() == 0 and sys.platform == 'linux', 'Нужен запуск от root на Linux.')
    require(not os.environ.get('DOCKER_HOST') and not os.environ.get('DOCKER_CONTEXT'),
            'Уберите DOCKER_HOST/DOCKER_CONTEXT: нужен локальный Docker.')
    require(bool(arguments.source) != bool(arguments.resume),
            'Укажите каталог исходников или --resume с каталогом сохранённой сборки.')
    work, resume_root = None, None
    try:
        with installation_lock():
            if arguments.resume:
                work = Path(arguments.resume)
                state = validate_work(work)
                source = work / 'source'
            else:
                source = Path(arguments.source)
                commit = verify_checkout(source)
            wizard = load_wizard(source)
            resume_root = existing_installation(wizard)
            if resume_root is None:
                if not arguments.resume:
                    work = Path(tempfile.mkdtemp(prefix='meetspace-source-build.', dir='/var/tmp'))
                    (work / 'scratch').mkdir(mode=0o700)
                    state = dict(schemaVersion=1, work=str(work), repository=REPOSITORY, commit=commit)
                    clone_for_build(source, work, commit)
                    save_state(work, state)
                    source = work / 'source'
                    wizard = load_wizard(source)
                bundle = build_and_bundle(work, state, wizard, arguments.build_mode)
        # The child acquires the same host lock itself; do not hold it across this call.
        if resume_root is not None:
            print('\nПродолжаем сохранённую установку. Новая сборка не запускалась; '
                  'мастер повторно проверит пакет и принадлежность ресурсов.')
            command(sys.executable, '-I', str(source / 'scripts/install/wizard.py'), '--resume', str(resume_root))
            return
        print('\nСборка и проверки завершены. Переходим к настройке сервера MeetSpace.')
        command(sys.executable, '-I', str(bundle / 'deployment/scripts/install/wizard.py'), str(bundle))
        print(f'\nИсходники и отчёты сборки сохранены в {work}.\n'
              'Они не удаляются автоматически; действующая установка хранит собственную проверенную копию.')
    except BaseException:
        if resume_root is not None:
            print(f'\nСохранённая установка не заменялась. Продолжение:\n'
                  f'sudo python3 -I {shlex.quote(str(source / "scripts/install/wizard.py"))} '
                  f'--resume {shlex.quote(str(resume_root))}', file=sys.stderr)
        elif work is not None and (work / 'source-build.json').is_file():
            entry = work / 'source/scripts/install/from-source.py'
            print(f'\nСборка и результаты проверок сохранены в {work}.\n'
                  'После устранения причины продолжить сборку:\n'
                  f'sudo python3 -I {shlex.quote(str(entry))} --resume {shlex.quote(str(work))}\n'
                  'Если мастер уже создал каталог установки, используйте команду продолжения установки, указанную им выше.',
                  file=sys.stderr)
        raise


if __name__ == '__main__':
    try:
        main()
    except (KeyboardInterrupt, EOFError):
        print('\nУстановка прервана пользователем.', file=sys.stderr)
        sys.exit(130)
    except (RuntimeError, OSError, ValueError, KeyError, TypeError, subprocess.CalledProcessError, tarfile.TarError) as error:
        print('\nУстановка из исходников остановлена: ' + (str(error) if not isinstance(error, subprocess.CalledProcessError)
              else 'служебная команда завершилась с ошибкой; см. сообщение выше.'), file=sys.stderr)
        sys.exit(1)
