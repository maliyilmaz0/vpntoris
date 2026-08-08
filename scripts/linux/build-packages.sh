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
  "$STAGE_DIR/usr/lib/systemd/user" \
  "$STAGE_DIR/usr/share/applications" \
  "$STAGE_DIR/etc/xdg/autostart" \
  "$STAGE_DIR/usr/share/icons/hicolor/16x16/apps" \
  "$STAGE_DIR/usr/share/icons/hicolor/22x22/apps" \
  "$STAGE_DIR/usr/share/icons/hicolor/24x24/apps" \
  "$STAGE_DIR/usr/share/icons/hicolor/32x32/apps" \
  "$STAGE_DIR/usr/share/icons/hicolor/48x48/apps" \
  "$STAGE_DIR/usr/share/icons/hicolor/64x64/apps" \
  "$STAGE_DIR/usr/share/icons/hicolor/128x128/apps" \
  "$STAGE_DIR/usr/share/icons/hicolor/256x256/apps" \
  "$STAGE_DIR/usr/share/pixmaps" \
  "$STAGE_DIR/var/lib/vpntoris/engines" \
  "$STAGE_DIR/var/log/vpntoris"

cp "$BIN_DIR/vpntorisd" "$STAGE_DIR/usr/lib/vpntoris/"
cp "$BIN_DIR/vpntoris-native-helper" "$STAGE_DIR/usr/lib/vpntoris/"
cp "$BIN_DIR/vpntoris-service" "$STAGE_DIR/usr/lib/vpntoris/"
cp "$BIN_DIR/vpntorisctl" "$STAGE_DIR/usr/bin/"
cp "$BIN_DIR/vpntoris-tray" "$STAGE_DIR/usr/bin/"
cp "$PACKAGING_DIR/vpntoris-tray-autostart.sh" "$STAGE_DIR/usr/lib/vpntoris/vpntoris-tray-autostart"
cp "$PACKAGING_DIR/vpntoris-native.service" "$STAGE_DIR/usr/lib/systemd/system/"
cp "$PACKAGING_DIR/vpntorisd.user.service" "$STAGE_DIR/usr/lib/systemd/user/vpntorisd.service"
cp "$PACKAGING_DIR/vpntoris-tray.desktop" "$STAGE_DIR/usr/share/applications/"
cp "$PACKAGING_DIR/vpntoris-tray-autostart.desktop" "$STAGE_DIR/etc/xdg/autostart/vpntoris-tray.desktop"
# Application icon (desktop menu + GNOME search + panel)
for size in 16 22 24 32 48 64 128 256; do
  src="$ROOT_DIR/assets/icons/vpntoris-${size}.png"
  if [[ -f $src ]]; then
    cp "$src" "$STAGE_DIR/usr/share/icons/hicolor/${size}x${size}/apps/vpntoris.png"
  fi
done
if [[ -f $ROOT_DIR/assets/icons/vpntoris-128.png ]]; then
  cp "$ROOT_DIR/assets/icons/vpntoris-128.png" "$STAGE_DIR/usr/share/pixmaps/vpntoris.png"
elif [[ -f $ROOT_DIR/assets/vpntoris-logo.png ]]; then
  cp "$ROOT_DIR/assets/vpntoris-logo.png" "$STAGE_DIR/usr/share/pixmaps/vpntoris.png"
fi
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

# ─── 2.5. Bundle AppIndicator GNOME Shell extension ───────────────
# GNOME has no built-in StatusNotifier host. Instead of running dnf/apt at
# install time (network access inside %post/postinst), the distro extension
# package is fetched at build time and shipped under a private UUID so it can
# never conflict with the distro's gnome-shell-extension-appindicator package.
EXT_UUID="vpntoris-appindicator@vpntoris.local"
EXT_DIR="$OUT_DIR/extension"
echo "[2.5] Fetching AppIndicator shell extension (build-time download)..."
rm -rf "$EXT_DIR"
mkdir -p "$EXT_DIR/deb" "$EXT_DIR/rpm"
docker run --rm --platform "$DOCKER_PLATFORM" \
  -v "$EXT_DIR/deb:/ext" \
  debian:bookworm-slim bash -ec '
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    cd /tmp
    apt-get download gnome-shell-extension-appindicator 2>/dev/null
    dpkg-deb -x gnome-shell-extension-appindicator*.deb /ext
  '
