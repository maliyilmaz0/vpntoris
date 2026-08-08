#!/bin/bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
OUT_DIR=${OUT_DIR:-"$ROOT_DIR/.build/windows"}
SKIP_SIGN=false
for arg in "$@"; do
  case "$arg" in
    --skip-sign) SKIP_SIGN=true ;;
    --skip-engines|--exes-only|--exe-only|--binaries-only)
      echo "error: Windows product builds always include native engines + complete MSI" >&2
      echo "       ($arg is not allowed — engines are a required product layer)" >&2
      exit 1
      ;;
    -h|--help)
      sed -n '2,12p' "$0"
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
  source "$ROOT_DIR/.env"
  set +a
fi

if [[ -n ${SAFENET_PIN:-} && ${SIGN_WINDOWS:-} != "false" ]]; then
  SIGN_WINDOWS=true
fi

mkdir -p "$OUT_DIR"
echo "[Windows] Cross-compiling PE to $OUT_DIR ..."
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

echo "[Windows] PE artifacts:"
ls -la "$OUT_DIR"/*.exe

msi_args=()
if [[ $SKIP_SIGN == true || ${SIGN_WINDOWS:-false} != "true" ]]; then
  msi_args+=(--skip-sign)
fi

VERSION=${VERSION:-2.0.0-dev}
export VERSION OUT_DIR SIGN_WINDOWS
echo "[Windows] Building complete MSI (required engines + PE)..."
"$ROOT_DIR/scripts/windows/build-msi.sh" "${msi_args[@]+"${msi_args[@]}"}"

echo "[Windows] Done (complete product)."
ls -la "$OUT_DIR"/*.{exe,msi} 2>/dev/null || ls -la "$OUT_DIR"
