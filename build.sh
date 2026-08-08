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
Usage: ./build.sh <version> [platform] [mode] [flags]

Platform: all | darwin | linux | windows   (default: all)

Mode (optional, after platform):
  dev               Local/dev build: no Developer ID codesign, no notarization,
                    no Windows Authenticode. Engines still bundled.
                    On darwin: after build, installs the complete-dev.pkg
                    (sudo), refreshes the native helper, and opens the app.
                    Example: ./build.sh 2.0.0 darwin dev

Flags:
  --dev             Same as mode "dev" (may appear anywhere after version)
  --no-install      With darwin dev: build only, do not install/open
  --skip-sign       Skip Windows Authenticode (release macOS is still signed unless dev)
  --skip-notarize   Skip Apple notarization of complete PKG (still codesigned unless dev)
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

Notarization (macOS release complete PKG):
  Requires VPNTORIS_MACOS_NOTARY_PROFILE (or NOTARY_PROFILE) in the environment.
  Not used in dev mode.

Examples:
  ./build.sh 2.0.0
  ./build.sh 2.0.0 darwin
  ./build.sh 2.0.0 darwin dev
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

# Optional mode token after platform: "dev"
DEV_MODE=false
if [[ $# -gt 0 && $1 == "dev" ]]; then
  DEV_MODE=true
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
SKIP_DEV_INSTALL=false
LINUX_ARCHS_OVERRIDE=""
for arg in "$@"; do
  case "$arg" in
    dev|--dev) DEV_MODE=true ;;
    --no-install) SKIP_DEV_INSTALL=true ;;
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

# Dev builds never codesign with Developer ID or notarize / Authenticode.
if [[ $DEV_MODE == true ]]; then
  SKIP_SIGN=true
  SKIP_NOTARIZE=true
  export VPNTORIS_MACOS_UNSIGNED=1
  echo "[build] DEV mode: no Developer ID codesign, no notarization, no Authenticode"
fi

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

