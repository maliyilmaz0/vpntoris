#!/bin/bash
# Build architecture-specific Linux packages (DEB + RPM) using Docker.
#
# Why Docker:
#   - Host is often macOS Apple Silicon; target is linux/amd64 or linux/arm64.
#   - Homebrew rpmbuild on macOS is unreliable.
#   - dpkg-deb for foreign arch packages is best done inside Debian.
#   - Packaging tools and arch-native verification should match the target.
#
# Product policy: engines are a required product layer (same as macOS complete
# PKG and Windows MSI). There is no app-only Linux package.
#
# Usage:
#   ./scripts/linux/build-packages.sh                 # amd64
#   GOARCH=arm64 VERSION=2.0.0 ./scripts/linux/build-packages.sh
#
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
PACKAGING_DIR="$ROOT_DIR/scripts/linux/packaging"
VERSION=${VERSION:-2.0.0-dev}
GOARCH=${GOARCH:-amd64}
for arg in "$@"; do
  case "$arg" in
    --skip-engines|--exes-only|--binaries-only)
      echo "error: Linux packages always include native engines (required product layer)" >&2
      echo "       ($arg is not allowed — build engines via scripts/linux/build-*.sh first)" >&2
      exit 1
      ;;
  esac
done

case "$GOARCH" in
  amd64)
    DOCKER_PLATFORM=linux/amd64
    DEB_ARCH=amd64
    RPM_ARCH=x86_64
    RPM_TARGET=x86_64
    ;;
  arm64)
    DOCKER_PLATFORM=linux/arm64
    DEB_ARCH=arm64
    RPM_ARCH=aarch64
    RPM_TARGET=aarch64
    ;;
  *)
    echo "unsupported GOARCH=$GOARCH (use amd64 or arm64)" >&2
    exit 1
    ;;
esac

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required for Linux package builds on this host" >&2
  exit 1
fi

OUT_DIR="$ROOT_DIR/.build/linux/$GOARCH"
STAGE_DIR="$OUT_DIR/stage"
DEB_ROOT="$OUT_DIR/deb_root"
RPM_ROOT="$OUT_DIR/rpm_build"
BIN_DIR="$OUT_DIR/bin"
mkdir -p "$OUT_DIR" "$BIN_DIR"

echo "=== VPNToris Linux packages ==="
echo "version:  $VERSION"
echo "GOARCH:   $GOARCH"
echo "platform: $DOCKER_PLATFORM"
echo "out:      $OUT_DIR"
echo

# ─── 1. Build binaries inside target-arch container ───────────────
echo "[1/4] Building Linux binaries via Docker ($DOCKER_PLATFORM)..."
BUILDER_TAG="vpntoris-linux-builder:${GOARCH}"
rm -rf "$BIN_DIR"
mkdir -p "$BIN_DIR"
# BuildKit local export avoids docker create on multi-stage/scratch images.
DOCKER_BUILDKIT=1 docker build \
  --platform "$DOCKER_PLATFORM" \
  -f "$PACKAGING_DIR/Dockerfile.builder" \
  --build-arg "TARGETPLATFORM=$DOCKER_PLATFORM" \
  --build-arg "GOARCH=$GOARCH" \
  --build-arg "VERSION=$VERSION" \
  -t "$BUILDER_TAG" \
  --output "type=local,dest=$BIN_DIR" \
  "$ROOT_DIR"

# Local export may nest under out/ and may include base-image FS layers; keep only our bins.
if [[ -d $BIN_DIR/out ]]; then
  mv "$BIN_DIR/out/"* "$BIN_DIR/" 2>/dev/null || true
  rm -rf "$BIN_DIR/out"
fi
# Drop busybox rootfs leftovers from local export (bin/dev/etc/...).
find "$BIN_DIR" -mindepth 1 -maxdepth 1 ! -name 'vpntoris*' -exec rm -rf {} +
for bin in vpntorisd vpntoris-native-helper vpntoris-service vpntorisctl vpntoris-tray; do
  test -x "$BIN_DIR/$bin" || { echo "missing binary: $bin" >&2; exit 1; }
done
chmod 755 "$BIN_DIR"/vpntoris*
echo "[1/4] Binaries:"
ls -la "$BIN_DIR"

