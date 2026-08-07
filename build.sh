#!/bin/bash
# VPNToris local release builder (this machine only).
#
# Maintainer-host release builder:
#   - macOS host (Apple Silicon OK)
#   - Linux packages built via Docker (dpkg-deb / rpmbuild inside containers)
#   - Windows PE cross-compiled here, Authenticode via SafeNet + osslsigncode
#   - macOS app/PKG via scripts/release.sh
#
# Usage:
#   ./build.sh 2.0.0
#   ./build.sh 2.0.0 linux
#   ./build.sh 2.0.0 windows    # complete MSI only (PE + engines)
#   ./build.sh 2.0.0 darwin     # complete PKG only (app + engines); never app-only .pkg
#   ./build.sh 2.0.0 all --skip-sign
#
# Product policy (all platforms):
#   Engines are a required product layer. Ship only complete installers:
#     macOS  → *-universal-complete.pkg
#     Linux  → DEB/RPM with engines
#     Windows→ *-windows-amd64.msi with engines
#   No app-only / exe-only / engine-skipped release artifacts.
#
# Env (.env loaded automatically):
#   LINUX_ARCHS=amd64 arm64     # default: amd64 arm64
#   SIGN_WINDOWS=true           # enable SafeNet Authenticode (PE + MSI)
#   SAFENET_PIN=...             # required when SIGN_WINDOWS=true
#   SIGN_PKCS11_MODULE / SIGN_PKCS11_KEY / SIGN_CERT_FILE
#   VPNTORIS_MACOS_* identities for darwin packaging
#   VPNTORIS_MACOS_NOTARY_PROFILE / NOTARY_PROFILE  (keychain profile name)
#   APPLE_TEAM_ID=...
#
# Output layout:
#   versions/<ver>/macos/   complete.pkg
#   versions/<ver>/linux/   .deb .rpm
#   versions/<ver>/windows/ complete.msi
#
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")" && pwd)
SCRIPTS="$ROOT_DIR/scripts"
VERSIONS_DIR="$ROOT_DIR/versions"
BUILD_ROOT="$ROOT_DIR/.build"

usage() {
  cat <<'EOF'
Usage: ./build.sh <version> [platform] [flags]

Platform: all | darwin | linux | windows   (default: all)

Flags:
  --skip-sign       Skip Windows Authenticode only; macOS complete PKG is ALWAYS signed
  --skip-notarize   Skip Apple notarization of complete PKG (still codesigned + productbuild signed)
  --skip-tests      Skip go test before packaging
  --linux-amd64     Only linux/amd64 packages
  --linux-arm64     Only linux/arm64 packages

Product policy:
  Native engines are a required product layer on every platform.
  Only complete installers are shippable under platform folders:
    versions/<ver>/macos/   VPNToris-<ver>-universal-complete.pkg
    versions/<ver>/linux/   *.deb *.rpm (engines embedded)
    versions/<ver>/windows/ VPNToris-<ver>-windows-amd64.msi
  --skip-engines / --exes-only are rejected.

Notarization (macOS complete PKG):
  Requires VPNTORIS_MACOS_NOTARY_PROFILE (or NOTARY_PROFILE) in the environment.

Examples:
  ./build.sh 2.0.0
  ./build.sh 2.0.0 darwin
  ./build.sh 2.0.0 linux
  SAFENET_PIN='****' ./build.sh 2.0.0 windows
EOF
  exit 1
}

