#!/usr/bin/env python3
"""Interactive, fresh-host installation of a checksum-verified MeetSpace bundle."""

import base64
from email.message import EmailMessage
import fcntl
import getpass
import hashlib
import ipaddress
import json
import math
import os
from pathlib import Path
import re
import secrets
import shutil
import socket
import smtplib
import ssl
import stat
import subprocess
import sys
import time
import urllib.error
import urllib.request
import warnings


def run(*args, capture=False, data=None):
    """Run explicit arguments without a shell; capture only non-secret output."""
    # Compose otherwise gives the caller's exported variables precedence over --env-file.
    environment = {key: os.environ[key] for key in ('PATH', 'HOME', 'TERM', 'LANG', 'LC_ALL') if key in os.environ}
    result = subprocess.run(args, input=data, stdin=None if data is not None else subprocess.DEVNULL,
                            stdout=subprocess.PIPE if capture else None, text=True, check=True, env=environment)
    return result.stdout.strip() if capture else None


def ask(label, default="", validate=None):
    """Read and validate a terminal value using a label, optional default and validator."""
    while True:
        # A TTY is not seekable; buffered update mode ('r+') cannot open it.
        # Separate streams also keep prompts interactive when stdin is /dev/null.
        with open('/dev/tty', 'r', encoding='utf-8') as terminal_input, \
                open('/dev/tty', 'w', encoding='utf-8') as terminal_output:
            terminal_output.write(f"{label}" + (f" [{default}]" if default else "") + ": ")
            terminal_output.flush()
            line = terminal_input.readline()
        if not line:
            raise RuntimeError('Ввод прерван.')
        value = line.strip() or default
        if validate is None or validate(value):
            return value
        print('Проверьте значение и повторите ввод.')


def yes(label, default=False):
    """Request an explicit yes/no answer without assuming consent on EOF."""
    value = ask(label + ' (да/нет)', 'да' if default else 'нет',
                lambda s: s.lower() in ('да', 'нет', 'yes', 'no', 'y', 'n'))
    return value.lower() in ('да', 'yes', 'y')


def smtp_host_valid(value):
    """Validate value as a bare DNS hostname or an IP address, never a URL."""
    if not value or len(value) > 253 or any(char in value for char in '/\\\r\n\t @?#'):
        return False
    try:
        ipaddress.ip_address(value)
        return True
    except ValueError:
        return all(re.fullmatch(r'[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?', label)
                   for label in value.split('.'))


def smtp_email_valid(value):
    """Accept one ASCII mailbox in value without display names or header delimiters."""
    if len(value) > 254 or not value.isascii() or value.count('@') != 1:
        return False
    local, domain = value.rsplit('@', 1)
    return (bool(re.fullmatch(r"[A-Za-z0-9!#$%&'*+/=?^_`{|}~.-]+", local))
            and len(local) <= 64 and not local.startswith('.') and not local.endswith('.')
            and '..' not in local and domain_valid(domain.lower()))


def smtp_password():
    """Read an SMTP password from the terminal without allowing an echoed fallback."""
    while True:
        with warnings.catch_warnings():
            warnings.simplefilter('error', getpass.GetPassWarning)
            try:
                password = getpass.getpass('Пароль SMTP или пароль приложения (ввод скрыт): ')
            except getpass.GetPassWarning:
                raise RuntimeError('Для безопасного ввода SMTP-пароля нужен интерактивный терминал.') from None
        if password and len(password) <= 4096 and not any(char in password for char in '\r\n\x00'):
            return password
        print('Введите непустой пароль длиной до 4096 символов без переводов строки.')


def check_smtp(settings, recipient=''):
    """Verify TLS and AUTH PLAIN; send a test only when recipient was explicitly supplied.

    Args:
        settings: SMTP environment values held in memory; credentials are never logged.
        recipient: Validated, operator-approved mailbox for an optional test message.
    """
    context = ssl.create_default_context()
    context.minimum_version = ssl.TLSVersion.TLSv1_2
    client = None
    try:
        options = dict(host=settings['SMTP_HOST'], port=int(settings['SMTP_PORT']), timeout=15)
        if settings['SMTP_TLS_MODE'] == 'tls':
            client = smtplib.SMTP_SSL(**options, context=context)
            client.ehlo('localhost')
        else:
            client = smtplib.SMTP(**options)
            client.ehlo('localhost')
            if not client.has_extn('starttls'):
                raise RuntimeError('SMTP-сервер не поддерживает обязательный STARTTLS.')
            client.starttls(context=context)
            client.ehlo('localhost')
        if 'PLAIN' not in client.esmtp_features.get('auth', '').upper().split():
            raise RuntimeError('SMTP-сервер должен поддерживать AUTH PLAIN через защищённое соединение.')
        credentials = '\x00' + settings['SMTP_USERNAME'] + '\x00' + settings['SMTP_PASSWORD']
        response = base64.b64encode(credentials.encode('utf-8')).decode('ascii')
        code, _ = client.docmd('AUTH', 'PLAIN ' + response)
        if code == 334:
            code, _ = client.docmd(response)
        if code != 235:
            raise RuntimeError('SMTP-сервер отклонил вход. Проверьте логин и пароль приложения.')
        if recipient:
            message = EmailMessage()
            message['From'] = settings['SMTP_FROM']
            message['To'] = recipient
            message['Subject'] = 'MeetSpace — проверка отправки уведомлений'
            message.set_content('Отправка email-уведомлений MeetSpace настроена.\n'
                                'Приглашения и напоминания о встречах будут приходить по электронной почте.\n')
            client.send_message(message)
    except ssl.SSLError:
        raise RuntimeError('Не удалось проверить TLS-сертификат SMTP-сервера. Проверьте адрес и режим шифрования.') from None
    except (OSError, smtplib.SMTPException):
        # SMTP replies can contain credentials, addresses or provider diagnostics.
        raise RuntimeError('Проверка SMTP не выполнена. Проверьте доступность сервера, порт, '
                           'отправителя и разрешение SMTP у почтового провайдера.') from None
    finally:
        if client is not None:
            client.close()