docker run --rm --platform "$DOCKER_PLATFORM" \
  -v "$EXT_DIR/rpm:/ext" \
  rockylinux:9 bash -ec '
    dnf install -y -q epel-release cpio >/dev/null 2>&1
    cd /tmp
    dnf download -y -q gnome-shell-extension-appindicator >/dev/null 2>&1
    mkdir -p /ext/x && cd /ext/x
    rpm2cpio /tmp/gnome-shell-extension-appindicator*.rpm | cpio -idm --quiet
  '

prepare_extension() {
  local src=$1 dst=$2
  if [[ ! -f $src/metadata.json ]]; then
    echo "error: AppIndicator extension payload missing at $src" >&2
    exit 1
  fi
  rm -rf "$dst"
  mkdir -p "$(dirname "$dst")"
  cp -a "$src" "$dst"
  # Private UUID + product name (host is macOS: BSD sed).
  sed -i '' \
    -e "s/\"uuid\": *\"[^\"]*\"/\"uuid\": \"$EXT_UUID\"/" \
    -e "s/\"name\": *\"[^\"]*\"/\"name\": \"VPNToris AppIndicator\"/" \
    "$dst/metadata.json"
}
# Upstream UUID differs per distro (EPEL: appindicatorsupport@…, Debian:
# ubuntu-appindicators@…), so discover the extracted extension directory.
DEB_EXT_SRC=$(find "$EXT_DIR/deb/usr/share/gnome-shell/extensions" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | head -1)
RPM_EXT_SRC=$(find "$EXT_DIR/rpm/x/usr/share/gnome-shell/extensions" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | head -1)
prepare_extension \
  "$DEB_EXT_SRC" \
  "$STAGE_DIR/usr/share/gnome-shell/extensions/$EXT_UUID"
prepare_extension \
  "$RPM_EXT_SRC" \
  "$EXT_DIR/rpm-staged"
mkdir -p "$STAGE_DIR/usr/share/glib-2.0/schemas"
cp "$PACKAGING_DIR/20_vpntoris-appindicator.gschema.override" "$STAGE_DIR/usr/share/glib-2.0/schemas/"
echo "[2.5] Bundled extension: $EXT_UUID"

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
  "$PACKAGING_DIR/vpntoris-native.service" \
  "$PACKAGING_DIR/vpntorisd.user.service" \
  "$PACKAGING_DIR/vpntoris-tray.desktop" \
  "$PACKAGING_DIR/vpntoris-tray-autostart.desktop" \
  "$PACKAGING_DIR/vpntoris-tray-autostart.sh" \
  "$RPM_ROOT/SOURCES/"
for size in 16 22 24 32 48 64 128 256; do
  if [[ -f $ROOT_DIR/assets/icons/vpntoris-${size}.png ]]; then
    cp "$ROOT_DIR/assets/icons/vpntoris-${size}.png" "$RPM_ROOT/SOURCES/vpntoris-${size}.png"
  fi
done
mkdir -p "$RPM_ROOT/SOURCES/engines/linux-$GOARCH"
cp -a "$ENGINE_SRC/." "$RPM_ROOT/SOURCES/engines/linux-$GOARCH/"
cp -a "$EXT_DIR/rpm-staged" "$RPM_ROOT/SOURCES/appindicator"
cp "$PACKAGING_DIR/20_vpntoris-appindicator.gschema.override" "$RPM_ROOT/SOURCES/"
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
