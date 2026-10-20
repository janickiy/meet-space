#!/usr/bin/env bash
# Prepare a commit-pinned bootstrapper and a mirror of public GitHub sources.
set -euo pipefail
source "$(dirname "$0")/common.sh"

# usage describes the publication inputs; this tool never deploys the application.
usage() {
  printf '%s\n' \
    'Usage: bash scripts/release/package-installer.sh --commit FULL_SHA --output /absolute/new-directory --base-url https://host/downloads' \
    'The commit must already exist in the public janickiy/meet-space GitHub repository.' \
    'The installer fetches GitHub directly; the downloadable archive is a source mirror, not prebuilt images.'
}

commit= output= base_url= staging=
while (($#)); do
  case "$1" in
    --help|-h) usage; exit 0;;
    --commit|--output|--base-url)
      (($# >= 2)) || fail "Missing value for $1"
      case "$1" in --commit) commit="$2";; --output) output="$2";; --base-url) base_url="$2";; esac
      shift 2;;
    *) fail "Unknown publication argument: $1";;
  esac
done
for dependency in python3 curl git; do command -v "$dependency" >/dev/null || fail "$dependency is required"; done
[[ "$commit" =~ ^[a-f0-9]{40}$ ]] || fail '--commit must be a full Git commit SHA'
[[ "$output" == /* && "$output" != / && "$output" != */ && ! -e "$output" && ! -L "$output" ]] || fail '--output must be a new absolute directory'
[[ -d "$(dirname "$output")" ]] || fail 'The output parent directory must exist'
[[ "$base_url" =~ ^https://[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?(:[0-9]{1,5})?(/[A-Za-z0-9._~-]+)*$ ]] || fail '--base-url must be HTTPS without credentials, query, fragment or trailing slash'
[[ "$base_url" != */../* && "$base_url" != */.. && "$base_url" != */./* && "$base_url" != */. ]] || fail 'URL path traversal is not allowed'
[[ "$(git -C "$RELEASE_ROOT" rev-parse HEAD)" == "$commit" ]] || fail 'Publish from the selected commit checkout'
[[ -z "$(git -C "$RELEASE_ROOT" status --porcelain --untracked-files=normal)" ]] || fail 'Publication requires a clean checkout'

umask 077
staging=$(mktemp -d "$(dirname "$output")/.meetspace-source-publication.XXXXXX")
# Only this invocation's private mktemp directory is removed, never the output.
cleanup() { [[ -z "$staging" || ! -d "$staging" ]] || rm -rf -- "$staging"; }
trap cleanup EXIT
mkdir "$staging/public"
archive="meetspace-source-$commit.tar.gz"
upstream="https://codeload.github.com/janickiy/meet-space/tar.gz/$commit"
curl --fail --show-error --location --proto '=https' --proto-redir '=https' \
  --connect-timeout 20 --max-time 1800 --retry 3 --max-filesize 536870912 \
  --output "$staging/public/$archive" "$upstream"

# Read the bootstrap from the downloaded commit, not an unrelated local worktree.
# Archive contents are inspected but never extracted into the publication directory.
python3 -I - "$staging/public" "$commit" "$base_url" "$upstream" <<'PY'
import hashlib
import json
from pathlib import Path, PurePosixPath
import re
import sys
import tarfile

root, commit, base_url, upstream = sys.argv[1:]
root = Path(root)
archive = root / f'meetspace-source-{commit}.tar.gz'
prefix = f'meet-space-{commit}'
with tarfile.open(archive, 'r:gz') as source:
    files, total = {}, 0
    for member in source:
        path = PurePosixPath(member.name)
        if (path.is_absolute() or '..' in path.parts or not path.parts or path.parts[0] != prefix
                or '\\' in member.name or any(ord(c) < 32 for c in member.name)
                or not (member.isfile() or member.isdir()) or member.name in files):
            raise SystemExit('Unsafe or unexpected GitHub archive member')
        files[member.name] = member
        total += member.size
        if len(files) > 50000 or total > 2 * 1024**3:
            raise SystemExit('GitHub source archive exceeds publication limits')
    for required in ('install.sh', 'scripts/install/from-source.py', 'scripts/install/wizard.py',
                     'scripts/release/build.sh', 'scripts/release/package.sh', 'go.mod'):
        if f'{prefix}/{required}' not in files or not files[f'{prefix}/{required}'].isfile():
            raise SystemExit('The selected GitHub commit lacks required installation files')
    installer = source.extractfile(files[f'{prefix}/install.sh']).read().decode('utf-8')
installer, count = re.subn(r"^DEFAULT_REF='[^'\n]+'$", f"DEFAULT_REF='{commit}'", installer, flags=re.M)
if count != 1 or "REPOSITORY_URL='https://github.com/janickiy/meet-space.git'" not in installer:
    raise SystemExit('Unexpected bootstrapper repository or default-ref declaration')
(root / 'install.sh').write_text(installer)
(root / 'install.sh').chmod(0o755)
(root / 'install.sh.sha256').write_text(hashlib.sha256(installer.encode()).hexdigest() + '  install.sh\n')
hasher = hashlib.sha256()
with archive.open('rb') as stream:
    for block in iter(lambda: stream.read(1024 * 1024), b''):
        hasher.update(block)
metadata = dict(schemaVersion=1, installationMode='github-source',
                repository='https://github.com/janickiy/meet-space.git', commit=commit,
                archive=dict(url=f'{base_url}/{archive.name}', upstreamURL=upstream,
                             sha256=hasher.hexdigest(), sizeBytes=archive.stat().st_size))
(root / 'source.json').write_text(json.dumps(metadata, indent=2) + '\n')
for item in root.iterdir():
    if item.name != 'install.sh':
        item.chmod(0o644)
root.chmod(0o755)
PY
bash -n "$staging/public/install.sh"
[[ ! -e "$output" && ! -L "$output" ]] || fail 'Output appeared during packaging; refusing to replace it'
mv "$staging/public" "$output"
printf 'Source installer prepared: %s\nGitHub commit: %s\nMetadata: %s/source.json\n' "$output" "$commit" "$base_url"
