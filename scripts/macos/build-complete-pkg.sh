#!/bin/zsh
# Merge intermediate app PKG + engine PKG into the ONLY shippable macOS product:
#   VPNToris-<ver>-universal-complete.pkg
#
# App-only universal.pkg is never a release artifact — it is only an input here.
# Set VPNTORIS_MACOS_UNSIGNED=1 (or pass --unsigned) for local/dev builds without
# Developer ID Installer signing / notarization.
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/../.." && pwd)
if [[ -f "$repo_root/.env" ]]; then
  set -a
  source "$repo_root/.env"
  set +a
fi

UNSIGNED=${VPNTORIS_MACOS_UNSIGNED:-0}
for arg in "$@"; do
  case "$arg" in
    --unsigned) UNSIGNED=1 ;;
  esac
done

# Strip optional flag from positional args
positional=()
for arg in "$@"; do
  case "$arg" in
    --unsigned) ;;
    *) positional+=("$arg") ;;
  esac
done
set -- "${positional[@]+"${positional[@]}"}"

if [[ $UNSIGNED != 1 && $UNSIGNED != true ]]; then
  : "${VPNTORIS_MACOS_INSTALLER_IDENTITY:?Set VPNTORIS_MACOS_INSTALLER_IDENTITY in .env (or use unsigned/dev build)}"
fi

version=${VERSION:-1.2.0}
architecture=${ARCH:-universal}
application_package=${1:-"$repo_root/dist/VPNToris-$version-$architecture.pkg"}
engine_package=${2:-"$repo_root/.build/VPNToris-Native-Engine-2.0.0-signed.pkg"}
output_path=${3:-"$repo_root/dist/VPNToris-$version-$architecture-complete.pkg"}
[[ -f "$application_package" ]] || { echo "Application package is missing (intermediate): $application_package" >&2; exit 1; }
[[ -f "$engine_package" ]] || {
  echo "Native engine package is missing (required product layer): $engine_package" >&2
  echo "complete PKG cannot be built without engines" >&2
  exit 1
}
rm -f "$output_path"
echo "Building complete PKG (app + engines) → $output_path"
if [[ $UNSIGNED == 1 || $UNSIGNED == true ]]; then
  productbuild --package "$engine_package" --package "$application_package" "$output_path"
  echo "Complete PKG ready (UNSIGNED / dev — do not distribute)"
else
  productbuild --package "$engine_package" --package "$application_package" --sign "$VPNTORIS_MACOS_INSTALLER_IDENTITY" "$output_path"
  pkgutil --check-signature "$output_path"
  echo "Complete PKG ready (ship this only; do not distribute app-only .pkg)"
fi