if [[ $# -lt 1 ]]; then
  usage
fi

VERSION="$1"
shift || true
PLATFORM="${1:-all}"
if [[ $# -gt 0 && $1 != --* ]]; then
  shift || true
fi

if ! [[ $VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9.]+)?$ ]]; then
  echo "error: version must look like 2.0.0 or 2.0.0-rc1" >&2
  exit 1
fi
if ! [[ $PLATFORM =~ ^(all|darwin|linux|windows)$ ]]; then
  echo "error: invalid platform: $PLATFORM" >&2
  usage
fi

SKIP_SIGN=false
SKIP_NOTARIZE=false
SKIP_TESTS=false
LINUX_ARCHS_OVERRIDE=""
for arg in "$@"; do
  case "$arg" in
    --skip-sign) SKIP_SIGN=true ;;
    --skip-notarize|--skip-notarization) SKIP_NOTARIZE=true ;;
    --skip-engines|--exes-only|--exe-only|--binaries-only)
      echo "error: native engines are a required product layer on every platform" >&2
      echo "       ($arg is not allowed — there is no app-only / exe-only release build)" >&2
      exit 1
      ;;
    --skip-tests) SKIP_TESTS=true ;;
    --linux-amd64) LINUX_ARCHS_OVERRIDE="amd64" ;;
    --linux-arm64) LINUX_ARCHS_OVERRIDE="arm64" ;;
    -h|--help) usage ;;
    *) echo "error: unknown flag: $arg" >&2; usage ;;
  esac
done

# ─── Host gate: builds are taken only from this macOS machine ─────
if [[ $(uname -s) != "Darwin" ]]; then
  echo "error: build.sh is designed for the maintainer macOS host only (got $(uname -s))" >&2
  exit 1
fi

if [[ -f "$ROOT_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +a
fi

PRODUCT_NAME=${PRODUCT_NAME:-VPNToris}
BUILD_TIME=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
HOST_ARCH=$(uname -m)
GO_VERSION=$(go version 2>/dev/null | awk '{print $3}' || echo unknown)

# macOS notary — profile/team come from the environment only (no private defaults).
APPLE_TEAM_ID=${APPLE_TEAM_ID:-}
if [[ -z ${VPNTORIS_MACOS_NOTARY_PROFILE:-} ]]; then
  VPNTORIS_MACOS_NOTARY_PROFILE=${NOTARY_PROFILE:-}
fi
NOTARY_PROFILE=${VPNTORIS_MACOS_NOTARY_PROFILE}

if [[ -n $LINUX_ARCHS_OVERRIDE ]]; then
  # shellcheck disable=SC2206
  LINUX_ARCHS=($LINUX_ARCHS_OVERRIDE)
elif [[ -n ${LINUX_ARCHS:-} ]]; then
  # shellcheck disable=SC2206
  LINUX_ARCHS=($LINUX_ARCHS)
else
  # Ship both arches by default when building from this machine.
  LINUX_ARCHS=(amd64 arm64)
fi

VERSION_OUT="$VERSIONS_DIR/$VERSION"
VERSION_MACOS="$VERSION_OUT/macos"
VERSION_LINUX="$VERSION_OUT/linux"
VERSION_WINDOWS="$VERSION_OUT/windows"
mkdir -p "$VERSION_OUT" "$VERSION_MACOS" "$VERSION_LINUX" "$VERSION_WINDOWS" "$BUILD_ROOT"

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "error: missing prerequisite: $1" >&2
    exit 1
  }
}

echo "=== VPNToris local build ==="
echo "version:   $VERSION"
echo "platform:  $PLATFORM"
echo "host:      Darwin/$HOST_ARCH"
echo "go:        $GO_VERSION"
echo "time:      $BUILD_TIME"
echo "out:       $VERSION_OUT"
echo "linux:     ${LINUX_ARCHS[*]}"
echo

# ─── Tests ────────────────────────────────────────────────────────
run_tests() {
  if [[ $SKIP_TESTS == true ]]; then
    echo "[test] skipped"
    return 0
  fi
  echo "[test] go test ./..."
  (cd "$ROOT_DIR/vpntoris-tray" && go test ./...)
}

