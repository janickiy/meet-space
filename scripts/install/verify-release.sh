#!/usr/bin/env bash
# Verify an installer release using the existing release integrity/security gates.
# Arguments: absolute bundle root, optional "loaded" to verify Docker image provenance.
set -euo pipefail
bundle="${1:?bundle directory required}"
source "$bundle/deployment/scripts/release/common.sh"
RELEASE_MANIFEST="$bundle/release/release.json"
jq -e '.schemaVersion == 1 and .artifactMode == "archive" and .sourceDirty == false
  and .sourceFingerprintVerified == true and .securityScanned == true
  and (.version | test("^v[0-9]+\\.[0-9]+\\.[0-9]+(-[A-Za-z0-9.-]+)?$"))
  and (.commit | test("^[a-f0-9]{40}$"))
  and (.sourceSHA256 | test("^[a-f0-9]{64}$"))' "$RELEASE_MANIFEST" >/dev/null || fail 'Invalid installer release'
RELEASE_VERSION=$(jq -r .version "$RELEASE_MANIFEST")
RELEASE_COMMIT=$(jq -r .commit "$RELEASE_MANIFEST")
RELEASE_LAYOUT=$(jq -r '.deploymentLayout // "classic"' "$RELEASE_MANIFEST")
[[ "$RELEASE_LAYOUT" == classic || "$RELEASE_LAYOUT" == hardened-shared-host ]] || fail 'Unsupported installer layout'
select_release_services "$RELEASE_LAYOUT"
if [[ "${2:-}" == loaded ]]; then verify_loaded_images; else verify_archive_security; fi
