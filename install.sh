#!/usr/bin/env bash
# Install verified prebuilt Docker images, or explicitly build a GitHub checkout.
set -euo pipefail
umask 077

REPOSITORY_URL='https://github.com/janickiy/meet-space.git'
DEFAULT_REF='main'
DEFAULT_RELEASE='latest'

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

# main selects prebuilt images by default and delegates to the interactive wizard.
# Arguments: --mode images|source, --release latest|TAG, --ref REF, and --build-mode MODE.
main() (
  local ref="$DEFAULT_REF" release="$DEFAULT_RELEASE" mode=images explicit_mode=false
  local source_option=false build_mode=auto scratch= architecture= commit= dependency missing=false
  while (($#)); do
    case "$1" in
      --help|-h)
        printf '%s\n' 'MeetSpace — интерактивная установка на новый Linux-сервер' \
          'По умолчанию: последний опубликованный релиз с готовыми Docker-образами для этого сервера.' \
          'Запуск: sudo bash install.sh [--mode images|source] [--release latest|VERSION]' \
          'Сборка на сервере: sudo bash install.sh --mode source [--ref REF] [--build-mode auto|low-memory|standard]' \
          'Исходники: https://github.com/janickiy/meet-space' \
          'Коммит инструментов закрепляется при публикации; версия образов выбирается при запуске.' \
          'Сборка актуального main: sudo bash install.sh --mode source --ref main' \
          'Режим images не компилирует приложение и не запускает сканер на вашем сервере.' \
          'live-worker по умолчанию не запускается; он нужен только для включённых субтитров или аналитики речи.' \
          'Для сборки из исходников рекомендуются 4 CPU, 8 ГиБ RAM и 35 ГиБ свободного места.' \
          'Мастер запросит домен и сертификат; SMTP необязателен. IPv4 определяется автоматически.' \
          'Каталог /opt/meetspace, проект meetspace и подсеть 172.30.240.0/24 используются по умолчанию.' \
          'Существующая установка не обновляется и не удаляется.'
        return;;
      --mode)
        (($# >= 2)) || fail 'Не задан режим установки.'
        mode="$2"; explicit_mode=true; shift 2;;
      --from-source) mode=source; explicit_mode=true; shift;;
      --release)
        (($# >= 2)) || fail 'Не задана версия готовых образов.'
        release="$2"; shift 2;;
      --ref)
        (($# >= 2)) || fail 'Не задано значение --ref.'
        ref="$2"; source_option=true; shift 2;;
      --build-mode)
        (($# >= 2)) || fail 'Не задано значение --build-mode.'
        build_mode="$2"; source_option=true; shift 2;;
      *) fail "Неизвестный параметр: $1";;
    esac
  done
  # Preserve existing explicit source-build commands; a plain launch uses images.
  if [[ "$source_option" == true && "$explicit_mode" == false ]]; then mode=source; fi
  [[ "$mode" == images || "$mode" == source ]] || fail 'Режим установки: images или source.'
  [[ "$mode" == source || "$source_option" == false ]] || fail '--ref и --build-mode требуют --mode source.'
  [[ "$release" == latest || "$release" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$ ]] \
    || fail 'Укажите latest или версию готовых образов (например v0.4.0-prebuilt.20261023.2).'
  [[ "$build_mode" == auto || "$build_mode" == low-memory || "$build_mode" == standard ]] \
    || fail 'Режим сборки: auto, low-memory или standard.'
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
  printf '\nMeetSpace · режим %s\n\n[1/7] Подготовка сервера\n' "$mode"
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
  printf '\n[2/7] Установщик из GitHub\nРепозиторий: %s\nКоммит/ветка: %s\nПлатформа: %s\n' "$REPOSITORY_URL" "$ref" "$architecture"
  if [[ "$mode" == images ]]; then
    if [[ "$release" == latest ]]; then
      printf 'Будет выбран последний совместимый релиз с готовыми образами из GitHub Releases.\n'
    else
      printf 'Готовые образы: %s.\n' "$release"
    fi
    printf 'Сборка на сервере не выполняется. При отсутствии пакета используйте --mode source --ref main.\n'
    ask 'Скачать готовые образы и запустить мастер установки? [да/нет]'
  else
    ask 'Скачать исходники и запустить сборку на этом сервере? [да/нет]'
  fi
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
  if [[ "$mode" == images ]]; then
    [[ -f "$scratch/source/scripts/install/from-images.py" && ! -L "$scratch/source/scripts/install/from-images.py" ]] \
      || fail 'В выбранной версии нет установщика готовых образов.'
    python3 -I "$scratch/source/scripts/install/from-images.py" --release "$release" --platform "$architecture"
    return
  fi
  if [[ "$build_mode" == auto ]]; then
    # Older explicitly selected source versions do not understand --build-mode.
    python3 -I "$scratch/source/scripts/install/from-source.py" "$scratch/source"
  else
    python3 -I "$scratch/source/scripts/install/from-source.py" "$scratch/source" --build-mode "$build_mode"
  fi
)

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then main "$@"; fi
