#!/bin/bash
# Build the complete VPNToris Windows MSI (app binaries + native engines).
#
# Product policy: engines are a required product layer (same as macOS
# complete.pkg). There is no app-only / engines-skipped Windows installer.
#
# Prerequisites (maintainer macOS host):
#   brew install msitools   # wixl, wixl-heat, msiextract
#   go, curl, unzip, python3
#
# Usage:
#   VERSION=2.0.0 ./scripts/windows/build-msi.sh
#   VERSION=2.0.0 ./scripts/windows/build-msi.sh --skip-sign
#
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
PACKAGING_DIR="$ROOT_DIR/scripts/windows/packaging"
VERSION=${VERSION:-2.0.0-dev}
PRODUCT_NAME=${PRODUCT_NAME:-VPNToris}
COMPANY_NAME=${COMPANY_NAME:-VPNToris}
OUT_DIR=${OUT_DIR:-"$ROOT_DIR/.build/windows"}
ENGINE_SRC="$ROOT_DIR/.build/native-engines/windows-amd64"
SKIP_SIGN=false

for arg in "$@"; do
  case "$arg" in
    --skip-sign) SKIP_SIGN=true ;;
    --skip-engines|--exes-only|--exe-only|--binaries-only)
      echo "error: Windows MSI always ships native engines (required product layer)" >&2
      echo "       ($arg is not allowed)" >&2
      exit 1
      ;;
    -h|--help)
      sed -n '2,14p' "$0"
      exit 0
      ;;
    *)
      echo "error: unknown flag: $arg" >&2
      exit 1
      ;;
  esac
done

if [[ -f "$ROOT_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +a
fi
PRODUCT_NAME=${PRODUCT_NAME:-VPNToris}
COMPANY_NAME=${COMPANY_NAME:-VPNToris}

if [[ -n ${SAFENET_PIN:-} && ${SIGN_WINDOWS:-} != "false" ]]; then
  SIGN_WINDOWS=true
fi

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "error: missing prerequisite: $1" >&2
    exit 1
  }
}

require_cmd go
require_cmd wixl
require_cmd wixl-heat
require_cmd python3

# WiX Version is Major.Minor.Build.Revision (max 255.255.65535.65535)
msi_version() {
  python3 - "$VERSION" <<'PY'
import re, sys
raw = sys.argv[1].strip()
# 2.0.0 / 2.0.0-rc1 → 2.0.0.0
m = re.match(r"^(\d+)\.(\d+)\.(\d+)", raw)
if not m:
    print("0.0.0.0")
    raise SystemExit(0)
parts = [min(int(m.group(1)), 255), min(int(m.group(2)), 255), min(int(m.group(3)), 65535), 0]
print(".".join(str(p) for p in parts))
PY
}

MSI_VERSION=$(msi_version)
STAGE="$OUT_DIR/msi_stage"
HEAT_DIR="$OUT_DIR/msi_heat"
MSI_NAME="${PRODUCT_NAME}-${VERSION}-windows-amd64.msi"
MSI_PATH="$OUT_DIR/$MSI_NAME"

mkdir -p "$OUT_DIR" "$STAGE" "$HEAT_DIR"
rm -rf "${STAGE:?}/"* "${HEAT_DIR:?}/"*
rm -f "$MSI_PATH"

echo "=== VPNToris Windows complete MSI ==="
echo "version:  $VERSION (MSI $MSI_VERSION)"
echo "product:  $PRODUCT_NAME"
echo "out:      $MSI_PATH"
echo "policy:   engines required (no app-only builds)"
echo

# ─── 1. Cross-compile PE if missing (never a product by itself) ───
echo "[1/5] Windows PE binaries..."
need_pe=false
for bin in vpntorisd.exe vpntoris-native-helper.exe vpntoris-service.exe vpntorisctl.exe vpntoris-tray.exe; do
  if [[ ! -f $OUT_DIR/$bin ]]; then
    need_pe=true
    break
  fi
done
if [[ $need_pe == true ]]; then
  echo "[1/5] compiling PE (intermediate; MSI+engines are the product)..."
  (
    cd "$ROOT_DIR/vpntoris-tray"
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" \
      -o "$OUT_DIR/vpntorisd.exe" .
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" \
      -o "$OUT_DIR/vpntoris-native-helper.exe" ./cmd/vpntoris-native-helper
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" \
      -o "$OUT_DIR/vpntorisctl.exe" ./cli
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" \
      -o "$OUT_DIR/vpntoris-service.exe" ./cmd/vpntoris-service
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w -H=windowsgui" \
      -o "$OUT_DIR/vpntoris-tray.exe" ./cmd/vpntoris-tray
  )
fi
for bin in vpntorisd.exe vpntoris-native-helper.exe vpntoris-service.exe vpntorisctl.exe vpntoris-tray.exe; do
  test -f "$OUT_DIR/$bin" || {
    echo "error: missing $bin in $OUT_DIR" >&2
    exit 1
  }
  cp -f "$OUT_DIR/$bin" "$STAGE/"
done
echo "[1/5] staged PE binaries"

# ─── 2. Native engines (always required) ──────────────────────────
echo "[2/5] Native engines (required product layer)..."
if [[ ! -f $ENGINE_SRC/openvpn/bin/openvpn.exe ]]; then
  echo "[2/5] building engines (openvpn/wintun + openconnect helpers)..."
  "$ROOT_DIR/scripts/windows/build-engines.sh"
