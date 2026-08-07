#!/bin/bash
set -euo pipefail

# Packages a Linux OpenVPN engine under:
#   .build/native-engines/linux-${GOARCH}/openvpn/{bin,lib,manifest.json,licenses}
#
# Build happens inside Docker so macOS hosts can produce linux-amd64/arm64
# payloads without a local cross toolchain. Engines are never resolved from PATH
# at runtime; only the packaged binary + libs are used.

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
# Align with the macOS packaged series where practical.
OPENVPN_VERSION=${OPENVPN_VERSION:-2.7.5}
OPENVPN_SHA256=${OPENVPN_SHA256:-c6864b3c7d4e059c7d6ce22d1b5fa646c8b379a06af872eeb9792b6083a44ac4}
GOARCH=${GOARCH:-amd64}
case "$GOARCH" in
  amd64) DOCKER_PLATFORM=linux/amd64 ;;
  arm64) DOCKER_PLATFORM=linux/arm64 ;;
  *) echo "unsupported GOARCH=$GOARCH" >&2; exit 1 ;;
esac

OUTPUT_ROOT=${1:-"$ROOT_DIR/.build/native-engines/linux-$GOARCH/openvpn"}
IMAGE=${IMAGE:-debian:bookworm-slim}

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required to build the Linux OpenVPN engine" >&2
  exit 1
fi

mkdir -p "$OUTPUT_ROOT"
rm -rf "${OUTPUT_ROOT:?}/"*
mkdir -p "$OUTPUT_ROOT"

echo "Building OpenVPN $OPENVPN_VERSION for linux/$GOARCH via $IMAGE …"

docker run --rm --platform "$DOCKER_PLATFORM" \
  -e OPENVPN_VERSION="$OPENVPN_VERSION" \
  -e OPENVPN_SHA256="$OPENVPN_SHA256" \
  -e GOARCH="$GOARCH" \
  -v "$OUTPUT_ROOT:/out" \
  -w /tmp \
  "$IMAGE" \
  bash -ec '
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends \
  build-essential curl ca-certificates pkg-config python3 patchelf \
  liblzo2-dev liblz4-dev libpam0g-dev libpkcs11-helper1-dev \
  libssl-dev libnl-3-dev libnl-genl-3-dev libcap-ng-dev
curl -L --fail -o openvpn.tar.gz "https://swupdate.openvpn.net/community/releases/openvpn-${OPENVPN_VERSION}.tar.gz"
echo "${OPENVPN_SHA256}  openvpn.tar.gz" | sha256sum -c -
mkdir -p src
tar -xzf openvpn.tar.gz -C src --strip-components=1
cd src
./configure --prefix=/opt/openvpn --disable-debug --disable-unit-tests --enable-static=no --disable-dco || \
  ./configure --prefix=/opt/openvpn --disable-debug --disable-unit-tests --enable-static=no
make -j"$(nproc)"
make install
mkdir -p /out/bin /out/lib /out/licenses /out/sources
cp /opt/openvpn/sbin/openvpn /out/bin/openvpn
chmod 755 /out/bin/openvpn
# Copy direct shared libraries used by the engine so runtime never needs PATH/ldconfig.
ldd /out/bin/openvpn | awk "/=> \\// {print \$3}" | while read -r lib; do
  base=$(basename "$lib")
  case "$base" in
    libc.so.*|libm.so.*|libdl.so.*|libpthread.so.*|librt.so.*|ld-linux*.so.*)
      continue
      ;;
  esac
  cp -L "$lib" "/out/lib/$base"
done
# Relocate loader search to the bundled lib directory (no host PATH/ldconfig).
if [[ -n "$(ls -A /out/lib 2>/dev/null || true)" ]]; then
  patchelf --set-rpath "\$ORIGIN/../lib" /out/bin/openvpn
  for lib in /out/lib/*; do
    patchelf --set-rpath "\$ORIGIN" "$lib" || true
  done
fi
cp COPYING /out/licenses/openvpn.txt || true
cp /tmp/openvpn.tar.gz /out/sources/openvpn-${OPENVPN_VERSION}.tar.gz
ENGINE_SHA256=$(sha256sum /out/bin/openvpn | awk "{print \$1}")
python3 - <<PY
import hashlib, json, os, pathlib
root = pathlib.Path("/out")
files = {}
lib_dir = root / "lib"
if lib_dir.exists():
    for path in sorted(lib_dir.glob("*")):
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        files[f"openvpn/lib/{path.name}"] = digest
manifest = {
  "id": "openvpn",
  "protocol": "openvpn",
  "version": os.environ["OPENVPN_VERSION"],
  "os": "linux",
  "architecture": os.environ["GOARCH"],
  "executable": "openvpn/bin/openvpn",
  "sha256": """$ENGINE_SHA256""",
  "license": "GPL-2.0-only WITH OpenSSL-exception",
  "capabilities": ["tun", "userpass", "challenge", "split-route"],
  "files": files,
}
(root / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
print("manifest written")
PY
'

# Fix loader path: write a tiny wrapper is avoided; use patchelf when available in image next iteration.
# Validate layout on host.
test -x "$OUTPUT_ROOT/bin/openvpn"
test -f "$OUTPUT_ROOT/manifest.json"
python3 - <<PY
import json, pathlib, sys
root = pathlib.Path("$OUTPUT_ROOT")
manifest = json.loads((root / "manifest.json").read_text())
assert manifest["os"] == "linux"
assert manifest["architecture"] == "$GOARCH"
assert manifest["executable"] == "openvpn/bin/openvpn"
assert len(manifest["sha256"]) == 64
print("OpenVPN engine package OK:", root)
print("  version:", manifest["version"])
print("  sha256:", manifest["sha256"])
PY
