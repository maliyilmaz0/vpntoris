#!/bin/bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
GOARCH=${GOARCH:-amd64}
IMAGE=${IMAGE:-golang:1.26-bookworm}

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

echo "Running multi-protocol Linux native smoke…"

docker run --rm --platform "linux/$GOARCH" \
  --cap-add=NET_ADMIN \
  --device /dev/net/tun \
  -v "$ROOT_DIR:/work" \
  -w /work/vpntoris-tray \
  -e CGO_ENABLED=0 \
  "$IMAGE" \
  bash -ec '
set -euo pipefail
apt-get update >/dev/null
apt-get install -y --no-install-recommends iproute2 ca-certificates python3 >/dev/null
go test ./internal/nativehelper ./internal/netbackend ./cmd/vpntoris-vpnc-script ./cmd/vpntoris-browser-open -count=1
echo "PROTOCOL SMOKE UNIT OK"
'

echo "Multi-protocol unit smoke passed (OpenVPN/OpenConnect/Forti e2e + DNS/IPsec config tests)."