# ─── Darwin (always signed complete PKG = only shippable product) ─
# App-only universal.pkg / DMG / engine-only PKG are intermediate build steps.
# Release drop: versions/<ver>/macos/*-universal-complete.pkg only.
build_darwin() {
  echo "[darwin] signed + notarized complete PKG (app + engines)..."
  require_cmd go
  require_cmd xcrun
  require_cmd pkgbuild
  require_cmd productbuild
  mkdir -p "$VERSION_MACOS"

  # macOS artifacts are ALWAYS signed on this host. --skip-sign only affects Windows.
  if [[ -z ${VPNTORIS_MACOS_APPLICATION_IDENTITY:-} || -z ${VPNTORIS_MACOS_INSTALLER_IDENTITY:-} ]]; then
    echo "error: darwin builds require VPNTORIS_MACOS_APPLICATION_IDENTITY and" >&2
    echo "       VPNTORIS_MACOS_INSTALLER_IDENTITY in .env (unsigned macOS packages are not allowed)" >&2
    exit 1
  fi

  if [[ $SKIP_NOTARIZE == false ]]; then
    xcrun --find notarytool >/dev/null 2>&1 || {
      echo "error: notarytool not found (Xcode CLT required)" >&2
      exit 1
    }
    xcrun --find stapler >/dev/null 2>&1 || {
      echo "error: stapler not found (Xcode CLT required)" >&2
      exit 1
    }
    if [[ -z ${NOTARY_PROFILE:-} ]]; then
      echo "error: VPNTORIS_MACOS_NOTARY_PROFILE (or NOTARY_PROFILE) is required for notarization" >&2
      echo "       set it in .env or pass --skip-notarize" >&2
      exit 1
    fi
    if ! xcrun notarytool history --keychain-profile "$NOTARY_PROFILE" >/dev/null 2>&1; then
      echo "error: notarytool keychain profile '$NOTARY_PROFILE' not found" >&2
      echo "       store credentials once:" >&2
      echo "         xcrun notarytool store-credentials $NOTARY_PROFILE \\" >&2
      echo "             --apple-id <email> --team-id \${APPLE_TEAM_ID} --password <app-specific-password>" >&2
      exit 1
    fi
    echo "[darwin] notary profile: $NOTARY_PROFILE${APPLE_TEAM_ID:+ (team $APPLE_TEAM_ID)}"
  else
    echo "[darwin] notarization skipped (--skip-notarize)"
  fi

  local release_args=(--skip-notarization)
  # Intermediate app PKG is never the product; only complete PKG is notarized.

  echo "[darwin] 1/3 application package (intermediate, signed)..."
  VERSION="$VERSION" ARCH=universal \
    "$SCRIPTS/release.sh" "${release_args[@]+"${release_args[@]}"}"

  local app_pkg="$ROOT_DIR/dist/VPNToris-$VERSION-universal.pkg"
  local engine_pkg="$BUILD_ROOT/VPNToris-Native-Engine-$VERSION-signed.pkg"
  local complete_pkg="$ROOT_DIR/dist/VPNToris-$VERSION-universal-complete.pkg"

  if [[ ! -f $app_pkg ]]; then
    echo "error: intermediate app package missing: $app_pkg" >&2
    exit 1
  fi

  echo "[darwin] 2/3 native engine package (intermediate, signed; engines required)..."
  "$SCRIPTS/macos/build-native-test-pkg.sh" "$engine_pkg"
  if [[ ! -f $engine_pkg ]]; then
    echo "error: intermediate engine package missing: $engine_pkg" >&2
    echo "       complete macOS product cannot ship without engines" >&2
    exit 1
  fi

  echo "[darwin] 3/3 complete installer (ONLY shippable product)..."
  VERSION="$VERSION" ARCH=universal \
    "$SCRIPTS/macos/build-complete-pkg.sh" "$app_pkg" "$engine_pkg" "$complete_pkg"

  if [[ ! -f $complete_pkg ]]; then
    echo "error: complete package was not produced: $complete_pkg" >&2
    exit 1
  fi
  pkgutil --check-signature "$complete_pkg"
  shasum -a 256 "$complete_pkg" >"$complete_pkg.sha256"

  # Notarize + staple complete PKG only.
  if [[ $SKIP_NOTARIZE == false ]]; then
    echo "[darwin] notarizing complete package (notarytool --wait)..."
    local notary_log="$BUILD_ROOT/notary-complete-$VERSION.log"
    if ! xcrun notarytool submit "$complete_pkg" \
      --keychain-profile "$NOTARY_PROFILE" \
      --wait \
      --output-format json >"$notary_log" 2>&1; then
      echo "error: notarization failed" >&2
      sed 's/^/  /' "$notary_log" >&2 || true
      local sub_id
      sub_id=$(grep -o '"id":"[^"]*"' "$notary_log" | head -1 | cut -d'"' -f4 || true)
      if [[ -n ${sub_id:-} ]]; then
        xcrun notarytool log "$sub_id" --keychain-profile "$NOTARY_PROFILE" 2>&1 | sed 's/^/  /' >&2 || true
      fi
      exit 1
    fi
    sed 's/^/  /' "$notary_log" || true
    if ! grep -q '"status":"Accepted"' "$notary_log"; then
      echo "error: notarization did not return Accepted" >&2
      local sub_id
      sub_id=$(grep -o '"id":"[^"]*"' "$notary_log" | head -1 | cut -d'"' -f4 || true)
      if [[ -n ${sub_id:-} ]]; then
        xcrun notarytool log "$sub_id" --keychain-profile "$NOTARY_PROFILE" 2>&1 | sed 's/^/  /' >&2 || true
      fi
      exit 1
    fi
    echo "[darwin] stapling notarization ticket..."
    xcrun stapler staple "$complete_pkg"
    xcrun stapler validate "$complete_pkg"
    # Refresh checksum after staple mutates the file
    shasum -a 256 "$complete_pkg" >"$complete_pkg.sha256"
  fi

  # Product drop: versions/<ver>/macos/ only
  cp -f "$complete_pkg" "$VERSION_MACOS/"
  cp -f "$complete_pkg.sha256" "$VERSION_MACOS/"
  if [[ -f $ROOT_DIR/dist/VPNToris-sbom.txt ]]; then
    cp -f "$ROOT_DIR/dist/VPNToris-sbom.txt" "$VERSION_MACOS/"
  fi

  echo "[darwin] shippable:"
  echo "  $VERSION_MACOS/$(basename "$complete_pkg")"
  cat "$complete_pkg.sha256" | sed 's|^|  |'
  echo "[darwin] intermediate (not shipped): $(basename "$app_pkg"), $(basename "$engine_pkg")"
  echo "[darwin] done"
  echo
}