def configure_smtp(domain):
    """Collect optional SMTP settings for domain and return them without persisting secrets."""
    print('\nEmail-уведомления: приглашение сразу и напоминание за 3 часа до встречи.')
    if not yes('Настроить отправку писем через SMTP сейчас?', default=True):
        print('Отправка email отключена. Настройки SMTP можно добавить после установки.')
        return None
    while True:
        settings = {'SMTP_HOST': ask('SMTP-сервер (без https:// и порта)', validate=smtp_host_valid)}
        print('Шифрование: tls — защищённое соединение с начала (обычно 465); '
              'starttls — обязательное переключение на TLS (обычно 587).')
        settings['SMTP_TLS_MODE'] = ask('Режим SMTP', 'tls', lambda value: value in ('tls', 'starttls'))
        settings['SMTP_PORT'] = ask('Порт SMTP', '465' if settings['SMTP_TLS_MODE'] == 'tls' else '587',
                                    lambda value: value.isascii() and value.isdigit() and 1 <= int(value) <= 65535)
        settings['SMTP_USERNAME'] = ask('Логин SMTP', validate=lambda value: bool(value) and len(value) <= 512
                                        and all(ord(char) >= 32 for char in value))
        settings['SMTP_PASSWORD'] = smtp_password()
        sender = ask('Email отправителя (разрешённый почтовым провайдером)', 'noreply@' + domain,
                     smtp_email_valid)
        settings.update(SMTP_FROM='MeetSpace <' + sender + '>', SMTP_TIMEOUT='15s')
        try:
            check_smtp(settings)
        except RuntimeError as error:
            print(str(error))
            if yes('Исправить настройки SMTP и повторить проверку?', default=True):
                continue
            if not yes('Сохранить непроверенные настройки SMTP и продолжить установку?'):
                raise RuntimeError('Настройка SMTP отменена.')
        else:
            print('Защищённое соединение и вход в SMTP проверены.')
            if yes('Отправить одно тестовое письмо для проверки доставки?'):
                recipient = ask('Email получателя тестового письма', sender, smtp_email_valid)
                try:
                    check_smtp(settings, recipient)
                    print('SMTP-сервер принял тестовое письмо. Проверьте входящие и папку «Спам».')
                except RuntimeError as error:
                    print(str(error))
                    if yes('Исправить настройки SMTP и повторить проверку?', default=True):
                        continue
                    if not yes('Продолжить установку с этими настройками SMTP?'):
                        raise RuntimeError('Настройка SMTP отменена.')
        return settings


def dotenv_literal(value):
    """Quote a single-line value for Compose env files without expanding dollar references."""
    if any(char in value for char in '\r\n\x00'):
        raise ValueError('Значение настройки содержит недопустимый управляющий символ.')
    return json.dumps(value, ensure_ascii=False).replace('$', '$$')


def write(path, text, mode=0o600):
    """Create a new private file; existing files must never be overwritten implicitly."""
    with path.open('x') as output:
        output.write(text)
    path.chmod(mode)


def digest(path):
    """Return a streaming SHA-256 digest without loading image archives into memory."""
    result = hashlib.sha256()
    with path.open('rb') as source:
        for block in iter(lambda: source.read(1024 * 1024), b''):
            result.update(block)
    return result.hexdigest()


def warn_disk_space(location, recommended_bytes):
    """Warn, without aborting, when location has less than recommended_bytes available."""
    available = shutil.disk_usage(location).free
    if available < recommended_bytes:
        print(f'Предупреждение: в {location} свободно {available / 1024**3:.1f} ГиБ; '
              f'рекомендуется не менее {recommended_bytes / 1024**3:.1f} ГиБ.')
        print('Продолжение разрешено. Если место закончится, сборка или установка '
              'может завершиться ошибкой и повлиять на другие службы этого диска.')


def verify_inventory(root, expected):
    """Validate an exact relative-path/SHA-256 inventory of ordinary files under root."""
    assert root.is_dir() and not root.is_symlink() and isinstance(expected, dict) and expected
    for name, value in expected.items():
        path = Path(name)
        assert isinstance(name, str) and str(path) == name and not path.is_absolute()
        assert '..' not in path.parts and '\\' not in name and all(ord(c) >= 32 for c in name)
        assert isinstance(value, str) and re.fullmatch('[a-f0-9]{64}', value)
        target = root / path
        assert not any(p.is_symlink() for p in (target, *target.parents))
        assert target.is_file() and digest(target) == value
    entries = list(root.rglob('*'))
    assert all(not p.is_symlink() and (p.is_file() or p.is_dir()) for p in entries)
    actual = {str(p.relative_to(root)) for p in entries if p.is_file()}
    assert actual == set(expected)


def metadata(bundle):
    """Check release, deployment and optional GitHub source inventories on every run."""
    info = json.loads((bundle / 'bundle.json').read_text())
    assert info['schemaVersion'] == 1
    assert digest(bundle / 'release/release.json') == info['manifestSHA256']
    verify_inventory(bundle / 'deployment', info['deploymentSHA256'])
    manifest = json.loads((bundle / 'release/release.json').read_text())
    assert info['version'] == manifest['version'] and info['platform'] == manifest['platform']
    if 'source' in info or (bundle / 'source').exists():
        source = info['source']
        assert source['repository'] == 'https://github.com/janickiy/meet-space.git'
        assert re.fullmatch('[a-f0-9]{40}', source['commit']) and source['commit'] == manifest['commit']
        assert not any(Path(name).parts[0] == '.git' for name in source['filesSHA256'])
        verify_inventory(bundle / 'source', source['filesSHA256'])
    return manifest


def apt(*packages):
    """Install named prerequisite packages only when apt is available."""
    if not shutil.which('apt-get'):
        raise RuntimeError('Для автоматической установки пакетов нужен Ubuntu/Debian; установите зависимости вручную.')
    run('apt-get', 'update')
    run('apt-get', 'install', '-y', *packages)


