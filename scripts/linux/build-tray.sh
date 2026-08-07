#!/bin/bash
# Build the Linux system tray client.
# Note: runtime needs a StatusNotifier/AppIndicator host (GNOME/KDE/etc).
# Secret prompts use zenity or kdialog when available.
set -euo pipefail
ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
OUT_DIR=${1:-"$ROOT_DIR/.build/linux"}
GOARCH=${GOARCH:-amd64}
mkdir -p "$OUT_DIR"
cd "$ROOT_DIR/vpntoris-tray"
# CGO may be required on some distros for full AppIndicator support; pure Go
# build is attempted first for CI/cross-compile friendliness.
CGO_ENABLED=${CGO_ENABLED:-0} GOOS=linux GOARCH="$GOARCH" go build -trimpath -ldflags="-s -w" \
  -o "$OUT_DIR/vpntoris-tray" ./cmd/vpntoris-tray
echo "Linux tray: $OUT_DIR/vpntoris-tray"
