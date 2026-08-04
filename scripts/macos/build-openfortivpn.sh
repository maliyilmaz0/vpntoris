#!/bin/zsh

set -euo pipefail

version=1.24.1
archive_sha256=c40d33acd97b89c2e943bfd839c19b69e5a7a5997052e2fc9a595602745c0465
output_root=${1:-"$PWD/.build/native-engines/darwin-arm64/openfortivpn"}
work_root=$(mktemp -d /tmp/vpntoris-openfortivpn.XXXXXX)
archive_path="$work_root/openfortivpn.tar.gz"
source_root="$work_root/source"
install_root="$work_root/install"
openssl_root=$(brew --prefix openssl@3)

mkdir -p "$source_root" "$output_root/bin" "$output_root/lib" "$output_root/licenses" "$output_root/sources"
curl -L --fail --silent --show-error "https://github.com/adrienverge/openfortivpn/archive/refs/tags/v${version}.tar.gz" -o "$archive_path"
actual_sha256=$(shasum -a 256 "$archive_path" | awk '{print $1}')
if [[ "$actual_sha256" != "$archive_sha256" ]]; then
  print -u2 "openfortivpn source digest mismatch"
  exit 1
fi
tar -xzf "$archive_path" -C "$source_root" --strip-components=1
cd "$source_root"
./autogen.sh
PKG_CONFIG_PATH="$openssl_root/lib/pkgconfig" CPPFLAGS="-I$openssl_root/include" LDFLAGS="-L$openssl_root/lib" ./configure --enable-legacy-pppd --prefix="$install_root"
make -j"$(sysctl -n hw.logicalcpu)"
make install
cp "$install_root/bin/openfortivpn" "$output_root/bin/openfortivpn"
cp "$openssl_root/lib/libssl.3.dylib" "$output_root/lib/libssl.3.dylib"
cp "$openssl_root/lib/libcrypto.3.dylib" "$output_root/lib/libcrypto.3.dylib"
chmod 0755 "$output_root/bin/openfortivpn" "$output_root/lib/libssl.3.dylib" "$output_root/lib/libcrypto.3.dylib"
install_name_tool -change "$openssl_root/lib/libssl.3.dylib" "@loader_path/../lib/libssl.3.dylib" "$output_root/bin/openfortivpn"
install_name_tool -change "$openssl_root/lib/libcrypto.3.dylib" "@loader_path/../lib/libcrypto.3.dylib" "$output_root/bin/openfortivpn"
install_name_tool -id "@loader_path/libssl.3.dylib" "$output_root/lib/libssl.3.dylib"
install_name_tool -id "@loader_path/libcrypto.3.dylib" "$output_root/lib/libcrypto.3.dylib"
ssl_crypto_path=$(otool -L "$output_root/lib/libssl.3.dylib" | awk '/libcrypto\.3\.dylib/ {print $1; exit}')
install_name_tool -change "$ssl_crypto_path" "@loader_path/libcrypto.3.dylib" "$output_root/lib/libssl.3.dylib"
codesign --force --sign - "$output_root/lib/libcrypto.3.dylib"
codesign --force --sign - "$output_root/lib/libssl.3.dylib"
codesign --force --sign - "$output_root/bin/openfortivpn"
engine_sha256=$(shasum -a 256 "$output_root/bin/openfortivpn" | awk '{print $1}')
ssl_sha256=$(shasum -a 256 "$output_root/lib/libssl.3.dylib" | awk '{print $1}')
crypto_sha256=$(shasum -a 256 "$output_root/lib/libcrypto.3.dylib" | awk '{print $1}')
printf '{\n  "id": "openfortivpn",\n  "protocol": "fortigate-ssl",\n  "version": "%s",\n  "os": "darwin",\n  "architecture": "arm64",\n  "executable": "openfortivpn/bin/openfortivpn",\n  "sha256": "%s",\n  "license": "GPL-3.0-or-later WITH OpenSSL-exception",\n  "capabilities": ["ppp", "otp", "split-route"],\n  "files": {\n    "openfortivpn/lib/libssl.3.dylib": "%s",\n    "openfortivpn/lib/libcrypto.3.dylib": "%s"\n  }\n}\n' "$version" "$engine_sha256" "$ssl_sha256" "$crypto_sha256" > "$output_root/manifest.json"
cp "$source_root/LICENSE" "$output_root/licenses/openfortivpn.txt"
cp "$openssl_root/LICENSE.txt" "$output_root/licenses/openssl.txt"
cp "$archive_path" "$output_root/sources/openfortivpn-${version}.tar.gz"
otool -L "$output_root/bin/openfortivpn"
codesign --verify --strict "$output_root/bin/openfortivpn" "$output_root/lib/libssl.3.dylib" "$output_root/lib/libcrypto.3.dylib"