def docker_setup():
    """Offer Docker installation from its official Ubuntu/Debian repository."""
    if os.environ.get('DOCKER_HOST') or os.environ.get('DOCKER_CONTEXT'):
        raise RuntimeError('Уберите DOCKER_HOST/DOCKER_CONTEXT: мастер работает только с локальным Docker.')
    if not shutil.which('docker'):
        if not yes('Docker не найден. Установить Docker Engine и Compose из официального репозитория?'):
            raise RuntimeError('Docker необходим для установки.')
        release = {}
        for line in Path('/etc/os-release').read_text().splitlines():
            if '=' in line:
                key, value = line.split('=', 1)
                release[key] = value.strip('"')
        distro, codename = release.get('ID'), release.get('VERSION_CODENAME', '')
        if distro not in ('ubuntu', 'debian') or not re.fullmatch('[a-z]+', codename):
            raise RuntimeError('Автоустановка Docker поддерживается на Ubuntu и Debian.')
        conflicting = run('dpkg-query', '-W', '-f=${binary:Package} ${db:Status-Abbrev}\n', capture=True)
        if any(re.match(r'(docker.io|podman-docker|containerd|runc)(:| )', line) and ' ii ' in ' ' + line
               for line in conflicting.splitlines()):
            raise RuntimeError('Обнаружены альтернативные пакеты Docker/containerd. Сначала настройте Docker по официальной инструкции.')
        apt('ca-certificates', 'curl', 'gnupg')
        key = Path('/etc/apt/keyrings/meetspace-docker.asc')
        sources = Path('/etc/apt/sources.list.d/meetspace-docker.list')
        if key.exists() or sources.exists():
            raise RuntimeError('Конфигурация репозитория Docker уже существует; завершите установку Docker и повторите запуск.')
        if not key.parent.exists():
            key.parent.mkdir(mode=0o755, parents=True)
            # The installer uses umask 077; apt's _apt user must traverse this directory.
            key.parent.chmod(0o755)
        if key.parent.stat().st_mode & 0o005 != 0o005:
            raise RuntimeError('/etc/apt/keyrings должен быть доступен для чтения apt (обычно права 0755).')
        run('curl', '--fail', '--show-error', '--location', '--proto', '=https', '--proto-redir', '=https',
            '--connect-timeout', '20', '--max-time', '120', '--output', str(key),
            f'https://download.docker.com/linux/{distro}/gpg')
        key.chmod(0o644)
        fingerprint = run('gpg', '--show-keys', '--with-colons', str(key), capture=True)
        if '9DC858229FC7DD38854AE2D88D81803C0EBFCD88' not in fingerprint:
            raise RuntimeError('Отпечаток ключа репозитория Docker не совпадает.')
        arch = run('dpkg', '--print-architecture', capture=True)
        if arch not in ('amd64', 'arm64'):
            raise RuntimeError('Неподдерживаемая архитектура Docker.')
        write(sources, f'deb [arch={arch} signed-by={key}] https://download.docker.com/linux/{distro} {codename} stable\n', 0o644)
        apt('docker-ce', 'docker-ce-cli', 'containerd.io', 'docker-buildx-plugin', 'docker-compose-plugin')
        run('systemctl', 'enable', '--now', 'docker')
    require_local_docker()
    version = run('docker', 'compose', 'version', '--short', capture=True).lstrip('v')
    parsed = re.match(r'(\d+)\.(\d+)\.(\d+)', version)
    if not parsed or tuple(map(int, parsed.groups())) < (2, 24, 4):
        raise RuntimeError('Нужен Docker Compose 2.24.4 или новее.')
    info = json.loads(run('docker', 'info', '--format', '{{json .}}', capture=True))
    if info['OSType'] != 'linux' or any('rootless' in str(x) for x in info.get('SecurityOptions', [])):
        raise RuntimeError('Нужен обычный Docker Engine на Linux (не rootless).')


def require_local_docker():
    """Reject a persisted remote Docker context before any installation or management."""
    if os.environ.get('DOCKER_HOST') or os.environ.get('DOCKER_CONTEXT'):
        raise RuntimeError('Уберите DOCKER_HOST/DOCKER_CONTEXT: требуется локальный Docker.')
    endpoint = run('docker', 'context', 'inspect', '--format', '{{.Endpoints.docker.Host}}', capture=True)
    if not endpoint.startswith('unix://'):
        raise RuntimeError('Выберите локальный Docker context с Unix-сокетом.')


def require_image_store(manifest):
    """Require containerd storage for OCI bundles referenced by manifest digest."""
    if not manifest.get('imageConfigIDs'):
        return
    info = json.loads(run('docker', 'info', '--format', '{{json .}}', capture=True))
    if not any(item == ['driver-type', 'io.containerd.snapshotter.v1']
               for item in info.get('DriverStatus', [])):
        raise RuntimeError('Для этого OCI-пакета нужен Docker с containerd image store '
                           '(стандарт новой установки Docker Engine 29). '
                           'Мастер не переключает хранилище существующего Docker автоматически.')


def domain_valid(value):
    """Accept a DNS hostname, excluding URLs, IP literals and shell metacharacters."""
    return not re.fullmatch(r'[0-9.]+', value) and len(value) <= 253 and '.' in value and all(
        re.fullmatch(r'[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?', label) for label in value.split('.'))


def public_ip(value):
    """Accept an assigned, globally routable IPv4 address."""
    try:
        address = ipaddress.ip_address(value)
        return address.version == 4 and address.is_global and not address.is_multicast
    except ValueError:
        return False


def detect_server_ipv4(domain):
    """Return the unique public IPv4 shared by domain DNS and a local interface.

    Args:
        domain: The validated MeetSpace hostname whose single A record must
            point directly to this server, without NAT or a CDN proxy.

    Multiple local addresses are allowed: DNS selects the intended interface.
    No external IP service or interactive address prompt is used. Invalid or
    ambiguous network data raises RuntimeError before installation is changed.
    """
    try:
        interfaces = json.loads(run('ip', '-j', '-4', 'addr', capture=True))
    except (json.JSONDecodeError, subprocess.CalledProcessError):
        raise RuntimeError('Не удалось определить IPv4 сетевых интерфейсов сервера.') from None
    if not isinstance(interfaces, list):
        raise RuntimeError('Не удалось прочитать список сетевых интерфейсов сервера.')
    local_ips = set()
    for interface in interfaces:
        if not isinstance(interface, dict) or not isinstance(interface.get('addr_info', []), list):
            raise RuntimeError('Не удалось прочитать адреса сетевых интерфейсов сервера.')
        for entry in interface.get('addr_info', []):
            if not isinstance(entry, dict) or not isinstance(entry.get('local'), str):
                raise RuntimeError('Не удалось прочитать IPv4 сетевого интерфейса сервера.')
            if public_ip(entry['local']):
                local_ips.add(entry['local'])
    if not local_ips:
        raise RuntimeError('На интерфейсах сервера не найден публичный IPv4. '
                           'Схема за NAT требует отдельной настройки TURN.')
    try:
        resolved = {record[4][0] for record in socket.getaddrinfo(domain, 443, socket.AF_INET)}
    except socket.gaierror:
        raise RuntimeError('Не удалось определить A-запись домена. '
                           'Проверьте DNS и повторите запуск.') from None
    if len(resolved) != 1 or not all(public_ip(address) for address in resolved):
        raise RuntimeError('У домена должна быть ровно одна A-запись с публичным IPv4 этого сервера.')
    address = next(iter(resolved))
    if address not in local_ips:
        raise RuntimeError('A-запись домена не совпадает с публичным IPv4 на интерфейсе сервера. '
                           'Направьте домен на этот сервер без CDN-прокси; '
                           'схема за NAT требует отдельной настройки TURN.')
    try:
        ipv6 = socket.getaddrinfo(domain, 443, socket.AF_INET6)
    except socket.gaierror as error:
        missing = {socket.EAI_NONAME, getattr(socket, 'EAI_NODATA', socket.EAI_NONAME)}
        if error.errno not in missing:
            raise RuntimeError('Не удалось проверить AAAA-запись домена. '
                               'Проверьте DNS и повторите запуск.') from None
        ipv6 = []
    if ipv6:
        raise RuntimeError('Этот мастер настраивает IPv4. Уберите AAAA-запись домена перед установкой.')
    return address


