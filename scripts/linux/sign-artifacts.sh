#!/bin/bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
if [[ -f "$ROOT_DIR/.env" ]]; then
    set -a
    source "$ROOT_DIR/.env"
    set +a
fi

if [[ -z ${VPNTORIS_LINUX_GPG_KEY_ID:-} ]]; then
    echo "VPNTORIS_LINUX_GPG_KEY_ID is required." >&2
    exit 1
fi

if [[ $# -lt 1 ]]; then
    echo "usage: sign-artifacts.sh artifact..." >&2
    exit 1
fi

for artifact in "$@"; do
    gpg --batch --yes --local-user "$VPNTORIS_LINUX_GPG_KEY_ID" --armor --detach-sign "$artifact"
    gpg --verify "$artifact.asc" "$artifact"
done
