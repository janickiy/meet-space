#!/usr/bin/env python3
"""Создаёт локальный LDAP-стенд и начальные учётные записи без вывода паролей."""

import json
import os
from pathlib import Path
import secrets
import subprocess
import urllib.request


ROOT = Path(__file__).resolve().parent
PRIVATE = ROOT / "secrets"
TLS = PRIVATE / "tls"
URL = "http://127.0.0.1:17170"
COMPOSE = ["docker", "compose", "--project-directory", str(ROOT), "-f", str(ROOT / "compose.yml")]
USERS = [
    ("svc-meetspace", "Служебная учётная запись MeetSpace", ["lldap_strict_readonly"]),
    ("alice", "Анна Тестовая", ["meetspace-users"]),
    ("bob", "Борис Тестовый", ["meetspace-users"]),
    ("outsider", "Пользователь без доступа к MeetSpace", []),
]


def private_write(path, text):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as handle:
        handle.write(text)
    path.chmod(0o600)


def run(command, **kwargs):
    return subprocess.run(command, check=True, **kwargs)


def request(path, body, token=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(URL + path, json.dumps(body).encode(), headers)
    with urllib.request.urlopen(req, timeout=15) as response:
        return json.load(response)


def main():
    PRIVATE.mkdir(mode=0o700, exist_ok=True)
    PRIVATE.chmod(0o700)
    TLS.mkdir(mode=0o700, exist_ok=True)
    credentials_file = PRIVATE / "credentials.json"
    if credentials_file.exists():
        credentials = json.loads(credentials_file.read_text())
    else:
        credentials = {
            name: secrets.token_urlsafe(24)
            for name in ["admin", "jwt_secret", *(user[0] for user in USERS)]
        }
        private_write(credentials_file, json.dumps(credentials, indent=2) + "\n")

    private_write(ROOT / ".env", "\n".join([
        f"LDAP_RUN_UID={os.getuid()}",
        f"LDAP_RUN_GID={os.getgid()}",
        f"LLDAP_ADMIN_PASSWORD={credentials['admin']}",
        f"LLDAP_JWT_SECRET={credentials['jwt_secret']}",
        "",
    ]))

    if not (TLS / "server.crt").exists():
        private_write(PRIVATE / "server.cnf", """[req]
prompt = no
distinguished_name = dn
[dn]
CN = localhost
[server_ext]
basicConstraints = critical,CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = DNS:localhost,DNS:ldap,DNS:host.docker.internal,IP:127.0.0.1
""")
        private_write(PRIVATE / "ca.cnf", """[req]
prompt = no
distinguished_name = dn
x509_extensions = ca_ext
[dn]
CN = MeetSpace Local LDAP Test CA
[ca_ext]
basicConstraints = critical,CA:TRUE
keyUsage = critical,keyCertSign,cRLSign
subjectKeyIdentifier = hash
""")
        commands = [
            ["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-sha256", "-days", "3650", "-config", str(PRIVATE / "ca.cnf"), "-keyout", str(PRIVATE / "ca.key"), "-out", str(TLS / "ca.crt")],
            ["openssl", "req", "-new", "-newkey", "rsa:2048", "-nodes", "-sha256", "-config", str(PRIVATE / "server.cnf"), "-keyout", str(TLS / "server.key"), "-out", str(PRIVATE / "server.csr")],
            ["openssl", "x509", "-req", "-in", str(PRIVATE / "server.csr"), "-CA", str(TLS / "ca.crt"), "-CAkey", str(PRIVATE / "ca.key"), "-CAcreateserial", "-CAserial", str(PRIVATE / "ca.srl"), "-days", "365", "-sha256", "-extfile", str(PRIVATE / "server.cnf"), "-extensions", "server_ext", "-out", str(TLS / "server.crt")],
        ]
        for command in commands:
            run(command, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
        (PRIVATE / "ca.key").chmod(0o600)
        (TLS / "server.key").chmod(0o600)

    run(COMPOSE + ["config", "--quiet"])
    run(COMPOSE + ["up", "-d", "--wait", "--wait-timeout", "60"])
    token = request("/auth/simple/login", {
        "username": "admin", "password": credentials["admin"],
    })["token"]

    def query(document, variables=None):
        result = request("/api/graphql", {"query": document, "variables": variables or {}}, token)
        if result.get("errors"):
            raise RuntimeError("Ошибка LDAP API: " + json.dumps(result["errors"], ensure_ascii=False))
        return result["data"]

    groups = {group["displayName"]: group["id"] for group in query("{groups{id displayName}}")['groups']}
    if "meetspace-users" not in groups:
        group = query('mutation {createGroup(name:"meetspace-users"){id}}')["createGroup"]
        groups["meetspace-users"] = group["id"]
    existing = {user["id"] for user in query("{users{id}}")['users']}
    for username, display_name, memberships in USERS:
        if username in existing:
            print(f"Сохранена существующая учётная запись: {username}")
            continue
        query("mutation($user:CreateUserInput!){createUser(user:$user){id}}", {
            "user": {"id": username, "email": username + "@meetspace.test", "displayName": display_name},
        })
        # Пароли передаются через stdin, не включаются в команду хоста или журнал.
        password_command = 'IFS= read -r admin_password; IFS= read -r LLDAP_USER_PASSWORD; export LLDAP_USER_PASSWORD; exec /app/lldap_set_password --base-url http://127.0.0.1:17170 --admin-password "$admin_password" --username "$1"'
        run(COMPOSE + ["exec", "-T", "ldap", "sh", "-c", password_command, "sh", username],
            input=credentials["admin"] + "\n" + credentials[username] + "\n", text=True)
        for group_name in memberships:
            query("mutation($user:String!,$group:Int!){addUserToGroup(userId:$user,groupId:$group){ok}}", {
                "user": username, "group": groups[group_name],
            })
        print(f"Создана учётная запись: {username}")

    print("LDAP-стенд готов. Веб-панель: " + URL)
    print("Пароли сохранены только локально: " + str(credentials_file))


if __name__ == "__main__":
    # Ограничивает доступ к создаваемым ключам и другим временным файлам.
    os.umask(0o077)
    main()
