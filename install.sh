#!/usr/bin/env bash
# Fetch an exact GitHub checkout and start the interactive source-build installer.
set -euo pipefail
umask 077

REPOSITORY_URL='https://github.com/janickiy/meet-space.git'
DEFAULT_REF='main'

# fail prints an actionable error without exposing credentials or configuration.
fail() { printf '\nОшибка: %s\n' "$*" >&2; exit 1; }

# ask reads the supplied prompt from the controlling terminal, not a download pipe.
ask() { printf '%s ' "$1" >/dev/tty; IFS= read -r REPLY </dev/tty || fail 'Ввод прерван.'; }

# source_git isolates Git from user configuration, hooks, credentials and URL rewrites.
# Arguments are passed directly to Git; the repository URL is fixed above.
source_git() {
  env -i PATH="$PATH" HOME=/nonexistent GIT_CONFIG_NOSYSTEM=1 \
    GIT_CONFIG_GLOBAL=/dev/null GIT_TERMINAL_PROMPT=0 \
    git -c core.hooksPath=/dev/null -c credential.helper= \
    -c protocol.allow=never -c protocol.https.allow=always "$@"
}

# main validates the requested ref, fetches public sources and delegates server setup.
# Arguments: optional --ref branch/tag/full commit; --help has no side effects.
main() (
  local ref="$DEFAULT_REF" scratch= architecture= commit= dependency missing=false
  while (($#)); do
    case "$1" in
      --help|-h)
        printf '%s\n' 'MeetSpace — интерактивная установка из GitHub на новый Linux-сервер' \
          'Запуск: sudo bash install.sh [--ref BRANCH_OR_TAG_OR_COMMIT]' \
          'Исходники: https://github.com/janickiy/meet-space' \
          'Опубликованный установщик закреплён за проверенным коммитом; --ref меняет его.' \
          'Docker-образы собираются на вашем сервере. Go и Node на хосте не нужны.' \
          'Рекомендуется 4 CPU, 8 ГиБ RAM и 35 ГиБ свободного места; нехватка места — предупреждение, не запрет.' \
          'Мастер запросит домен, IP, сертификат и новый каталог установки.' \
          'Существующая установка не обновляется и не удаляется.'
        return;;
      --ref)
        (($# >= 2)) || fail 'Не задано значение --ref.'
        ref="$2"; shift 2;;
      *) fail "Неизвестный параметр: $1";;
    esac
  done
  [[ "$ref" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]*$ && "$ref" != *..* && "$ref" != */ && "$ref" != *//* ]] \
    || fail 'Укажите имя ветки, тега или полный SHA коммита без специальных символов.'
  [[ $(uname -s) == Linux ]] || fail 'Установщик предназначен для Linux-сервера.'
  [[ $(id -u) == 0 ]] || fail 'Запустите: sudo bash install.sh'
  [[ -r /dev/tty && -w /dev/tty ]] || fail 'Нужен интерактивный терминал (SSH с терминалом).'
  case $(uname -m) in
    x86_64) architecture=linux/amd64;;
    aarch64|arm64) architecture=linux/arm64;;
    *) fail 'Поддерживаются amd64 и arm64.';;
  esac
  printf '\nMeetSpace · установка из исходников\n\n[1/7] Подготовка сервера\n'
  for dependency in python3 curl git jq openssl tar gzip rg; do
    command -v "$dependency" >/dev/null || missing=true
  done
  if [[ "$missing" == true ]]; then
    command -v apt-get >/dev/null || fail 'Установите python3, curl, git, jq, openssl, tar, gzip, ripgrep и ca-certificates.'
    ask 'Установить необходимые системные пакеты через apt? [да/нет]'
    [[ "$REPLY" == да || "$REPLY" == yes || "$REPLY" == y ]] || fail 'Установка отменена.'
    apt-get update
    apt-get install -y python3 curl git jq openssl tar gzip ripgrep ca-certificates
  fi
  source_git check-ref-format "refs/heads/$ref" >/dev/null || fail 'Некорректное имя ветки или тега.'
  printf '\n[2/7] Исходники из GitHub\nРепозиторий: %s\nВерсия: %s\nПлатформа: %s\n' "$REPOSITORY_URL" "$ref" "$architecture"
  ask 'Скачать эти исходники и запустить интерактивную сборку? [да/нет]'
  [[ "$REPLY" == да || "$REPLY" == yes || "$REPLY" == y ]] || fail 'Установка отменена.'
  scratch=$(mktemp -d /var/tmp/meetspace-install.XXXXXXXX)
  # The build helper makes an independent checkout and retains it on failure.
  # Only this invocation's temporary download is removed by the bootstrapper.
  trap 'case "${scratch:-}" in /var/tmp/meetspace-install.*) rm -rf -- "$scratch";; esac' EXIT
  source_git init --quiet "$scratch/source"
  source_git -C "$scratch/source" remote add origin "$REPOSITORY_URL"
  source_git -C "$scratch/source" fetch --depth 1 --no-tags origin "$ref" \
    || fail 'Не удалось получить версию из GitHub. Проверьте доступ к репозиторию и --ref.'
  source_git -C "$scratch/source" -c advice.detachedHead=false checkout --quiet --detach FETCH_HEAD
  commit=$(source_git -C "$scratch/source" rev-parse HEAD)
  [[ "$commit" =~ ^[a-f0-9]{40}$ ]] || fail 'GitHub вернул некорректный идентификатор коммита.'
  if [[ "$ref" =~ ^[a-f0-9]{40}$ && "$commit" != "$ref" ]]; then
    fail 'Загруженный коммит не совпадает с запрошенным.'
  fi
  [[ -f "$scratch/source/scripts/install/from-source.py" && ! -L "$scratch/source/scripts/install/from-source.py" ]] \
    || fail 'В выбранной версии нет установщика из исходников. Выберите более новый коммит.'
  printf '\nПолучен коммит: %s\n' "$commit"
  python3 -I "$scratch/source/scripts/install/from-source.py" "$scratch/source"
)

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then main "$@"; fi