# ─── 2. Stage filesystem layout ───────────────────────────────────
echo "[2/4] Staging package root..."
rm -rf "$STAGE_DIR"
mkdir -p \
  "$STAGE_DIR/usr/lib/vpntoris" \
  "$STAGE_DIR/usr/bin" \
  "$STAGE_DIR/usr/lib/systemd/system" \
  "$STAGE_DIR/usr/share/applications" \
  "$STAGE_DIR/var/lib/vpntoris/engines" \
  "$STAGE_DIR/var/log/vpntoris"

cp "$BIN_DIR/vpntorisd" "$STAGE_DIR/usr/lib/vpntoris/"
cp "$BIN_DIR/vpntoris-native-helper" "$STAGE_DIR/usr/lib/vpntoris/"
cp "$BIN_DIR/vpntoris-service" "$STAGE_DIR/usr/lib/vpntoris/"
cp "$BIN_DIR/vpntorisctl" "$STAGE_DIR/usr/bin/"
cp "$BIN_DIR/vpntoris-tray" "$STAGE_DIR/usr/bin/"
cp "$PACKAGING_DIR/vpntoris-native.service" "$STAGE_DIR/usr/lib/systemd/system/"
cp "$PACKAGING_DIR/vpntoris-tray.desktop" "$STAGE_DIR/usr/share/applications/"
chmod 755 "$STAGE_DIR/usr/lib/vpntoris/"* "$STAGE_DIR/usr/bin/vpntorisctl" "$STAGE_DIR/usr/bin/vpntoris-tray"

ENGINE_SRC="$ROOT_DIR/.build/native-engines/linux-$GOARCH"
if [[ ! -x $ENGINE_SRC/openvpn/bin/openvpn ]]; then
  echo "[2/4] engines missing — building Linux engines for $GOARCH..."
  GOARCH="$GOARCH" "$ROOT_DIR/scripts/linux/build-openvpn.sh"
  GOARCH="$GOARCH" "$ROOT_DIR/scripts/linux/build-openconnect.sh" || true
  GOARCH="$GOARCH" "$ROOT_DIR/scripts/linux/build-openfortivpn.sh" || true
  GOARCH="$GOARCH" "$ROOT_DIR/scripts/linux/build-strongswan.sh" || true
fi
if [[ ! -x $ENGINE_SRC/openvpn/bin/openvpn ]]; then
  echo "error: openvpn engine missing under $ENGINE_SRC" >&2
  echo "       Linux product packages cannot ship without engines" >&2
  exit 1
fi
echo "[2/4] Bundling engines from $ENGINE_SRC (required product layer)"
mkdir -p "$STAGE_DIR/var/lib/vpntoris/engines/linux-$GOARCH"
cp -a "$ENGINE_SRC/." "$STAGE_DIR/var/lib/vpntoris/engines/linux-$GOARCH/"

# ─── 3. DEB via Debian container ──────────────────────────────────
echo "[3/4] Building DEB (Docker / dpkg-deb)..."
rm -rf "$DEB_ROOT"
mkdir -p "$DEB_ROOT"
cp -a "$STAGE_DIR/." "$DEB_ROOT/"
mkdir -p "$DEB_ROOT/DEBIAN"
sed -e "s/__VERSION__/$VERSION/g" -e "s/__DEB_ARCH__/$DEB_ARCH/g" \
  "$PACKAGING_DIR/deb/control.in" >"$DEB_ROOT/DEBIAN/control"
cp "$PACKAGING_DIR/deb/postinst" "$PACKAGING_DIR/deb/prerm" "$PACKAGING_DIR/deb/postrm" "$DEB_ROOT/DEBIAN/"
chmod 755 "$DEB_ROOT/DEBIAN/postinst" "$DEB_ROOT/DEBIAN/prerm" "$DEB_ROOT/DEBIAN/postrm"

DEB_IMAGE="vpntoris-deb-builder:${GOARCH}"
DOCKER_BUILDKIT=1 docker build \
  --platform "$DOCKER_PLATFORM" \
  -f "$PACKAGING_DIR/Dockerfile.deb" \
  --build-arg "TARGETPLATFORM=$DOCKER_PLATFORM" \
  -t "$DEB_IMAGE" \
  "$PACKAGING_DIR" >/dev/null