# ─── Linux (Docker) ───────────────────────────────────────────────
build_linux() {
  echo "[linux] Docker-based packages for: ${LINUX_ARCHS[*]}"
  require_cmd docker
  mkdir -p "$VERSION_LINUX"
  if ! docker info >/dev/null 2>&1; then
    echo "error: Docker daemon is not running" >&2
    exit 1
  fi

  for arch in "${LINUX_ARCHS[@]}"; do
    echo "[linux] --- GOARCH=$arch ---"
    # Engines are a required product layer (same as macOS complete.pkg / Windows MSI).
    VERSION="$VERSION" GOARCH="$arch" \
      "$SCRIPTS/linux/build-packages.sh"

    local out="$BUILD_ROOT/linux/$arch"
    # Shippable product only under versions/<ver>/linux/
    if compgen -G "$out/*.deb" >/dev/null; then
      cp -f "$out"/*.deb "$VERSION_LINUX/"
    else
      echo "error: no DEB produced for linux/$arch" >&2
      exit 1
    fi
    if compgen -G "$out/*.rpm" >/dev/null; then
      cp -f "$out"/*.rpm "$VERSION_LINUX/"
    else
      echo "error: no RPM produced for linux/$arch" >&2
      exit 1
    fi
  done

  if [[ $SKIP_SIGN == false && -n ${VPNTORIS_LINUX_GPG_KEY_ID:-} ]]; then
    echo "[linux] GPG detach-signing packages..."
    local artifacts=()
    while IFS= read -r -d '' f; do
      artifacts+=("$f")
    done < <(find "$VERSION_LINUX" \( -name '*.deb' -o -name '*.rpm' \) -print0)
    if [[ ${#artifacts[@]} -gt 0 ]]; then
      "$SCRIPTS/linux/sign-artifacts.sh" "${artifacts[@]}"
    fi
  else
    echo "[linux] GPG signing skipped (set VPNTORIS_LINUX_GPG_KEY_ID or omit --skip-sign)"
  fi
  echo "[linux] shippable under $VERSION_LINUX"
  ls -la "$VERSION_LINUX" | sed 's/^/  /'
  echo "[linux] done"
  echo
}

# ─── Windows (complete MSI + engines, same role as macOS complete.pkg) ─
build_windows() {
  echo "[windows] complete MSI (PE + required engines) on macOS..."
  require_cmd go
  require_cmd wixl
  local out="$BUILD_ROOT/windows"
  mkdir -p "$out"

  # Ergonomics: SAFENET_PIN alone enables signing (SIGN_WINDOWS defaults true in .env).
  if [[ -n ${SAFENET_PIN:-} && ${SIGN_WINDOWS:-} != "false" ]]; then
    SIGN_WINDOWS=true
  fi

  local pack_args=()
  if [[ $SKIP_SIGN == true ]]; then
    pack_args+=(--skip-sign)
  elif [[ ${SIGN_WINDOWS:-false} == true ]]; then
    if [[ -z ${SAFENET_PIN:-} ]]; then
      echo "error: Windows signing is enabled but SAFENET_PIN is empty" >&2
      echo "       Usage: SAFENET_PIN=**** ./build.sh $VERSION windows" >&2
      exit 1
    fi
    require_cmd osslsigncode
  else
    pack_args+=(--skip-sign)
  fi

  # Always: PE + engines + complete MSI. No app-only path.
  OUT_DIR="$out" VERSION="$VERSION" SIGN_WINDOWS="${SIGN_WINDOWS:-false}" \
    "$SCRIPTS/windows/build-and-sign.sh" "${pack_args[@]+"${pack_args[@]}"}"

  # Shippable product only under versions/<ver>/windows/
  mkdir -p "$VERSION_WINDOWS"
  local msi_found=false
  for f in "$out"/*-windows-amd64.msi; do
    if [[ -f $f ]]; then
      cp -f "$f" "$VERSION_WINDOWS/"
      if [[ -f $f.sha256 ]]; then
        cp -f "$f.sha256" "$VERSION_WINDOWS/"
      fi
      msi_found=true
    fi
  done
  if [[ $msi_found != true ]]; then
    echo "error: complete Windows MSI was not produced" >&2
    exit 1
  fi

  if [[ $SKIP_SIGN == false && ${SIGN_WINDOWS:-false} != true ]]; then
    echo "[windows] note: MSI is UNSIGNED"
    echo "         Sign with: SAFENET_PIN=**** ./build.sh $VERSION windows"
  fi
  echo "[windows] shippable under $VERSION_WINDOWS"
  ls -la "$VERSION_WINDOWS" | sed 's/^/  /'
  echo "[windows] done (complete MSI with engines)"
  echo
}

# ─── Manifest ─────────────────────────────────────────────────────
write_manifest() {
  local manifest="$VERSION_OUT/BUILD_MANIFEST.txt"
  {
    echo "VPNToris $VERSION"
    echo "built_at=$BUILD_TIME"
    echo "host=Darwin/$HOST_ARCH"
    echo "go=$GO_VERSION"
    echo "platform=$PLATFORM"
    echo "linux_archs=${LINUX_ARCHS[*]}"
    echo "skip_sign=$SKIP_SIGN"
    echo "skip_notarize=$SKIP_NOTARIZE"
    echo "notary_profile=$NOTARY_PROFILE"
    echo "engines=required"
    echo "sign_windows=${SIGN_WINDOWS:-false}"
    echo "layout=macos/ linux/ windows/"
    echo
    echo "artifacts:"
    (cd "$VERSION_OUT" && find . -type f | sed 's|^\./||' | sort)
  } >"$manifest"
  echo "[manifest] $manifest"
}

# ─── Run ──────────────────────────────────────────────────────────
run_tests

case "$PLATFORM" in
  darwin) build_darwin ;;
  linux) build_linux ;;
  windows) build_windows ;;
  all)
    build_darwin
    build_linux
    build_windows
    ;;
esac

write_manifest

echo "=== Build complete ==="
echo "Output: $VERSION_OUT"
ls -la "$VERSION_OUT" | sed 's/^/  /'
echo
echo "This tree is the release drop from this machine only."
echo "Do not re-build the same version on another host without changing the version."
