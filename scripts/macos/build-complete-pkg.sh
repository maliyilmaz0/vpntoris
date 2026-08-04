#!/bin/zsh

set -euo pipefail

repo_root=$(cd "$(dirname "$0")/../.." && pwd)
if [[ -f "$repo_root/.env" ]]; then
  set -a
  source "$repo_root/.env"
  set +a
fi
: "${VPNTORIS_MACOS_INSTALLER_IDENTITY:?Set VPNTORIS_MACOS_INSTALLER_IDENTITY in .env}"
version=${VERSION:-1.1.0}
architecture=${ARCH:-universal}
application_package=${1:-"$repo_root/dist/VPNToris-$version-$architecture.pkg"}
engine_package=${2:-"$repo_root/.build/VPNToris-Native-Engine-2.0.0-signed.pkg"}
output_path=${3:-"$repo_root/dist/VPNToris-$version-$architecture-complete.pkg"}
[[ -f "$application_package" ]] || { echo "Application package is missing" >&2; exit 1; }
[[ -f "$engine_package" ]] || { echo "Native engine package is missing" >&2; exit 1; }
rm -f "$output_path"
productbuild --package "$engine_package" --package "$application_package" --sign "$VPNTORIS_MACOS_INSTALLER_IDENTITY" "$output_path"
pkgutil --check-signature "$output_path"