def safe_root(value):
    """Require a new, canonical installation directory below /opt or /srv."""
    path = Path(value)
    return valid_installation_path(value) and not path.exists() and not path.is_symlink()


def valid_installation_path(value):
    """Accept a canonical, non-symlink path below /opt or /srv, including existing paths.

    Args:
        value: Absolute installation directory entered by the operator.
    """
    path = Path(value)
    return (bool(re.fullmatch(r'/(opt|srv)/[A-Za-z0-9_-]+(?:/[A-Za-z0-9_-]+)*', value))
            and path.resolve() == path and not path.is_symlink())


def inspect_existing_install(root):
    """Read a private installer-owned directory without changing its files or resources.

    Args:
        root: Canonical installation directory to inspect.

    Returns:
        The saved state and verified manifest, or None when the directory is absent.
        Existing directories that cannot be safely resumed raise RuntimeError.
    """
    if not valid_installation_path(str(root)):
        raise RuntimeError('Укажите обычный абсолютный каталог внутри /opt или /srv без символических ссылок.')
    if not root.exists():
        return None
    attributes = root.stat()
    if not root.is_dir() or attributes.st_uid != 0 or attributes.st_mode & 0o077:
        raise RuntimeError(f'Каталог {root} уже существует и не является закрытым каталогом установщика '
                           '(требуются владелец root и права 0700). Выберите новый каталог; существующие файлы не изменены.')
    state_path = root / 'installation.json'
    try:
        attributes = state_path.lstat()
        if not stat.S_ISREG(attributes.st_mode) or attributes.st_uid != 0 or attributes.st_mode & 0o077:
            raise ValueError('Unsafe installation state')
        state = json.loads(state_path.read_text())
        if (not isinstance(state, dict) or state.get('schemaVersion') != 1 or state.get('root') != str(root)
                or not isinstance(state.get('complete'), bool)
                or not isinstance(state.get('domain'), str) or not domain_valid(state['domain'])
                or not isinstance(state.get('ip'), str) or not public_ip(state['ip'])
                or not isinstance(state.get('project'), str)
                or not re.fullmatch(r'meetspace(?:-[a-z0-9-]+)?', state['project'])
                or not isinstance(state.get('token'), str) or not re.fullmatch(r'[a-f0-9]{32}', state['token'])
                or not valid_subnet(state.get('subnet', '')) or state.get('tls') not in ('1', '2')
                or not isinstance(state.get('email'), str) or not isinstance(state.get('certificateDirectory'), str)):
            raise ValueError('Invalid installation state')
        manifest = metadata(root / 'bundle')
        if state.get('sourceCommit', manifest['commit']) != manifest['commit']:
            raise ValueError('Saved source differs from release')
    except (OSError, ValueError, KeyError, TypeError, AssertionError):
        raise RuntimeError(f'Каталог {root} уже существует, но состояние установки или контрольные суммы не прошли проверку. '
                           'Он не будет перезаписан. Выберите новый каталог либо проверьте сохранённую установку.') from None
    return state, manifest


def offer_existing_install(root, requested_manifest=None, expected_domain=None):
    """Offer explicit continuation of a verified incomplete install, never an upgrade.

    Args:
        root: Existing private installation directory, or an unused default path.
        requested_manifest: Optional new release metadata to contrast with the saved release.
        expected_domain: Optional hostname that must match the saved installation.

    Returns:
        Saved state and manifest only after confirmation; None if absent or declined.
    """
    saved = inspect_existing_install(root)
    if saved is None:
        return None
    state, manifest = saved
    print(f'\nНайдена установка MeetSpace: https://{state["domain"]}\nКаталог: {root}\n'
          f'Сохранённая версия: {manifest["version"]}\nКоммит: {manifest["commit"]}')
    if state['complete']:
        raise RuntimeError(f'MeetSpace в {root} уже установлен. Состояние: sudo {root}/manage.sh status. '
                           'Для обновления используйте процедуру выпуска; мастер не перезаписывает действующую установку.')
    if expected_domain is not None and state['domain'] != expected_domain:
        raise RuntimeError(f'Сохранённая установка относится к {state["domain"]}, а не к {expected_domain}. '
                           'Выберите другой каталог или продолжите установку для сохранённого домена.')
    if requested_manifest is not None and (manifest.get('version'), manifest.get('commit')) != (
            requested_manifest.get('version'), requested_manifest.get('commit')):
        print('Внимание: сохранённая версия отличается от выбранных исходников или нового пакета.')
    print('Продолжение использует ранее сохранённый пакет, настройки, сертификат и данные. '
          'Новый пакет не заменит существующую версию.')
    require_local_docker()
    check_project(state['project'], state['token'])
    if yes('Продолжить эту незавершённую установку без повторной сборки?', default=True):
        return saved
    return None


def check_ports(include_http=False, include_grafana=False):
    """Bind every required TCP/UDP port briefly, without changing firewall rules."""
    sockets = []
    tcp = [443, 3478, 5349, 50010, 19090] + ([80] if include_http else []) + ([13001] if include_grafana else [])
    udp = [3478, 50010] + list(range(49160, 49260))
    required = {f'{port}/tcp' for port in tcp} | {f'{port}/udp' for port in udp}
    containers = run('docker', 'container', 'ls', '-q', capture=True).splitlines()
    if containers:
        ports = run('docker', 'inspect', '--format', '{{json .NetworkSettings.Ports}}', *containers, capture=True)
        for line in ports.splitlines():
            for target, bindings in (json.loads(line) or {}).items():
                protocol = target.rsplit('/', 1)[-1]
                if any(f'{binding["HostPort"]}/{protocol}' in required for binding in bindings or []):
                    raise RuntimeError('Один из необходимых портов уже опубликован другим контейнером Docker.')
    try:
        for kind, ports in ((socket.SOCK_STREAM, tcp), (socket.SOCK_DGRAM, udp)):
            for port in ports:
                sock = socket.socket(socket.AF_INET, kind)
                sockets.append(sock)
                try:
                    sock.bind(('0.0.0.0', port))
                except OSError:
                    raise RuntimeError(f'Порт {port}/{"tcp" if kind == socket.SOCK_STREAM else "udp"} занят.') from None
    finally:
        for sock in sockets:
            sock.close()


