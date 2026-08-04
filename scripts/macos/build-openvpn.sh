#!/bin/zsh

set -euo pipefail

version=2.7.5
archive_sha256=c6864b3c7d4e059c7d6ce22d1b5fa646c8b379a06af872eeb9792b6083a44ac4
output_root=${1:-"$PWD/.build/native-engines/darwin-arm64/openvpn"}
work_root=$(mktemp -d /tmp/vpntoris-openvpn.XXXXXX)
archive_path="$work_root/openvpn.tar.gz"
source_root="$work_root/source"
install_root="$work_root/install"
openssl_root=$(brew --prefix openssl@3)
lz4_root=$(brew --prefix lz4)
lzo_root=$(brew --prefix lzo)
pkcs11_root=$(brew --prefix pkcs11-helper)
pkg_config_path="$openssl_root/lib/pkgconfig:$lz4_root/lib/pkgconfig:$lzo_root/lib/pkgconfig:$pkcs11_root/lib/pkgconfig"

mkdir -p "$source_root" "$output_root/bin" "$output_root/lib" "$output_root/licenses" "$output_root/sources"
curl -L --fail --silent --show-error "https://swupdate.openvpn.org/community/releases/openvpn-${version}.tar.gz" -o "$archive_path"
actual_sha256=$(shasum -a 256 "$archive_path" | awk '{print $1}')
if [[ "$actual_sha256" != "$archive_sha256" ]]; then
  print -u2 "OpenVPN source digest mismatch"
  exit 1
fi
tar -xzf "$archive_path" -C "$source_root" --strip-components=1
cd "$source_root"
PKG_CONFIG_PATH="$pkg_config_path" ./configure --prefix="$install_root" --disable-debug --disable-dependency-tracking --disable-plugins
make -j"$(sysctl -n hw.logicalcpu)"
make install
cp "$install_root/sbin/openvpn" "$output_root/bin/openvpn"
cp "$openssl_root/lib/libssl.3.dylib" "$output_root/lib/libssl.3.dylib"
cp "$openssl_root/lib/libcrypto.3.dylib" "$output_root/lib/libcrypto.3.dylib"
cp "$lz4_root/lib/liblz4.1.dylib" "$output_root/lib/liblz4.1.dylib"
cp "$lzo_root/lib/liblzo2.2.dylib" "$output_root/lib/liblzo2.2.dylib"
chmod 0755 "$output_root/bin/openvpn" "$output_root/lib/"*.dylib
for library in libssl.3.dylib libcrypto.3.dylib liblz4.1.dylib liblzo2.2.dylib; do
  original=$(otool -L "$output_root/bin/openvpn" | awk -v name="$library" '$1 ~ name {print $1; exit}')
  install_name_tool -change "$original" "@loader_path/../lib/$library" "$output_root/bin/openvpn"
  install_name_tool -id "@loader_path/$library" "$output_root/lib/$library"
done
ssl_crypto_path=$(otool -L "$output_root/lib/libssl.3.dylib" | awk '/libcrypto\.3\.dylib/ {print $1; exit}')
install_name_tool -change "$ssl_crypto_path" "@loader_path/libcrypto.3.dylib" "$output_root/lib/libssl.3.dylib"
for library in "$output_root/lib/"*.dylib; do codesign --force --sign - "$library"; done
codesign --force --sign - "$output_root/bin/openvpn"
engine_sha256=$(shasum -a 256 "$output_root/bin/openvpn" | awk '{print $1}')
ssl_sha256=$(shasum -a 256 "$output_root/lib/libssl.3.dylib" | awk '{print $1}')
crypto_sha256=$(shasum -a 256 "$output_root/lib/libcrypto.3.dylib" | awk '{print $1}')
lz4_sha256=$(shasum -a 256 "$output_root/lib/liblz4.1.dylib" | awk '{print $1}')
lzo_sha256=$(shasum -a 256 "$output_root/lib/liblzo2.2.dylib" | awk '{print $1}')
printf '{\n  "id": "openvpn",\n  "protocol": "openvpn",\n  "version": "%s",\n  "os": "darwin",\n  "architecture": "arm64",\n  "executable": "openvpn/bin/openvpn",\n  "sha256": "%s",\n  "license": "GPL-2.0-only WITH OpenSSL-exception",\n  "capabilities": ["tun", "userpass", "challenge", "split-route"],\n  "files": {\n    "openvpn/lib/libssl.3.dylib": "%s",\n    "openvpn/lib/libcrypto.3.dylib": "%s",\n    "openvpn/lib/liblz4.1.dylib": "%s",\n    "openvpn/lib/liblzo2.2.dylib": "%s"\n  }\n}\n' "$version" "$engine_sha256" "$ssl_sha256" "$crypto_sha256" "$lz4_sha256" "$lzo_sha256" > "$output_root/manifest.json"
cp "$source_root/COPYING" "$output_root/licenses/openvpn.txt"
cp "$openssl_root/LICENSE.txt" "$output_root/licenses/openssl.txt"
cp "$lz4_root/LICENSE" "$output_root/licenses/lz4.txt"
cp "$lzo_root/share/doc/lzo/COPYING" "$output_root/licenses/lzo.txt"
cp "$archive_path" "$output_root/sources/openvpn-${version}.tar.gz"
otool -L "$output_root/bin/openvpn"
codesign --verify --strict "$output_root/bin/openvpn" "$output_root/lib/"*.dylib