# ─── Darwin complete PKG (release: signed+notarized; dev: unsigned local) ─
# App-only universal.pkg / DMG / engine-only PKG are intermediate build steps.
# Drop: versions/<ver>/macos/*-universal-complete.pkg only.
build_darwin() {
  if [[ $DEV_MODE == true ]]; then
    echo "[darwin] DEV complete PKG (app + engines, no Developer ID / notary)..."
  else
    echo "[darwin] signed + notarized complete PKG (app + engines)..."
  fi
  require_cmd go
  require_cmd xcrun
  require_cmd pkgbuild
  require_cmd productbuild
  mkdir -p "$VERSION_MACOS"

  if [[ $DEV_MODE == false ]]; then
    if [[ -z ${VPNTORIS_MACOS_APPLICATION_IDENTITY:-} || -z ${VPNTORIS_MACOS_INSTALLER_IDENTITY:-} ]]; then
      echo "error: darwin release builds require VPNTORIS_MACOS_APPLICATION_IDENTITY and" >&2
      echo "       VPNTORIS_MACOS_INSTALLER_IDENTITY in .env" >&2
      echo "       for local/unsigned: ./build.sh $VERSION darwin dev" >&2
      exit 1
    fi
  fi

  if [[ $DEV_MODE == false && $SKIP_NOTARIZE == false ]]; then
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
      echo "       set it in .env, pass --skip-notarize, or use: ./build.sh $VERSION darwin dev" >&2
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
    if [[ $DEV_MODE == true ]]; then
      echo "[darwin] notarization skipped (dev mode)"
    else
      echo "[darwin] notarization skipped (--skip-notarize)"
    fi
  fi

  local release_args=(--skip-notarization)
  if [[ $DEV_MODE == true ]]; then
    release_args=(--unsigned)
  fi

  echo "[darwin] 1/3 application package (intermediate)..."
  VERSION="$VERSION" ARCH=universal \
    "$SCRIPTS/release.sh" "${release_args[@]+"${release_args[@]}"}"

  local app_pkg="$ROOT_DIR/dist/VPNToris-$VERSION-universal.pkg"
  local engine_pkg="$BUILD_ROOT/VPNToris-Native-Engine-$VERSION-signed.pkg"
  if [[ $DEV_MODE == true ]]; then
    engine_pkg="$BUILD_ROOT/VPNToris-Native-Engine-$VERSION-dev.pkg"
  fi
  local complete_pkg="$ROOT_DIR/dist/VPNToris-$VERSION-universal-complete.pkg"
  if [[ $DEV_MODE == true ]]; then
    complete_pkg="$ROOT_DIR/dist/VPNToris-$VERSION-universal-complete-dev.pkg"
  fi

  if [[ ! -f $app_pkg ]]; then
    echo "error: intermediate app package missing: $app_pkg" >&2
    exit 1
  fi

  echo "[darwin] 2/3 native engine package (engines required)..."
  if [[ $DEV_MODE == true ]]; then
    VPNTORIS_MACOS_UNSIGNED=1 "$SCRIPTS/macos/build-native-test-pkg.sh" "$engine_pkg"
  else
    "$SCRIPTS/macos/build-native-test-pkg.sh" "$engine_pkg"
  fi
  if [[ ! -f $engine_pkg ]]; then
    echo "error: intermediate engine package missing: $engine_pkg" >&2
    echo "       complete macOS product cannot ship without engines" >&2
    exit 1
  fi

  echo "[darwin] 3/3 complete installer..."
  if [[ $DEV_MODE == true ]]; then
    VERSION="$VERSION" ARCH=universal VPNTORIS_MACOS_UNSIGNED=1 \
      "$SCRIPTS/macos/build-complete-pkg.sh" "$app_pkg" "$engine_pkg" "$complete_pkg"
  else
    VERSION="$VERSION" ARCH=universal \
      "$SCRIPTS/macos/build-complete-pkg.sh" "$app_pkg" "$engine_pkg" "$complete_pkg"
  fi

  if [[ ! -f $complete_pkg ]]; then
    echo "error: complete package was not produced: $complete_pkg" >&2
    exit 1
  fi
  if [[ $DEV_MODE == false ]]; then
    pkgutil --check-signature "$complete_pkg"
  else
    echo "[darwin] DEV: package is unsigned (do not distribute)"
  fi
  shasum -a 256 "$complete_pkg" >"$complete_pkg.sha256"

  # Notarize + staple complete PKG only (never in dev).
  if [[ $DEV_MODE == false && $SKIP_NOTARIZE == false ]]; then
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
    shasum -a 256 "$complete_pkg" >"$complete_pkg.sha256"
  fi

  # Product drop: versions/<ver>/macos/ only
  cp -f "$complete_pkg" "$VERSION_MACOS/"
  cp -f "$complete_pkg.sha256" "$VERSION_MACOS/"
  if [[ -f $ROOT_DIR/dist/VPNToris-sbom.txt ]]; then
    cp -f "$ROOT_DIR/dist/VPNToris-sbom.txt" "$VERSION_MACOS/"
  fi

  echo "[darwin] artifact:"
  echo "  $VERSION_MACOS/$(basename "$complete_pkg")"
  cat "$complete_pkg.sha256" | sed 's|^|  |'
  echo "[darwin] intermediate: $(basename "$app_pkg"), $(basename "$engine_pkg")"
  if [[ $DEV_MODE == true ]]; then
    if [[ $SKIP_DEV_INSTALL == true ]]; then
      echo "[darwin] DEV done (unsigned build only; --no-install)"
    else
      install_darwin_dev "$complete_pkg"
    fi
  else
    echo "[darwin] done"
  fi
  echo
}

