#!/bin/bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
GOARCH=${GOARCH:-amd64}
case "$GOARCH" in
  amd64) DOCKER_PLATFORM=linux/amd64 ;;
  arm64) DOCKER_PLATFORM=linux/arm64 ;;
  *) echo "unsupported GOARCH=$GOARCH" >&2; exit 1 ;;
esac
OUTPUT_ROOT=${1:-"$ROOT_DIR/.build/native-engines/linux-$GOARCH/openconnect"}
IMAGE=${IMAGE:-debian:bookworm-slim}

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

mkdir -p "$OUTPUT_ROOT"
rm -rf "${OUTPUT_ROOT:?}/"*
mkdir -p "$OUTPUT_ROOT/bin" "$OUTPUT_ROOT/lib" "$OUTPUT_ROOT/licenses"

echo "Building OpenConnect helpers and packaging engine for linux/${GOARCH}..."

mkdir -p "$ROOT_DIR/.build/tmp-linux-$GOARCH"
docker run --rm --platform "$DOCKER_PLATFORM" \
  -v "$ROOT_DIR:/src" -w /src/vpntoris-tray \
  -e CGO_ENABLED=0 \
  golang:1.26-bookworm \
  bash -ec "
    go build -o /src/.build/tmp-linux-$GOARCH/vpntoris-vpnc-script ./cmd/vpntoris-vpnc-script
    go build -o /src/.build/tmp-linux-$GOARCH/vpntoris-browser-open ./cmd/vpntoris-browser-open
  "

docker run --rm --platform "$DOCKER_PLATFORM" \
  -e GOARCH="$GOARCH" \
  -v "$OUTPUT_ROOT:/out" \
  -v "$ROOT_DIR/.build/tmp-linux-$GOARCH:/helpers:ro" \
  "$IMAGE" \
  bash -ec '
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends openconnect ca-certificates patchelf python3
cp "$(command -v openconnect)" /out/bin/openconnect
chmod 755 /out/bin/openconnect
cp /helpers/vpntoris-vpnc-script /helpers/vpntoris-browser-open /out/bin/
chmod 755 /out/bin/vpntoris-vpnc-script /out/bin/vpntoris-browser-open
ldd /out/bin/openconnect | awk "/=> \\// {print \$3}" | while read -r lib; do
  base=$(basename "$lib")
  case "$base" in
    libc.so.*|libm.so.*|libdl.so.*|libpthread.so.*|librt.so.*|ld-linux*.so.*) continue ;;
  esac
  cp -L "$lib" "/out/lib/$base" || true
done
if [[ -n "$(ls -A /out/lib 2>/dev/null || true)" ]]; then
  patchelf --set-rpath "\$ORIGIN/../lib" /out/bin/openconnect || true
fi
VERSION=$(openconnect --version 2>&1 | head -1 | awk "{print \$NF}" || echo unknown)
ENGINE_SHA256=$(sha256sum /out/bin/openconnect | awk "{print \$1}")
python3 - <<PY
import hashlib, json, os, pathlib
root = pathlib.Path("/out")
files = {}
for path in sorted((root / "lib").glob("*")):
    files[f"openconnect/lib/{path.name}"] = hashlib.sha256(path.read_bytes()).hexdigest()
manifest = {
  "id": "openconnect",
  "protocol": "openconnect",
  "version": os.environ.get("VERSION", "distro"),
  "os": "linux",
  "architecture": os.environ["GOARCH"],
  "executable": "openconnect/bin/openconnect",
  "sha256": """$ENGINE_SHA256""",
  "license": "LGPL-2.1-or-later",
  "capabilities": ["anyconnect","gp","pulse","nc","f5","fortinet","array","otp","split-route"],
  "files": files,
}
version = """$VERSION""".strip()
if version and version != "unknown":
    manifest["version"] = version
(root / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
print("openconnect package ready")
PY
'

echo "OpenConnect engine package OK: $OUTPUT_ROOT"
