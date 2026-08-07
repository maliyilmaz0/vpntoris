#!/bin/bash
set -euo pipefail

# Packages strongSwan charon/swanctl for Linux with kernel-netlink plugins.
ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
GOARCH=${GOARCH:-amd64}
case "$GOARCH" in
  amd64) DOCKER_PLATFORM=linux/amd64 ;;
  arm64) DOCKER_PLATFORM=linux/arm64 ;;
  *) echo "unsupported GOARCH=$GOARCH" >&2; exit 1 ;;
esac
OUTPUT_ROOT=${1:-"$ROOT_DIR/.build/native-engines/linux-$GOARCH/strongswan"}
IMAGE=${IMAGE:-debian:bookworm-slim}

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

mkdir -p "$OUTPUT_ROOT"
rm -rf "${OUTPUT_ROOT:?}/"*
mkdir -p "$OUTPUT_ROOT/bin" "$OUTPUT_ROOT/lib" "$OUTPUT_ROOT/plugins" "$OUTPUT_ROOT/licenses"

echo "Packaging strongSwan for linux/${GOARCH}..."

docker run --rm --platform "$DOCKER_PLATFORM" \
  -e GOARCH="$GOARCH" \
  -v "$OUTPUT_ROOT:/out" \
  "$IMAGE" \
  bash -ec '
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends strongswan strongswan-charon strongswan-swanctl libstrongswan libstrongswan-standard-plugins libstrongswan-extra-plugins ca-certificates python3
# Locate binaries across Debian layout variants.
CHARON=$(command -v charon || true)
SWANCTL=$(command -v swanctl || true)
if [[ -z "$CHARON" ]]; then
  for candidate in /usr/lib/ipsec/charon /usr/libexec/ipsec/charon /usr/sbin/charon; do
    [[ -x "$candidate" ]] && CHARON=$candidate && break
  done
fi
if [[ -z "$SWANCTL" ]]; then
  for candidate in /usr/sbin/swanctl /usr/bin/swanctl; do
    [[ -x "$candidate" ]] && SWANCTL=$candidate && break
  done
fi
test -n "$CHARON" && test -n "$SWANCTL"
cp "$CHARON" /out/bin/charon
cp "$SWANCTL" /out/bin/swanctl
chmod 755 /out/bin/charon /out/bin/swanctl
# Collect plugins commonly required for IKE/XAuth/EAP.
for dir in /usr/lib/ipsec/plugins /usr/lib/strongswan/plugins /usr/lib/*/strongswan/plugins; do
  if [[ -d "$dir" ]]; then
    cp -a "$dir"/. /out/plugins/ || true
  fi
done
ENGINE_SHA256=$(sha256sum /out/bin/charon | awk "{print \$1}")
python3 - <<PY
import hashlib, json, os, pathlib
root = pathlib.Path("/out")
files = {}
for path in sorted((root / "plugins").rglob("*")):
    if path.is_file():
        rel = path.relative_to(root)
        files[f"strongswan/{rel.as_posix()}"] = hashlib.sha256(path.read_bytes()).hexdigest()
manifest = {
  "id": "strongswan",
  "protocol": "ipsec",
  "version": "distro",
  "os": "linux",
  "architecture": os.environ["GOARCH"],
  "executable": "strongswan/bin/charon",
  "sha256": """$ENGINE_SHA256""",
  "license": "GPL-2.0-or-later",
  "capabilities": ["ikev1","ikev2","xauth","xauth-otp","eap","sha1","dh20","split-route"],
  "files": files,
}
(root / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
print("strongswan package ready")
PY
'

echo "strongSwan engine package OK: $OUTPUT_ROOT"