DEB_NAME="vpntoris_${VERSION}_${DEB_ARCH}.deb"
docker run --rm --platform "$DOCKER_PLATFORM" \
  -v "$DEB_ROOT:/work/root" \
  -v "$OUT_DIR:/work/out" \
  "$DEB_IMAGE" \
  bash -ec "dpkg-deb --build /work/root /work/out/$DEB_NAME && dpkg-deb -I /work/out/$DEB_NAME | head -20"
echo "[3/4] DEB: $OUT_DIR/$DEB_NAME"

# ─── 4. RPM via Rocky container ───────────────────────────────────
echo "[4/4] Building RPM (Docker / rpmbuild)..."
rm -rf "$RPM_ROOT"
mkdir -p "$RPM_ROOT"/{BUILD,RPMS,SOURCES,SPECS,SRPMS}
cp "$BIN_DIR/vpntorisd" "$BIN_DIR/vpntoris-native-helper" "$BIN_DIR/vpntoris-service" \
  "$BIN_DIR/vpntorisctl" "$BIN_DIR/vpntoris-tray" \
  "$PACKAGING_DIR/vpntoris-native.service" "$PACKAGING_DIR/vpntoris-tray.desktop" \
  "$RPM_ROOT/SOURCES/"
mkdir -p "$RPM_ROOT/SOURCES/engines/linux-$GOARCH"
cp -a "$ENGINE_SRC/." "$RPM_ROOT/SOURCES/engines/linux-$GOARCH/"
sed -e "s/__VERSION__/${VERSION//-/_}/g" -e "s/__RPM_ARCH__/$RPM_ARCH/g" \
  "$PACKAGING_DIR/rpm/vpntoris.spec.in" >"$RPM_ROOT/SPECS/vpntoris.spec"

RPM_IMAGE="vpntoris-rpm-builder:${GOARCH}"
if ! docker image inspect "$RPM_IMAGE" >/dev/null 2>&1; then
  echo "[4/4] Preparing RPM builder image ($RPM_IMAGE)..."
  DOCKER_BUILDKIT=1 docker build \
    --platform "$DOCKER_PLATFORM" \
    -f "$PACKAGING_DIR/Dockerfile.rpm" \
    --build-arg "TARGETPLATFORM=$DOCKER_PLATFORM" \
    -t "$RPM_IMAGE" \
    "$PACKAGING_DIR"
else
  echo "[4/4] Using cached RPM builder image $RPM_IMAGE"
fi

RPM_LOG="$OUT_DIR/rpm_docker.log"
if docker run --rm --platform "$DOCKER_PLATFORM" \
  -v "$RPM_ROOT:/build" \
  "$RPM_IMAGE" \
  rpmbuild --define "_topdir /build" --target "$RPM_TARGET" -bb /build/SPECS/vpntoris.spec \
  >"$RPM_LOG" 2>&1; then
  find "$RPM_ROOT/RPMS" -type f -name '*.rpm' -exec cp {} "$OUT_DIR/" \;
  echo "[4/4] RPM build OK"
  ls -la "$OUT_DIR"/*.rpm 2>/dev/null || true
else
  echo "[4/4] RPM build FAILED — error context:" >&2
  # Prefer the real failure (unpackaged files / error:) over a long file list tail.
  if grep -q 'unpackaged file' "$RPM_LOG" 2>/dev/null; then
    grep -n -E 'error:|unpackaged|RPM build errors' "$RPM_LOG" | head -20 | sed 's/^/  /' >&2
    echo "  (full log: $RPM_LOG)" >&2
  else
    tail -40 "$RPM_LOG" | sed 's/^/  /' >&2
  fi
  exit 1
fi

echo
echo "=== Done ==="
echo "Artifacts under $OUT_DIR:"
ls -la "$OUT_DIR"/*.{deb,rpm} 2>/dev/null || ls -la "$OUT_DIR"
echo
echo "Install (Debian/Ubuntu): sudo dpkg -i $OUT_DIR/$DEB_NAME"
echo "Install (RHEL/Rocky):    sudo rpm -Uvh $OUT_DIR/*.rpm"