def check_project(project, token=None):
    """Reject existing Compose resources unless they belong to this incomplete install."""
    for kind in ('container', 'volume', 'network'):
        names = set(run('docker', kind, 'ls', '-q', '--filter', f'label=com.docker.compose.project={project}',
                        *(['-a'] if kind == 'container' else []), capture=True).splitlines())
        # Compose can adopt an unlabelled volume/network with its conventional name.
        if kind in ('volume', 'network'):
            candidates = {f'{project}_{name}' for name in
                          (('postgres', 'rabbitmq', 'minio', 'objects', 'prometheus', 'grafana') if kind == 'volume' else ('private',))}
            names.update(candidates.intersection(run('docker', kind, 'ls', '--format', '{{.Name}}', capture=True).splitlines()))
        for name in sorted(names):
            label = run('docker', kind, 'inspect', '--format',
                        '{{index .Config.Labels "io.meetspace.install-id"}}' if kind == 'container'
                        else '{{index .Labels "io.meetspace.install-id"}}', name, capture=True)
            if not token or label != token:
                resource = {'container': 'контейнер', 'volume': 'том', 'network': 'сеть'}[kind]
                raise RuntimeError(f'Проект {project} уже содержит посторонние ресурсы: {resource} '
                                   f'{json.dumps(name, ensure_ascii=False)} не принадлежит этой попытке установки. '
                                   'Для продолжения прежней установки используйте её сохранённый каталог с --resume. '
                                   'Для отдельной установки выберите другое имя проекта. Данные не изменены.')


def validate_certificate(directory, domain):
    """Check certificate hostname, expiry and matching public keys before copying."""
    cert, key = directory / 'fullchain.pem', directory / 'privkey.pem'
    if not cert.is_file() or not key.is_file():
        raise RuntimeError('В каталоге сертификата нужны fullchain.pem и privkey.pem.')
    run('openssl', 'x509', '-in', str(cert), '-noout', '-checkend', '86400')
    run('openssl', 'x509', '-in', str(cert), '-noout', '-checkhost', domain)
    cert_key = run('openssl', 'x509', '-in', str(cert), '-pubkey', '-noout', capture=True)
    private_key = run('openssl', 'pkey', '-in', str(key), '-passin', 'pass:', '-pubout', capture=True)
    if cert_key != private_key:
        raise RuntimeError('Сертификат и приватный ключ не соответствуют друг другу.')


def configure(bundle, manifest):
    """Collect installation choices, then create only the newly confirmed directory."""
    print('\n[3/7] Параметры MeetSpace')
    if not shutil.which('ip'):
        if yes('Установить iproute2 для проверки сетевого адреса?'):
            apt('iproute2')
        else:
            raise RuntimeError('Для проверки IP нужен iproute2.')
    domain = ask('Домен MeetSpace (например meet.company.ru)', validate=domain_valid)
    address = detect_server_ipv4(domain)
    print(f'Публичный IPv4 определён автоматически: {address}')
    while True:
        root = Path(ask('Каталог установки', '/opt/meetspace'))
        if safe_root(str(root)):
            break
        try:
            saved = offer_existing_install(root, manifest, expected_domain=domain)
        except RuntimeError as error:
            print(str(error))
        else:
            if saved is not None:
                return root, saved[0]
            print('Выберите другой новый каталог для отдельной установки.')
    project = ask('Имя Docker Compose проекта', 'meetspace', lambda s: bool(re.fullmatch(r'meetspace(?:-[a-z0-9-]+)?', s)))
    check_project(project)
    subnet = ask('Отдельная Docker-подсеть /24', '172.30.240.0/24', valid_subnet)
    occupied = run('docker', 'network', 'ls', '-q', capture=True).splitlines()
    if occupied:
        for network in json.loads(run('docker', 'network', 'inspect', *occupied, capture=True)):
            for config in network.get('IPAM', {}).get('Config', []) or []:
                existing = config.get('Subnet')
                if existing and ipaddress.ip_network(existing).version == 4 and ipaddress.ip_network(subnet).overlaps(ipaddress.ip_network(existing)):
                    raise RuntimeError('Выбранная подсеть пересекается с существующей сетью Docker.')
    for route in json.loads(run('ip', '-j', '-4', 'route', 'show', capture=True)):
        network = route.get('dst', 'default')
        if network != 'default' and ipaddress.ip_network(subnet).overlaps(ipaddress.ip_network(network, strict=False)):
            raise RuntimeError('Выбранная подсеть пересекается с сетевым маршрутом сервера.')
    print('\nHTTPS: 1 — получить сертификат Let’s Encrypt; 2 — использовать готовый сертификат.')
    tls = ask('Выберите вариант', '1', lambda s: s in ('1', '2'))
    email, cert_dir = '', None
    if tls == '1':
        email = ask('Email для Let’s Encrypt', validate=lambda s: bool(re.fullmatch(r'[^\s@]+@[^\s@]+\.[^\s@]+', s)))
        print('Будет принят договор Let’s Encrypt: https://letsencrypt.org/repository/.')
        if not yes('Согласны получить сертификат и настроить его автоматическое продление?', default=True):
            raise RuntimeError('Получение сертификата отменено.')
        if not shutil.which('certbot'):
            apt('certbot')
    else:
        cert_dir = Path(ask('Абсолютный каталог fullchain.pem и privkey.pem', validate=lambda s: s.startswith('/')))
        validate_certificate(cert_dir, domain)
    smtp = configure_smtp(domain)
    check_ports(tls == '1', manifest.get('deploymentLayout') == 'classic')
    info = json.loads(run('docker', 'info', '--format', '{{json .}}', capture=True))
    if info.get('NCPU', 0) < 4 or info.get('MemTotal', 0) < 8 * 1024**3:
        print('Рекомендуется не менее 4 CPU и 8 ГБ RAM; при записи ресурсы особенно важны.')
        if not yes('Продолжить на этом сервере?'):
            raise RuntimeError('Установка отменена.')
    required = sum(p.stat().st_size for p in bundle.rglob('*') if p.is_file()) + 15 * 1024**3
    ancestor = root.parent
    while not ancestor.exists():
        ancestor = ancestor.parent
    warn_disk_space(ancestor, required)
    warn_disk_space(Path(info['DockerRootDir']), 15 * 1024**3)
    print(f'\nВерсия: {manifest["version"]}\nСайт: https://{domain}\nIP: {address}\nКаталог: {root}\nПроект: {project}')
    print('Email-уведомления: ' + ('SMTP настроен; напоминание за 3 часа.' if smtp else 'отключены.'))
    print('Откройте во внешнем firewall: TCP 80,443,3478,5349,50010; UDP 3478,50010,49160–49259.\n'
          'Мастер не меняет firewall и не открывает наружу базы данных.')
    ask('Нажмите Enter, чтобы начать установку (Ctrl+C — отмена)', validate=lambda s: s == '')
    root.mkdir(mode=0o700, parents=True)
    shutil.copytree(bundle, root / 'bundle')
    # Configuration mounts must be readable by unprivileged container users.
    deployment = root / 'bundle/deployment'
    deployment.chmod(0o755)
    for path in deployment.rglob('*'):
        path.chmod(0o755 if path.is_dir() else 0o644)
    for name in ('config', 'certs', 'secrets', 'storage'):
        (root / name).mkdir(mode=0o700)
    (root / 'storage').chmod(0o750)
    os.chown(root / 'storage', 1000, 1000)
    # Container bind mounts resolve the leaf on the host; parent stays root-only.
    (root / 'certs').chmod(0o750)
    os.chown(root / 'certs', 0, 1000)
    state = dict(schemaVersion=1, root=str(root), domain=domain, ip=address, project=project,
                 subnet=subnet, token=secrets.token_hex(16), tls=tls, email=email,
                 certificateDirectory=str(cert_dir) if cert_dir else '', complete=False)
    if (root / 'bundle/source').is_dir():
        state.update(sourceRepository='https://github.com/janickiy/meet-space.git', sourceCommit=manifest['commit'])
    make_configuration(root, manifest, state, smtp)
    write(root / 'installation.json', json.dumps(state, indent=2) + '\n')
    print(f'Конфигурация сохранена в {root}. Пароли созданы автоматически.')
    return root, state


