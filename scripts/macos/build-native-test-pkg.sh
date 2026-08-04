#!/bin/zsh

set -euo pipefail
export COPYFILE_DISABLE=1

repo_root=$(cd "$(dirname "$0")/../.." && pwd)
if [[ -f "$repo_root/.env" ]]; then
  set -a
  source "$repo_root/.env"
  set +a
fi
: "${VPNTORIS_MACOS_APPLICATION_IDENTITY:?Set VPNTORIS_MACOS_APPLICATION_IDENTITY in .env}"
: "${VPNTORIS_MACOS_INSTALLER_IDENTITY:?Set VPNTORIS_MACOS_INSTALLER_IDENTITY in .env}"
stage_root=$(mktemp -d /tmp/vpntoris-native-pkg-root.XXXXXX)
scripts_root=$(mktemp -d /tmp/vpntoris-native-pkg-scripts.XXXXXX)
output_path=${1:-"$repo_root/.build/VPNToris-Native-Engine-2.0.0-test.pkg"}
engine_root="$repo_root/.build/native-engines/darwin-arm64/openfortivpn"
openvpn_root="$repo_root/.build/native-engines/darwin-arm64/openvpn"
openvpn_intel_root="$repo_root/.build/native-engines/darwin-amd64/openvpn"
openconnect_root="$repo_root/.build/native-engines/darwin-arm64/openconnect"
openconnect_intel_root="$repo_root/.build/native-engines/darwin-amd64/openconnect"

if [[ ! -x "$engine_root/bin/openfortivpn" ]]; then
  "$repo_root/scripts/macos/build-openfortivpn.sh"
fi
if [[ ! -x "$openvpn_root/bin/openvpn" ]]; then
  "$repo_root/scripts/macos/build-openvpn.sh"
fi
if [[ ! -x "$openvpn_intel_root/bin/openvpn" ]]; then
  "$repo_root/scripts/macos/build-openvpn-intel.sh"
fi
if [[ ! -x "$openconnect_root/bin/openconnect" ]]; then
  "$repo_root/scripts/macos/build-openconnect.sh" arm64
fi
if [[ ! -x "$openconnect_intel_root/bin/openconnect" ]]; then
  "$repo_root/scripts/macos/build-openconnect.sh" amd64