fi
if [[ ! -f $ENGINE_SRC/openvpn/bin/openvpn.exe ]]; then
  echo "error: openvpn engine missing after build-engines.sh" >&2
  echo "       Windows product cannot ship without engines" >&2
  exit 1
fi
if [[ ! -f $ENGINE_SRC/openvpn/bin/wintun.dll && ! -f $ENGINE_SRC/openvpn/lib/wintun.dll ]]; then
  echo "error: wintun.dll missing from openvpn engine package" >&2
  exit 1
fi
if [[ ! -f $ENGINE_SRC/openvpn/manifest.json ]]; then
  echo "error: openvpn manifest.json missing" >&2
  exit 1
fi
mkdir -p "$STAGE/engines"
cp -a "$ENGINE_SRC" "$STAGE/engines/windows-amd64"
echo "[2/5] engines from $ENGINE_SRC"
du -sh "$STAGE/engines/windows-amd64"/* 2>/dev/null || true

# ─── 3. Heat engine files + merge WiX ─────────────────────────────
echo "[3/5] Generating WiX fragments..."
WXS_MAIN="$HEAT_DIR/vpntoris.wxs"
WXS_ENGINES="$HEAT_DIR/engines-fragment.wxs"

sed -e "s/PRODUCT_NAME_PLACEHOLDER/${PRODUCT_NAME//\//\\/}/g" \
    -e "s/COMPANY_PLACEHOLDER/${COMPANY_NAME//\//\\/}/g" \
    -e "s/VERSION_PLACEHOLDER/${MSI_VERSION//\//\\/}/g" \
    "$PACKAGING_DIR/vpntoris.wxs" >"$WXS_MAIN"

ENGINE_SOURCE="$STAGE/engines"
if [[ ! -d $ENGINE_SOURCE ]] || [[ -z $(find "$ENGINE_SOURCE" -type f 2>/dev/null | head -1) ]]; then
  echo "error: engine stage is empty — refusing app-only MSI" >&2
  exit 1
fi

find "$ENGINE_SOURCE" -type f | sort | wixl-heat \
  --win64 \
  --component-group EngineFiles \
  --directory-ref ENGINEFOLDER \
  --var var.EngineSource \
  -p "${ENGINE_SOURCE}/" \
  >"$WXS_ENGINES"

file_count=$(rg -c 'File Id=' "$WXS_ENGINES" 2>/dev/null || echo 0)
# Fallback if rg missing
if [[ $file_count == 0 ]]; then
  file_count=$(grep -c 'File Id=' "$WXS_ENGINES" 2>/dev/null || echo 0)
fi
echo "[3/5] heat fragment: $WXS_ENGINES ($file_count files)"
if [[ ${file_count:-0} -lt 1 ]]; then
  echo "error: engine heat produced no File entries — refusing app-only MSI" >&2
  exit 1
fi
# openvpn.exe must be in the harvested set
if ! grep -q 'openvpn.exe' "$WXS_ENGINES"; then
  echo "error: heat fragment missing openvpn.exe" >&2
  exit 1
fi

# ─── 4. Sign PE binaries before cab (MSI embeds them) ─────────────
echo "[4/5] Authenticode (PE)..."
if [[ $SKIP_SIGN == false && ${SIGN_WINDOWS:-false} == true ]]; then
  if [[ -z ${SAFENET_PIN:-} ]]; then
    echo "error: SIGN_WINDOWS=true but SAFENET_PIN empty" >&2
    exit 1
  fi
  "$ROOT_DIR/scripts/windows/sign-osslsigncode.sh" "$STAGE"/*.exe
  cp -f "$STAGE"/*.exe "$OUT_DIR/"
else
  echo "[4/5] PE signing skipped"
fi

# ─── 5. wixl package ──────────────────────────────────────────────
echo "[5/5] wixl packaging..."
cp -f "$STAGE"/*.exe "$HEAT_DIR/"
cd "$HEAT_DIR"

wixl -v -a x64 \
  -D "EngineSource=$ENGINE_SOURCE" \
  -D "Win64=yes" \
  -o "$MSI_PATH" \
  "$WXS_MAIN" "$WXS_ENGINES"

test -f "$MSI_PATH" || {
  echo "error: MSI not produced: $MSI_PATH" >&2
  exit 1
}

if [[ $SKIP_SIGN == false && ${SIGN_WINDOWS:-false} == true ]]; then
  echo "[5/5] Authenticode (MSI)..."
  "$ROOT_DIR/scripts/windows/sign-osslsigncode.sh" --desc "$PRODUCT_NAME Installer" "$MSI_PATH"
else
  echo "[5/5] MSI signing skipped"
fi

cp -f "$MSI_PATH" "$OUT_DIR/${PRODUCT_NAME}-windows-amd64.msi"

echo
echo "=== Complete MSI ready (engines included) ==="
ls -lh "$MSI_PATH" "$OUT_DIR/${PRODUCT_NAME}-windows-amd64.msi"
shasum -a 256 "$MSI_PATH" | tee "$MSI_PATH.sha256"
echo
echo "Install layout:"
echo "  Program Files\\$PRODUCT_NAME\\{vpntorisd,helper,service,ctl,tray}.exe"
echo "  ProgramData\\VPNToris\\Engines\\windows-amd64\\..."
echo "  Service: VPNTorisNativeHelper"