def valid_subnet(value):
    """Allow an RFC1918 /24 for the isolated Compose network."""
    try:
        network = ipaddress.ip_network(value)
        return network.version == 4 and network.prefixlen == 24 and any(
            network.subnet_of(ipaddress.ip_network(private)) for private in ('10.0.0.0/8', '172.16.0.0/12', '192.168.0.0/16'))
    except ValueError:
        return False


def make_configuration(root, manifest, state, smtp=None):
    """Generate private configuration and Compose overlays for the verified installation.

    Args:
        root: Newly created, root-owned installation directory.
        manifest: Verified release metadata with immutable service image references.
        state: Non-secret installation choices persisted for resumable deployment.
        smtp: Optional SMTP values, stored separately and only passed to email services.
    """
    cpu_count = json.loads(run('docker', 'info', '--format', '{{json .}}', capture=True)).get('NCPU')
    if not isinstance(cpu_count, int) or isinstance(cpu_count, bool) or cpu_count < 1:
        raise RuntimeError('Docker не сообщил допустимое количество CPU сервера.')
    domain, address = state['domain'], state['ip']
    values = {key: secrets.token_hex(32) for key in (
        'JWT_SECRET', 'MEDIA_TICKET_SECRET', 'MEDIA_INTERNAL_SECRET', 'WORKER_INTERNAL_SECRET',
        'METRICS_SECRET', 'TURN_SHARED_SECRET', 'POSTGRES_PASSWORD', 'REDIS_PASSWORD',
        'RABBIT_MQ_PASSWORD', 'MINIO_ROOT_PASSWORD')}
    values.update({key: 'meetspace' for key in ('POSTGRES_USER', 'POSTGRES_DB', 'RABBIT_MQ_USER', 'MINIO_ROOT_USER')})
    values.update({key: 'https://' + domain for key in ('PUBLIC_FRONTEND_URL', 'MINIO_PUBLIC_ENDPOINT', 'WS_ALLOWED_ORIGINS')})
    values.update(MEDIA_NAT_IPS=address, TURN_PUBLIC_IP=address, TURN_REALM=domain,
                  TURN_URLS=f'turn:{domain}:3478?transport=udp,turn:{domain}:3478?transport=tcp,turns:{domain}:5349?transport=tcp',
                  HTTP_TRUSTED_PROXIES=state['subnet'], BUILD_VERSION=manifest['version'],
                  RECORDER_STORAGE_DIR=str(root / 'storage'), TLS_CERT_DIR=str(root / 'certs'),
                  TURN_TLS_CERT_DIR=str(root / 'certs'), PROVIDER_TOKEN_ENCRYPTION_KEY='',
                  DB_MAX_OPEN='10', DB_MAX_IDLE='2', REMINDER_OFFSETS='3h')
    template = (root / 'bundle/deployment/.env.production.example').read_text()
    lines = []
    for line in template.splitlines():
        key = line.split('=', 1)[0]
        lines.append(f'{key}={values[key]}' if key in values else line)
    lines.extend([f'RELEASE_ENV_FILE={root}/config/meetspace.env', 'RELEASE_APP_ENV=production',
                  f'METRICS_TOKEN_FILE={root}/secrets/metrics_token', f'GRAFANA_PASSWORD_FILE={root}/secrets/grafana_password'])
    for service, image in manifest['images'].items():
        if not re.fullmatch(r'[a-z][a-z-]*', service) or not re.fullmatch(r'sha256:[a-f0-9]{64}', image):
            raise RuntimeError('Некорректная ссылка на образ выпуска.')
        lines.append(f'{service.upper().replace("-", "_")}_IMAGE={image}')
    lines.append('OBJECT_STORAGE_IMAGE=' + manifest['images']['minio'])
    write(root / 'config/meetspace.env', '\n'.join(lines) + '\n')
    # Only the individual bind-mounted files are readable by container users.
    write(root / 'secrets/metrics_token', values['METRICS_SECRET'], 0o644)
    write(root / 'secrets/grafana_password', secrets.token_hex(24), 0o644)
    labels = {'io.meetspace.install-id': state['token']}
    services = {name: {'labels': labels} for name in manifest['images']}
    services['migrate'] = {'labels': labels}
    if smtp:
        smtp_path = root / 'config/smtp.env'
        write(smtp_path, ''.join(f'{key}={dotenv_literal(value)}\n' for key, value in smtp.items()))
        for name in ('api', 'product-worker'):
            services[name].update(env_file=[str(smtp_path)], environment={'EMAIL_PROVIDER_MODE': 'smtp'})
    services['proxy'].update(volumes=[f'{root}/config/nginx.conf:/etc/nginx/conf.d/default.conf:ro'])
    # Render the verified release with this installation's private environment.
    # Only CPU quotas above the host capacity are lowered; existing smaller limits remain.
    release = json.loads(run(*compose_base_args(root, manifest, state), '--profile', '*',
                             'config', '--format', 'json', capture=True))
    for name, quota in bounded_cpu_limits(release['services'], cpu_count).items():
        services.setdefault(name, {'labels': labels})['cpus'] = quota
    # Use host-network TURN to avoid hairpin NAT while retaining private-peer denial.
    override = json.dumps({'services': services, 'volumes': {name: {'labels': labels} for name in
                          ('postgres', 'rabbitmq', 'minio', 'objects', 'prometheus', 'grafana')},
                          'networks': {'private': {'labels': labels, 'ipam': {'config': [{'subnet': state['subnet']}]}}}}, indent=2)
    write(root / 'config/compose.json', override + '\n')
    write(root / 'config/turn.yml', '''services:
  coturn:
    user: "1000:1000"
    network_mode: host
    networks: !reset []
    ports: !reset []
    environment:
      TURN_BIND_PUBLIC: "true"
''')
    write(root / 'config/nginx.conf', '''map $http_upgrade $connection_upgrade { default upgrade; '' close; }
log_format safe '$remote_addr $request_method $uri $status';
server {
    listen 443 ssl;
    server_name _;
    ssl_certificate /run/certs/fullchain.pem;
    ssl_certificate_key /run/certs/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    server_tokens off;
    client_max_body_size 1m;
    client_body_timeout 30s;
    access_log /var/log/nginx/access.log safe;
    resolver 127.0.0.11 valid=5s ipv6=off;
    set $api_upstream api:8085;
    set $frontend_upstream frontend:8080;
    add_header Strict-Transport-Security "max-age=31536000" always;
    add_header X-Content-Type-Options nosniff always;
    add_header Referrer-Policy no-referrer always;
    include /etc/nginx/snippets/recorder-routes.conf;
}
''', 0o644)
    write(root / 'manage.sh', f'''#!/usr/bin/env bash
# Run the installation's fixed Compose configuration; do not print its secrets.
set -euo pipefail
exec python3 -I '{root}/bundle/deployment/scripts/install/wizard.py' --manage '{root}' "$@"
''', 0o700)