# Install unsigned complete-dev.pkg and launch the app (local iteration only).
install_darwin_dev() {
  local pkg=$1
  local app=/Applications/VPNToris.app
  local helper=/Library/PrivilegedHelperTools/com.vpntoris.native-helper
  local plist=/Library/LaunchDaemons/com.vpntoris.native-helper.plist
  local engines="/Library/Application Support/VPNToris/Engines"
  local label=com.vpntoris.native-helper

  echo "[darwin-dev] installing $pkg (requires sudo)..."
  require_cmd installer
  require_cmd open

  # Stop running UI/daemon so the package can replace binaries.
  /usr/bin/pkill -x VPNToris 2>/dev/null || true
  /usr/bin/pkill -x vpntorisd 2>/dev/null || true
  sleep 0.5

  # sudo keeps a short credential cache so one password prompt covers the rest.
  if ! sudo -v; then
    echo "error: sudo required to install the dev package" >&2
    exit 1
  fi

  if ! sudo /usr/sbin/installer -pkg "$pkg" -target / -verboseR; then
    echo "error: installer failed for $pkg" >&2
    exit 1
  fi

  echo "[darwin-dev] clearing quarantine / ad-hoc signing (required for launchd on modern macOS)..."
  sudo /usr/bin/xattr -dr com.apple.quarantine "$app" 2>/dev/null || true
  sudo /usr/bin/xattr -dr com.apple.quarantine "$helper" "$engines" 2>/dev/null || true

  # release.sh --unsigned leaves Mach-Os with no signature. launchd then fails with
  # RBSRequestErrorDomain Code=5 / POSIX 163 ("Launchd job spawn failed").
  # Ad-hoc sign the app bundle + nested binaries so local open works without Developer ID.
  if [[ -d $app ]]; then
    # Sign nested helpers first, then the outer bundle.
    for bin in tun2socks vpntoris-route-helper vpntorisd vpntorisctl VPNToris; do
      if [[ -f $app/Contents/MacOS/$bin ]]; then
        sudo /usr/bin/codesign --force --sign - "$app/Contents/MacOS/$bin" 2>/dev/null || true
      fi
    done
    sudo /usr/bin/codesign --force --deep --sign - "$app" || {
      echo "error: ad-hoc codesign of $app failed" >&2
      exit 1
    }
  fi

  if [[ -f $helper ]]; then
    sudo /usr/sbin/chown root:wheel "$helper" 2>/dev/null || true
    sudo /bin/chmod 0755 "$helper" 2>/dev/null || true
    sudo /usr/bin/codesign --force --sign - "$helper" 2>/dev/null || true
  fi
  # Engines were ad-hoc signed at package time; re-sign helper-adjacent bits if present.
  if [[ -d $engines ]]; then
    sudo /usr/bin/find "$engines" -type f \( -perm -111 -o -name '*.dylib' -o -name '*.so' \) -print0 2>/dev/null \
      | while IFS= read -r -d '' f; do
          sudo /usr/bin/codesign --force --sign - "$f" 2>/dev/null || true
        done
  fi

  if [[ -f $plist ]]; then
    local console_uid
    console_uid=$(/usr/bin/stat -f %u /dev/console 2>/dev/null || echo "$(id -u)")
    sudo /usr/libexec/PlistBuddy -c "Set :ProgramArguments:2 $console_uid" "$plist" 2>/dev/null || true
    sudo /usr/sbin/chown root:wheel "$plist" 2>/dev/null || true
    sudo /bin/chmod 0644 "$plist" 2>/dev/null || true
    sudo /bin/launchctl bootout "system/$label" 2>/dev/null || true
    if ! sudo /bin/launchctl bootstrap system "$plist" 2>/dev/null; then
      # Already loaded or race — try kickstart
      sudo /bin/launchctl kickstart -k "system/$label" 2>/dev/null || true
    else
      sudo /bin/launchctl enable "system/$label" 2>/dev/null || true
    fi
  fi

  # Symlinks for CLI (best-effort; may already exist).
  if [[ -x $app/Contents/MacOS/vpntorisctl ]]; then
    sudo /bin/mkdir -p /usr/local/bin /opt/homebrew/bin 2>/dev/null || true
    sudo /bin/ln -sfn "$app/Contents/MacOS/vpntorisctl" /usr/local/bin/vpntorisctl 2>/dev/null || true
    sudo /bin/ln -sfn "$app/Contents/MacOS/vpntorisctl" /opt/homebrew/bin/vpntorisctl 2>/dev/null || true
  fi

  if [[ ! -d $app ]]; then
    echo "error: $app missing after install" >&2
    exit 1
  fi

  # Ensure ownership allows the console user to launch (pkg may install as root:wheel).
  local console_user
  console_user=$(/usr/bin/stat -f %Su /dev/console 2>/dev/null || echo "$(id -un)")
  if [[ -n $console_user && $console_user != root && $console_user != loginwindow ]]; then
    sudo /usr/sbin/chown -R "$console_user:staff" "$app" 2>/dev/null || true
  fi

  echo "[darwin-dev] opening VPNToris..."
  # Prefer direct exec path if open fails under some launch policies.
  if ! /usr/bin/open -a "$app" 2>/dev/null; then
    if ! /usr/bin/open "$app" 2>/dev/null; then
      echo "[darwin-dev] open failed; launching binary directly..."
      /usr/bin/open -n "$app/Contents/MacOS/VPNToris" 2>/dev/null \
        || "$app/Contents/MacOS/VPNToris" >/tmp/vpntoris-dev-launch.log 2>&1 &
      sleep 1
    fi
  fi
  sleep 1
  if ! /usr/bin/pgrep -x VPNToris >/dev/null 2>&1; then
    echo "error: VPNToris did not stay running after launch" >&2
    echo "       try: codesign -dv /Applications/VPNToris.app && open -a VPNToris" >&2
    exit 1
  fi

  echo "[darwin-dev] installed + launched"
  echo "  app:    $app"
  echo "  helper: $helper"
  if [[ -S /var/run/vpntoris-native/helper.sock ]] || ls /var/run/vpntoris-native/helper.sock >/dev/null 2>&1; then
    echo "  socket: /var/run/vpntoris-native/helper.sock (up)"
  else
    echo "  socket: helper.sock not visible yet (helper may still be starting)"
  fi
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
