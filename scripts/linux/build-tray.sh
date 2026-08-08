#!/bin/bash
set -euo pipefail
ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
OUT_DIR=${1:-"$ROOT_DIR/.build/linux"}
GOARCH=${GOARCH:-amd64}
mkdir -p "$OUT_DIR"
cd "$ROOT_DIR/vpntoris-tray"
CGO_ENABLED=${CGO_ENABLED:-0} GOOS=linux GOARCH="$GOARCH" go build -trimpath -ldflags="-s -w" \
  -o "$OUT_DIR/vpntoris-tray" ./cmd/vpntoris-tray
echo "Linux tray: $OUT_DIR/vpntoris-tray"