fi
mkdir -p "$stage_root/Library/PrivilegedHelperTools"
mkdir -p "$stage_root/Library/LaunchDaemons"
mkdir -p "$stage_root/Library/Application Support/VPNToris/Engines/darwin-arm64"
mkdir -p "$stage_root/Library/Application Support/VPNToris/Engines/darwin-amd64"
mkdir -p "$scripts_root"
cd "$repo_root/vpntoris-tray"
GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o "$scripts_root/helper-arm64" ./cmd/vpntoris-native-helper
GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "$scripts_root/helper-amd64" ./cmd/vpntoris-native-helper
lipo -create "$scripts_root/helper-arm64" "$scripts_root/helper-amd64" -output "$stage_root/Library/PrivilegedHelperTools/com.vpntoris.native-helper"
cp -R "$engine_root" "$stage_root/Library/Application Support/VPNToris/Engines/darwin-arm64/openfortivpn"
cp -R "$openvpn_root" "$stage_root/Library/Application Support/VPNToris/Engines/darwin-arm64/openvpn"
cp -R "$openvpn_intel_root" "$stage_root/Library/Application Support/VPNToris/Engines/darwin-amd64/openvpn"
cp -R "$openconnect_root" "$stage_root/Library/Application Support/VPNToris/Engines/darwin-arm64/openconnect"
cp -R "$openconnect_intel_root" "$stage_root/Library/Application Support/VPNToris/Engines/darwin-amd64/openconnect"
cp "$repo_root/scripts/macos/native-helper.plist" "$stage_root/Library/LaunchDaemons/com.vpntoris.native-helper.plist"
cp "$repo_root/scripts/macos/native-postinstall" "$scripts_root/postinstall"
chmod 0755 "$stage_root/Library/PrivilegedHelperTools/com.vpntoris.native-helper" "$scripts_root/postinstall"
chmod 0644 "$stage_root/Library/LaunchDaemons/com.vpntoris.native-helper.plist"
codesign --force --options runtime --timestamp --sign "$VPNTORIS_MACOS_APPLICATION_IDENTITY" "$stage_root/Library/Application Support/VPNToris/Engines/darwin-arm64/openfortivpn/lib/libcrypto.3.dylib"
codesign --force --options runtime --timestamp --sign "$VPNTORIS_MACOS_APPLICATION_IDENTITY" "$stage_root/Library/Application Support/VPNToris/Engines/darwin-arm64/openfortivpn/lib/libssl.3.dylib"
codesign --force --options runtime --timestamp --sign "$VPNTORIS_MACOS_APPLICATION_IDENTITY" "$stage_root/Library/Application Support/VPNToris/Engines/darwin-arm64/openfortivpn/bin/openfortivpn"
codesign --force --options runtime --timestamp --sign "$VPNTORIS_MACOS_APPLICATION_IDENTITY" "$stage_root/Library/PrivilegedHelperTools/com.vpntoris.native-helper"
packaged_openvpn="$stage_root/Library/Application Support/VPNToris/Engines/darwin-arm64/openvpn"
for library in "$packaged_openvpn/lib/"*.dylib; do codesign --force --options runtime --timestamp --sign "$VPNTORIS_MACOS_APPLICATION_IDENTITY" "$library"; done
codesign --force --options runtime --timestamp --sign "$VPNTORIS_MACOS_APPLICATION_IDENTITY" "$packaged_openvpn/bin/openvpn"
openvpn_sha256=$(shasum -a 256 "$packaged_openvpn/bin/openvpn" | awk '{print $1}')
openvpn_ssl_sha256=$(shasum -a 256 "$packaged_openvpn/lib/libssl.3.dylib" | awk '{print $1}')
openvpn_crypto_sha256=$(shasum -a 256 "$packaged_openvpn/lib/libcrypto.3.dylib" | awk '{print $1}')
openvpn_lz4_sha256=$(shasum -a 256 "$packaged_openvpn/lib/liblz4.1.dylib" | awk '{print $1}')
openvpn_lzo_sha256=$(shasum -a 256 "$packaged_openvpn/lib/liblzo2.2.dylib" | awk '{print $1}')
jq -n --arg engine "$openvpn_sha256" --arg ssl "$openvpn_ssl_sha256" --arg crypto "$openvpn_crypto_sha256" --arg lz4 "$openvpn_lz4_sha256" --arg lzo "$openvpn_lzo_sha256" '{id:"openvpn",protocol:"openvpn",version:"2.7.5",os:"darwin",architecture:"arm64",executable:"openvpn/bin/openvpn",sha256:$engine,license:"GPL-2.0-only WITH OpenSSL-exception",capabilities:["tun","userpass","challenge","split-route"],files:{"openvpn/lib/libssl.3.dylib":$ssl,"openvpn/lib/libcrypto.3.dylib":$crypto,"openvpn/lib/liblz4.1.dylib":$lz4,"openvpn/lib/liblzo2.2.dylib":$lzo}}' > "$packaged_openvpn/manifest.json"
packaged_openvpn_intel="$stage_root/Library/Application Support/VPNToris/Engines/darwin-amd64/openvpn"
codesign --force --options runtime --timestamp --sign "$VPNTORIS_MACOS_APPLICATION_IDENTITY" "$packaged_openvpn_intel/bin/openvpn"
openvpn_intel_sha256=$(shasum -a 256 "$packaged_openvpn_intel/bin/openvpn" | awk '{print $1}')
jq -n --arg engine "$openvpn_intel_sha256" '{id:"openvpn",protocol:"openvpn",version:"2.7.5",os:"darwin",architecture:"amd64",executable:"openvpn/bin/openvpn",sha256:$engine,license:"GPL-2.0-only WITH OpenSSL-exception",capabilities:["tun","userpass","challenge","split-route"]}' > "$packaged_openvpn_intel/manifest.json"
for packaged_openconnect in "$stage_root/Library/Application Support/VPNToris/Engines/darwin-arm64/openconnect" "$stage_root/Library/Application Support/VPNToris/Engines/darwin-amd64/openconnect"; do
  for library in "$packaged_openconnect/lib/"*.dylib; do codesign --force --options runtime --timestamp --sign "$VPNTORIS_MACOS_APPLICATION_IDENTITY" "$library"; done
  codesign --force --options runtime --timestamp --sign "$VPNTORIS_MACOS_APPLICATION_IDENTITY" "$packaged_openconnect/bin/openconnect"
  architecture=$(basename "$(dirname "$packaged_openconnect")" | sed 's/darwin-//')
  engine_sha256=$(shasum -a 256 "$packaged_openconnect/bin/openconnect" | awk '{print $1}')
  files=$(for library in "$packaged_openconnect/lib/"*.dylib; do printf '%s\t%s\n' "openconnect/lib/$(basename "$library")" "$(shasum -a 256 "$library" | awk '{print $1}')"; done | jq -Rn '[inputs | split("\t") | {(.[0]): .[1]}] | add')
  jq -n --arg engine "$engine_sha256" --arg architecture "$architecture" --argjson files "$files" '{id:"openconnect",protocol:"openconnect",version:"9.21",os:"darwin",architecture:$architecture,executable:"openconnect/bin/openconnect",sha256:$engine,license:"LGPL-2.1-or-later",capabilities:["anyconnect","gp","pulse","nc","f5","fortinet","array","otp","split-route"],files:$files}' > "$packaged_openconnect/manifest.json"
done
packaged_engine="$stage_root/Library/Application Support/VPNToris/Engines/darwin-arm64/openfortivpn"
engine_sha256=$(shasum -a 256 "$packaged_engine/bin/openfortivpn" | awk '{print $1}')
ssl_sha256=$(shasum -a 256 "$packaged_engine/lib/libssl.3.dylib" | awk '{print $1}')
crypto_sha256=$(shasum -a 256 "$packaged_engine/lib/libcrypto.3.dylib" | awk '{print $1}')
jq -n --arg engine "$engine_sha256" --arg ssl "$ssl_sha256" --arg crypto "$crypto_sha256" '{id:"openfortivpn",protocol:"fortigate-ssl",version:"1.24.1",os:"darwin",architecture:"arm64",executable:"openfortivpn/bin/openfortivpn",sha256:$engine,license:"GPL-3.0-or-later WITH OpenSSL-exception",capabilities:["ppp","otp","split-route"],files:{"openfortivpn/lib/libssl.3.dylib":$ssl,"openfortivpn/lib/libcrypto.3.dylib":$crypto}}' > "$packaged_engine/manifest.json"
xattr -cr "$stage_root"
/usr/sbin/dot_clean -m "$stage_root"
rm -f "$output_path"
pkgbuild --root "$stage_root" --scripts "$scripts_root" --identifier com.vpntoris.native-engine --version 2.0.0 --install-location / --sign "$VPNTORIS_MACOS_INSTALLER_IDENTITY" "$output_path"
pkgutil --check-signature "$output_path" || true
