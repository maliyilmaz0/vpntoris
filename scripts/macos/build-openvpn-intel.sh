#!/bin/zsh

set -euo pipefail

openvpn_version=2.7.5
openvpn_sha256=c6864b3c7d4e059c7d6ce22d1b5fa646c8b379a06af872eeb9792b6083a44ac4
openssl_version=3.5.7
openssl_sha256=a8c0d28a529ca480f9f36cf5792e2cd21984552a3c8e4aa11a24aa31aeac98e8
lz4_version=1.10.0
lz4_sha256=537512904744b35e232912055ccf8ec66d768639ff3abe5788d90d792ec5f48b
lzo_version=2.10
lzo_sha256=c0f892943208266f9b6543b3ae308fab6284c5c90e627931446fb49b4221a072
output_root=${1:-"$PWD/.build/native-engines/darwin-amd64/openvpn"}
work_root=$(mktemp -d /tmp/vpntoris-openvpn-intel.XXXXXX)
dependency_root="$work_root/dependencies"
compiler="xcrun clang -arch x86_64 -mmacosx-version-min=13.0"

fetch() {
  local url=$1
  local destination=$2
  local expected=$3
  curl -L --fail --silent --show-error "$url" -o "$destination"
  local actual=$(shasum -a 256 "$destination" | awk '{print $1}')
  if [[ "$actual" != "$expected" ]]; then
    print -u2 "source digest mismatch: $destination"
    exit 1
  fi
}

mkdir -p "$dependency_root" "$output_root/bin" "$output_root/licenses" "$output_root/sources"
fetch "https://github.com/openssl/openssl/releases/download/openssl-${openssl_version}/openssl-${openssl_version}.tar.gz" "$work_root/openssl.tar.gz" "$openssl_sha256"
fetch "https://github.com/lz4/lz4/archive/refs/tags/v${lz4_version}.tar.gz" "$work_root/lz4.tar.gz" "$lz4_sha256"
fetch "https://www.oberhumer.com/opensource/lzo/download/lzo-${lzo_version}.tar.gz" "$work_root/lzo.tar.gz" "$lzo_sha256"
fetch "https://swupdate.openvpn.org/community/releases/openvpn-${openvpn_version}.tar.gz" "$work_root/openvpn.tar.gz" "$openvpn_sha256"

for source in openssl lz4 lzo openvpn; do mkdir -p "$work_root/$source"; tar -xzf "$work_root/$source.tar.gz" -C "$work_root/$source" --strip-components=1; done

cd "$work_root/openssl"
./Configure darwin64-x86_64-cc no-shared no-tests no-apps --prefix="$dependency_root/openssl" --openssldir="$dependency_root/openssl/ssl" -mmacosx-version-min=13.0
make -j"$(sysctl -n hw.logicalcpu)"
make install_sw

cd "$work_root/lzo"
CC="$compiler" ./configure --host=x86_64-apple-darwin --prefix="$dependency_root/lzo" --disable-shared --enable-static
make -j"$(sysctl -n hw.logicalcpu)"
make install

cd "$work_root/lz4"
make -C lib -j"$(sysctl -n hw.logicalcpu)" CC="$compiler" BUILD_SHARED=no
mkdir -p "$dependency_root/lz4/include" "$dependency_root/lz4/lib"
cp lib/lz4.h lib/lz4frame.h lib/lz4frame_static.h lib/lz4hc.h "$dependency_root/lz4/include/"
cp lib/liblz4.a "$dependency_root/lz4/lib/"

cd "$work_root/openvpn"
CC="$compiler" CPPFLAGS="-I$dependency_root/openssl/include -I$dependency_root/lzo/include -I$dependency_root/lz4/include" LDFLAGS="-L$dependency_root/openssl/lib -L$dependency_root/lzo/lib -L$dependency_root/lz4/lib" OPENSSL_CFLAGS="-I$dependency_root/openssl/include" OPENSSL_LIBS="$dependency_root/openssl/lib/libssl.a $dependency_root/openssl/lib/libcrypto.a" LZO_CFLAGS="-I$dependency_root/lzo/include" LZO_LIBS="$dependency_root/lzo/lib/liblzo2.a" LZ4_CFLAGS="-I$dependency_root/lz4/include" LZ4_LIBS="$dependency_root/lz4/lib/liblz4.a" ./configure --host=x86_64-apple-darwin --prefix="$work_root/install" --disable-debug --disable-dependency-tracking --disable-plugins --disable-shared --enable-static
make -j"$(sysctl -n hw.logicalcpu)"
make install
cp "$work_root/install/sbin/openvpn" "$output_root/bin/openvpn"
chmod 0755 "$output_root/bin/openvpn"
codesign --force --sign - "$output_root/bin/openvpn"
engine_sha256=$(shasum -a 256 "$output_root/bin/openvpn" | awk '{print $1}')
printf '{\n  "id": "openvpn",\n  "protocol": "openvpn",\n  "version": "%s",\n  "os": "darwin",\n  "architecture": "amd64",\n  "executable": "openvpn/bin/openvpn",\n  "sha256": "%s",\n  "license": "GPL-2.0-only WITH OpenSSL-exception",\n  "capabilities": ["tun", "userpass", "challenge", "split-route"]\n}\n' "$openvpn_version" "$engine_sha256" > "$output_root/manifest.json"
cp "$work_root/openvpn/COPYING" "$output_root/licenses/openvpn.txt"
cp "$work_root/openssl/LICENSE.txt" "$output_root/licenses/openssl.txt"
cp "$work_root/lz4/LICENSE" "$output_root/licenses/lz4.txt"
cp "$work_root/lzo/COPYING" "$output_root/licenses/lzo.txt"
cp "$work_root/openvpn.tar.gz" "$output_root/sources/openvpn-${openvpn_version}.tar.gz"
cp "$work_root/openssl.tar.gz" "$output_root/sources/openssl-${openssl_version}.tar.gz"
cp "$work_root/lz4.tar.gz" "$output_root/sources/lz4-${lz4_version}.tar.gz"
cp "$work_root/lzo.tar.gz" "$output_root/sources/lzo-${lzo_version}.tar.gz"
/usr/bin/file "$output_root/bin/openvpn"
otool -L "$output_root/bin/openvpn"
codesign --verify --strict "$output_root/bin/openvpn"
