#!/bin/bash
set -euo pipefail

if [[ -z ${GPG_SIGNING_KEY_ID:-} ]]; then
    echo "GPG_SIGNING_KEY_ID is required." >&2
    exit 1
fi

if [[ $# -lt 1 ]]; then
    echo "usage: sign-artifacts.sh artifact..." >&2
    exit 1
fi

for artifact in "$@"; do
    gpg --batch --yes --local-user "$GPG_SIGNING_KEY_ID" --armor --detach-sign "$artifact"
    gpg --verify "$artifact.asc" "$artifact"
done