def bounded_cpu_limits(services, cpu_count):
    """Return reduced CPU quotas for rendered services exceeding cpu_count available CPUs."""
    limits = {}
    for name, configuration in services.items():
        if 'cpus' not in configuration:
            continue
        quota = float(configuration['cpus'])
        if not math.isfinite(quota) or quota < 0:
            raise RuntimeError('Некорректное ограничение CPU в конфигурации службы: ' + name)
        if quota > cpu_count:
            limits[name] = cpu_count
    return limits


def compose_base_args(root, manifest, state):
    """Select the verified release layout for root using manifest and installation state."""
    deploy = root / 'bundle/deployment'
    args = ['docker', 'compose', '--project-name', state['project'], '--env-file', str(root / 'config/meetspace.env'),
            '-f', str(deploy / 'docker-compose.production.yml')]
    layout = 'docker-compose.hardened.yml' if manifest.get('deploymentLayout') == 'hardened-shared-host' else 'docker-compose.grafana.yml'
    args.extend(['-f', str(deploy / layout)])
    return args


def compose_args(root, manifest, state):
    """Add root's generated installation overlays to the verified release Compose arguments."""
    return [*compose_base_args(root, manifest, state), '-f', str(root / 'config/compose.json'),
            '-f', str(root / 'config/turn.yml')]


def certificates(root, state):
    """Issue/copy TLS credentials and install a renewal hook for this installation."""
    print('\n[4/7] HTTPS-сертификат')
    source = Path(state['certificateDirectory']) if state['tls'] == '2' else Path('/etc/letsencrypt/live') / ('meetspace-' + state['token'])
    if state['tls'] == '1' and not (source / 'fullchain.pem').exists():
        run('certbot', 'certonly', '--standalone', '--non-interactive', '--agree-tos',
            '--email', state['email'], '--cert-name', source.name, '-d', state['domain'])
    validate_certificate(source, state['domain'])
    for name, mode in (('fullchain.pem', 0o644), ('privkey.pem', 0o640)):
        target = root / 'certs' / name
        shutil.copyfile(source / name, target)
        os.chown(target, 0, 1000)
        target.chmod(mode)
    if state['tls'] == '1':
        hook = Path('/etc/letsencrypt/renewal-hooks/deploy') / ('meetspace-' + state['token'])
        if not hook.exists():
            hook.parent.mkdir(parents=True, exist_ok=True)
            # All interpolated paths are constrained by safe_root or generated tokens.
            write(hook, f'''#!/usr/bin/env bash
set -euo pipefail
[[ "${{RENEWED_LINEAGE:-}}" == '{source}' ]] || exit 0
install -o root -g 1000 -m 0644 '{source}/fullchain.pem' '{root}/certs/fullchain.pem'
install -o root -g 1000 -m 0640 '{source}/privkey.pem' '{root}/certs/privkey.pem'
'{root}/manage.sh' reload-tls
''', 0o700)
        run('systemctl', 'enable', '--now', 'certbot.timer')


def verify_public(state, manifest):
    """Verify trusted HTTPS and both public build identifiers with bounded retries."""
    for attempt in range(12):
        try:
            for suffix in ('/', '/version', '/version.json'):
                with urllib.request.urlopen('https://' + state['domain'] + suffix, timeout=15) as response:
                    if response.status != 200:
                        raise ValueError('HTTP status')
                    if suffix != '/':
                        data = json.load(response)
                        if data.get('version') != manifest['version'] or data.get('commit') != manifest['commit']:
                            raise ValueError('Public version differs from installed release')
            return
        except (OSError, ValueError, urllib.error.URLError):
            if attempt == 11:
                raise RuntimeError('HTTPS/версия сайта не прошли проверку. Проверьте DNS, цепочку TLS и firewall.') from None
            time.sleep(5)


def install(root, state, manifest):
    """Resume only this installer-owned fresh installation; never upgrade an existing app."""
    if state['complete']:
        raise RuntimeError('MeetSpace уже установлен. Для обновления используйте процедуру выпуска.')
    check_project(state['project'], state['token'])
    bundle = root / 'bundle'
    args = compose_args(root, manifest, state)
    certificates(root, state)
    run(*args, '--profile', 'observability', '--profile', 'migration', 'config', '--quiet')
    print('\n[5/7] Загрузка проверенных образов и подготовка базы данных')
    run('docker', 'image', 'load', '--quiet', '--input', str(bundle / 'release/images.tar'))
    run('bash', str(bundle / 'deployment/scripts/install/verify-release.sh'), str(bundle), 'loaded')
    run(*args, 'up', '-d', '--no-build', '--wait', '--wait-timeout', '240', 'postgres', 'redis', 'rabbitmq', 'minio')
    run(*args, '--profile', 'migration', 'run', '--rm', '--no-deps', 'migrate')
    print('\n[6/7] Запуск MeetSpace')
    run(*args, '--profile', 'observability', 'up', '-d', '--no-build', '--wait', '--wait-timeout', '240')
    print('\n[7/7] Проверка сайта и версии')
    verify_public(state, manifest)
    state['complete'] = True
    pending = root / 'installation.json.next'
    write(pending, json.dumps(state, indent=2) + '\n')
    pending.replace(root / 'installation.json')
    print(f'\nMeetSpace установлен: https://{state["domain"]}\n'
          f'Состояние: sudo {root}/manage.sh status\nЛоги: sudo {root}/manage.sh logs api\n'
          f'Настройки и секреты: {root}/config/meetspace.env (доступ только root).\n'
          'Проверьте звонок с устройства из другой сети.\n'
          'Проверки HTTPS и готовности сервисов не заменяют проверку внешних UDP/TURN-соединений.')
    if state.get('sourceCommit'):
        print(f'Исходники: {root}/bundle/source\nКоммит GitHub: {state["sourceCommit"]}')
    if yes('Создать первый пользовательский аккаунт прямо сейчас?'):
        create_account(state)


