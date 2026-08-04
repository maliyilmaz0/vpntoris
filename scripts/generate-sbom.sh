#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${1:-$ROOT_DIR/dist}"
OUT_FILE="$OUT_DIR/VPNToris-sbom.txt"
mkdir -p "$OUT_DIR"

{
  echo "VPNToris Software Bill of Materials"
  echo "Generated: $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  echo
  echo "Application"
  echo "- VPNToris: ${VERSION:-1.2.0}"
  echo "- Target: macOS universal (arm64, x86_64)"
  echo
  echo "Build toolchain"
  echo "- Go: $(go version 2>/dev/null || echo 'not available')"
  echo "- Swift: $(swift --version 2>/dev/null | head -1 || echo 'not available')"
  echo "- Xcode: $(xcodebuild -version 2>/dev/null | tr '\\n' '; ' || echo 'not available')"
  echo
  echo "Go modules"
  (cd "$ROOT_DIR/vpntoris-tray" && go list -m all 2>/dev/null) || echo "Go module inventory unavailable"
  echo
  echo "Bundled engines"
  find "$ROOT_DIR/build" "$ROOT_DIR/dist" -type f \( -name 'engine-manifest.json' -o -name '*manifest*.json' \) -print 2>/dev/null | sort || true
  echo
  echo "Source repositories"
  echo "- tun2socks: github.com/xjasonlyu/tun2socks/v2 v2.7.0"
  echo "- strongSwan: 5.9.13 with VPNToris xauth-generic OTP patch"
  echo "- openvpn, openconnect, openfortivpn: bundled platform builds; see docs/NATIVE_ENGINE.md"
} > "$OUT_FILE"

echo "SBOM written to $OUT_FILE"
