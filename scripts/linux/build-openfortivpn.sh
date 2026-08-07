#!/bin/bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
GOARCH=${GOARCH:-amd64}
case "$GOARCH" in
  amd64) DOCKER_PLATFORM=linux/amd64 ;;
  arm64) DOCKER_PLATFORM=linux/arm64 ;;
  *) echo "unsupported GOARCH=$GOARCH" >&2; exit 1 ;;
esac
OUTPUT_ROOT=${1:-"$ROOT_DIR/.build/native-engines/linux-$GOARCH/openfortivpn"}
IMAGE=${IMAGE:-debian:bookworm-slim}

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

mkdir -p "$OUTPUT_ROOT"
rm -rf "${OUTPUT_ROOT:?}/"*
mkdir -p "$OUTPUT_ROOT/bin" "$OUTPUT_ROOT/lib" "$OUTPUT_ROOT/licenses"

echo "Packaging openfortivpn for linux/${GOARCH}..."

docker run --rm --platform "$DOCKER_PLATFORM" \
  -e GOARCH="$GOARCH" \
  -v "$OUTPUT_ROOT:/out" \
  "$IMAGE" \
  bash -ec '
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends openfortivpn ca-certificates patchelf python3
if ! command -v openfortivpn >/dev/null 2>&1; then
  echo "openfortivpn package unavailable in this image" >&2
  exit 1
fi
cp "$(command -v openfortivpn)" /out/bin/openfortivpn
chmod 755 /out/bin/openfortivpn
ldd /out/bin/openfortivpn | awk "/=> \\// {print \$3}" | while read -r lib; do
  base=$(basename "$lib")
  case "$base" in
    libc.so.*|libm.so.*|libdl.so.*|libpthread.so.*|librt.so.*|ld-linux*.so.*) continue ;;
  esac
  cp -L "$lib" "/out/lib/$base" || true
done
if [[ -n "$(ls -A /out/lib 2>/dev/null || true)" ]]; then
  patchelf --set-rpath "\$ORIGIN/../lib" /out/bin/openfortivpn || true
fi
ENGINE_SHA256=$(sha256sum /out/bin/openfortivpn | awk "{print \$1}")
VERSION=$(openfortivpn --version 2>&1 | head -1 | awk "{print \$NF}" || echo distro)
python3 - <<PY
import hashlib, json, os, pathlib
root = pathlib.Path("/out")
files = {f"openfortivpn/lib/{p.name}": hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted((root/"lib").glob("*"))}
manifest = {
  "id": "openfortivpn",
  "protocol": "fortigate-ssl",
  "version": """$VERSION""".strip() or "distro",
  "os": "linux",
  "architecture": os.environ["GOARCH"],
  "executable": "openfortivpn/bin/openfortivpn",
  "sha256": """$ENGINE_SHA256""",
  "license": "GPL-3.0-or-later WITH OpenSSL-exception",
  "capabilities": ["ppp", "otp", "split-route"],
  "files": files,
}
(root / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
print("openfortivpn package ready")
PY
'

echo "openfortivpn engine package OK: $OUTPUT_ROOT"