def create_account(state):
    """Register an ordinary user over verified HTTPS; never save or print the password."""
    print('Создаётся обычный аккаунт MeetSpace, без дополнительных прав администратора.')
    email = ask('Email аккаунта', validate=lambda s: len(s) <= 254 and bool(re.fullmatch(r'[^\s@]+@[^\s@]+\.[^\s@]+', s)))
    name = ask('Ваше имя', validate=lambda s: 1 <= len(s) <= 100 and all(ord(c) >= 32 for c in s))
    while True:
        password = getpass.getpass('Пароль (8–128 символов, ввод скрыт): ')
        if not 8 <= len(password) <= 128:
            print('Длина пароля должна быть от 8 до 128 символов.')
            continue
        if password == getpass.getpass('Повторите пароль: '):
            break
        print('Пароли не совпадают.')
    origin = 'https://' + state['domain']
    request = urllib.request.Request(origin + '/api/v1/auth/register',
                                     data=json.dumps(dict(email=email, displayName=name, password=password)).encode(),
                                     headers={'Content-Type': 'application/json', 'Origin': origin}, method='POST')
    class NoRedirect(urllib.request.HTTPRedirectHandler):
        """Prevent registration credentials from being forwarded to another origin."""
        def redirect_request(self, request, response, code, message, headers, target):
            return None
    try:
        with urllib.request.build_opener(NoRedirect()).open(request, timeout=30) as response:
            if response.status != 201 or json.load(response).get('status') != 'success':
                raise ValueError('Registration response')
        print('Аккаунт создан. Теперь можно войти в MeetSpace.')
    except (OSError, ValueError, urllib.error.URLError) as error:
        code = getattr(error, 'code', None)
        print('Не удалось создать аккаунт' + (f' (HTTP {code})' if code else '') + '. Установка MeetSpace завершена.\n'
              f'Повторить: sudo {state["root"]}/manage.sh create-account')


def manage(root, command, extra):
    """Expose a small set of explicit operational commands without automatic upgrades."""
    require_local_docker()
    state = json.loads((root / 'installation.json').read_text())
    manifest = metadata(root / 'bundle')
    require_image_store(manifest)
    args = compose_args(root, manifest, state)
    if command == 'status':
        run(*args, '--profile', 'observability', 'ps', '-a')
    elif command == 'logs':
        if extra and (len(extra) != 1 or extra[0] not in manifest['images']):
            raise RuntimeError('Укажите имя одной службы, например api.')
        run(*args, 'logs', '--tail', '100', *extra)
    elif command == 'reload-tls':
        validate_certificate(root / 'certs', state['domain'])
        run(*args, 'exec', '-T', 'proxy', 'nginx', '-s', 'reload')
        # Coturn needs a process restart to pick up its renewed TLS keypair.
        run(*args, 'restart', 'coturn')
    elif command == 'create-account':
        create_account(state)
    elif command == 'certificates':
        certificates(root, state)
        manage(root, 'reload-tls', [])
    else:
        raise RuntimeError('Команды: status, logs [служба], create-account, certificates, reload-tls.')


def main():
    """Hold a host-wide lock and dispatch fresh install, resume or management."""
    os.umask(0o077)
    if os.geteuid() != 0 or sys.platform != 'linux':
        raise RuntimeError('Нужен запуск от root на Linux.')
    # /run is root-owned; O_NOFOLLOW also rejects a pre-existing symlink.
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
        if sys.argv[1] == '--manage':
            manage(Path(sys.argv[2]), sys.argv[3] if len(sys.argv) > 3 else 'status', sys.argv[4:])
            return
        root, state = None, None
        try:
            if sys.argv[1] == '--resume':
                root = Path(sys.argv[2])
                saved = offer_existing_install(root)
                if saved is None:
                    if not root.exists():
                        raise RuntimeError(f'В {root} нет сохранённой установки. Для новой установки запустите мастер без --resume.')
                    return
                state, manifest = saved
                bundle = root / 'bundle'
            else:
                bundle = Path(sys.argv[1]).resolve()
                manifest = metadata(bundle)
            run('bash', str(bundle / 'deployment/scripts/install/verify-release.sh'), str(bundle))
            docker_setup()
            require_image_store(manifest)
            architecture = run('docker', 'info', '--format', '{{.Architecture}}', capture=True)
            platform = {'x86_64': 'linux/amd64', 'amd64': 'linux/amd64', 'aarch64': 'linux/arm64', 'arm64': 'linux/arm64'}.get(architecture)
            if platform != manifest['platform']:
                raise RuntimeError('Архитектура Docker не соответствует пакету установки.')
            if root is None:
                root, state = configure(bundle, manifest)
                selected_bundle = root / 'bundle'
                if digest(selected_bundle / 'bundle.json') != digest(bundle / 'bundle.json'):
                    # An explicit resume must use only its retained bundle, never the new release.
                    manifest = metadata(selected_bundle)
                    run('bash', str(selected_bundle / 'deployment/scripts/install/verify-release.sh'), str(selected_bundle))
                    require_image_store(manifest)
                    if platform != manifest['platform']:
                        raise RuntimeError('Архитектура Docker не соответствует сохранённому пакету установки.')
            install(root, state, manifest)
        except BaseException:
            if root and state is not None and (root / 'installation.json').is_file():
                if state.get('complete'):
                    print(f'\nУстановка уже завершена. Управление: sudo {root}/manage.sh status', file=sys.stderr)
                else:
                    print(f'\nФайлы и данные сохранены в {root}. После устранения причины продолжите:\n'
                          f'sudo python3 -I {root}/bundle/deployment/scripts/install/wizard.py --resume {root}', file=sys.stderr)
            raise


if __name__ == '__main__':
    try:
        main()
    except (KeyboardInterrupt, EOFError):
        print('\nУстановка прервана пользователем.', file=sys.stderr)
        sys.exit(130)
    except (RuntimeError, OSError, ValueError, KeyError, AssertionError, subprocess.CalledProcessError) as error:
        # Subprocess output may contain private diagnostics; never include command arguments.
        print('\nУстановка остановлена: ' + (str(error) if not isinstance(error, subprocess.CalledProcessError)
              else 'служебная команда завершилась с ошибкой; см. сообщение выше.'), file=sys.stderr)
        sys.exit(1)
