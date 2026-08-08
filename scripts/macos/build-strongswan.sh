#!/bin/zsh

set -euo pipefail

repo_root=$(cd "$(dirname "$0")/../.." && pwd)
architecture=${1:-arm64}
version=6.0.7
source_sha256=e518e34e159514f4c6ba80d1f926cb151e0dd4e3a1d94213171234b8b9ae6f55
case "$architecture" in
arm64)
  bottle_tag=arm64_tahoe
  output_arch=arm64
  ;;
amd64)
  bottle_tag=sonoma
  output_arch=x86_64
  ;;
*)
  echo "architecture must be arm64 or amd64" >&2
  exit 1
  ;;
esac
output_root="$repo_root/.build/native-engines/darwin-$architecture/strongswan"
runtime_plugin_path="/var/run/vpntoris-native/plugins"
work_root=$(mktemp -d /tmp/vpntoris-strongswan.XXXXXX)
trap 'rm -rf "$work_root"' EXIT
[[ "$output_root" == "$repo_root/.build/native-engines/"* ]] || exit 1
chmod -R u+w "$output_root" 2>/dev/null || true
rm -rf "$output_root"
mkdir -p "$work_root/cellar" "$work_root/bottles" "$output_root/bin" "$output_root/lib" "$output_root/plugins" "$output_root/licenses" "$output_root/sources"
queue="$work_root/queue"
processed="$work_root/processed"
print -r -- strongswan > "$queue"
: > "$processed"
while [[ -s "$queue" ]]; do
  formula=$(head -n 1 "$queue")
  sed '1d' "$queue" > "$queue.next"
  mv "$queue.next" "$queue"
  grep -qx "$formula" "$processed" && continue
  json="$work_root/$formula.json"
  curl -fsSL --retry 3 "https://formulae.brew.sh/api/formula/$formula.json" -o "$json"
  url=$(jq -r --arg tag "$bottle_tag" '.bottle.stable.files[$tag].url // .bottle.stable.files.all.url // empty' "$json")
  digest=$(jq -r --arg tag "$bottle_tag" '.bottle.stable.files[$tag].sha256 // .bottle.stable.files.all.sha256 // empty' "$json")
  [[ -n "$url" && -n "$digest" ]] || { echo "No $bottle_tag bottle for $formula" >&2; exit 1; }
  scope=$(print -r -- "$url" | sed -E 's#https://ghcr.io/v2/(.*)/blobs/.*#repository:\1:pull#')
  token=$(curl -fsSLG --data-urlencode service=ghcr.io --data-urlencode scope="$scope" https://ghcr.io/token | jq -r .token)
  archive="$work_root/bottles/$formula.tar.gz"
  curl -fsSL --retry 3 -H "Authorization: Bearer $token" "$url" -o "$archive"
  actual=$(shasum -a 256 "$archive" | awk '{print $1}')
  [[ "$actual" == "$digest" ]] || { echo "Bottle digest mismatch for $formula" >&2; exit 1; }
  tar -xf "$archive" -C "$work_root/cellar"
  jq -r '.dependencies[]?' "$json" >> "$queue"
  print -r -- "$formula" >> "$processed"
done
charon=$(find "$work_root/cellar" -type f -path '*/strongswan/*/libexec/ipsec/charon' | head -n 1)
swanctl=$(find "$work_root/cellar" -type f -path '*/strongswan/*/bin/swanctl' | head -n 1)
strong_root=$(dirname "$(dirname "$(dirname "$charon")")")
[[ -n "$charon" && -n "$swanctl" && -d "$strong_root/lib/ipsec/plugins" ]] || { echo "strongSwan payload was not found" >&2; exit 1; }
cp "$charon" "$output_root/bin/charon"
cp "$swanctl" "$output_root/bin/swanctl"
for plugin in "$strong_root/lib/ipsec/plugins/"*.so; do cp "$plugin" "$output_root/plugins/$(basename "$plugin")"; done
for library in "$strong_root/lib/ipsec/"*.dylib; do
  [[ -L "$library" ]] && continue
  cp "$library" "$output_root/lib/$(basename "$library")"
done
chmod -R u+rwX "$output_root"
pending="$work_root/pending"
find "$output_root/bin" "$output_root/lib" "$output_root/plugins" -type f > "$pending"
processed_macho="$work_root/processed-macho"
: > "$processed_macho"
while [[ -s "$pending" ]]; do
  binary=$(head -n 1 "$pending")
  sed '1d' "$pending" > "$pending.next"
  mv "$pending.next" "$pending"
  grep -qxF "$binary" "$processed_macho" && continue
  print -r -- "$binary" >> "$processed_macho"
  otool -L "$binary" | tail -n +2 | awk '{print $1}' | while IFS= read -r dependency; do
    case "$dependency" in
      /usr/lib/*|/System/*|@loader_path/*|@rpath/*) continue ;;
    esac
    basename_value=$(basename "$dependency")
    destination="$output_root/lib/$basename_value"
    if [[ ! -f "$destination" ]]; then
      source_value=$(find "$work_root/cellar" -name "$basename_value" | head -n 1)
      [[ -n "$source_value" ]] || { echo "Missing dependency $dependency" >&2; exit 1; }
      cp "$source_value" "$destination"
      print -r -- "$destination" >> "$pending"
    fi
  done
done
for binary in "$output_root/bin/"* "$output_root/lib/"* "$output_root/plugins/"*; do
  if [[ "$binary" == "$output_root/lib/"* ]]; then
    install_name_tool -id "@loader_path/$(basename "$binary")" "$binary"
    prefix=@loader_path
  elif [[ "$binary" == "$output_root/plugins/"* ]]; then
    prefix=@loader_path/../lib
  else
    prefix=@loader_path/../lib
  fi
  otool -L "$binary" | tail -n +2 | awk '{print $1}' | while IFS= read -r dependency; do
    case "$dependency" in
      /usr/lib/*|/System/*|@loader_path/*|@rpath/*) continue ;;
    esac
    install_name_tool -change "$dependency" "$prefix/$(basename "$dependency")" "$binary"
  done
done
plugin_path=$(strings "$output_root/lib/libstrongswan.0.dylib" | awk '/Cellar\/strongswan\/6\.0\.7\/lib\/ipsec\/plugins/ {print; exit}')
[[ -n "$plugin_path" ]] || { echo "strongSwan plugin path was not found" >&2; exit 1; }
[[ ${#runtime_plugin_path} -le ${#plugin_path} ]] || { echo "runtime plugin path is too long" >&2; exit 1; }
PLUGIN_PATH="$plugin_path" RUNTIME_PLUGIN_PATH="$runtime_plugin_path" perl -0pi -e '$old=$ENV{"PLUGIN_PATH"}; $new=$ENV{"RUNTIME_PLUGIN_PATH"}; $new .= "\0" x (length($old)-length($new)); s/\Q$old\E/$new/g' "$output_root/lib/libstrongswan.0.dylib"
file "$output_root/bin/charon" | grep -q "$output_arch" || { echo "Unexpected strongSwan architecture" >&2; exit 1; }
source_archive="$output_root/sources/strongswan-$version.tar.bz2"
cp /tmp/strongswan-$version.tar.bz2 "$source_archive" 2>/dev/null || curl -fsSL --retry 3 "https://download.strongswan.org/strongswan-$version.tar.bz2" -o "$source_archive"
actual_source=$(shasum -a 256 "$source_archive" | awk '{print $1}')
[[ "$actual_source" == "$source_sha256" ]] || { echo "strongSwan source digest mismatch" >&2; exit 1; }
tar -xOf "$source_archive" "strongswan-$version/COPYING" > "$output_root/licenses/strongswan.txt"

source_build="$work_root/source-build"
mkdir -p "$source_build"
tar -xf "$source_archive" -C "$source_build"
source_tree="$source_build/strongswan-$version"
patch -p1 -d "$source_tree" < "$repo_root/scripts/macos/xauth-generic-otp.patch"
autoreconf_bin=$(command -v autoreconf || true)
[[ -n "$autoreconf_bin" ]] || { echo "autoreconf is required to build the native XAuth plugin" >&2; exit 1; }
(
  cd "$source_tree"
  "$autoreconf_bin" -fi >/dev/null 2>&1
  GPERF=/nonexistent CFLAGS="-arch $output_arch" ./configure --disable-defaults --enable-xauth-generic >/dev/null
  make -C src/libcharon/plugins/xauth_generic -j2 >/dev/null
)
cp "$source_tree/src/libcharon/plugins/xauth_generic/.libs/libstrongswan-xauth-generic.so" \
  "$output_root/plugins/libstrongswan-xauth-generic.so"
engine_sha256=$(shasum -a 256 "$output_root/bin/charon" | awk '{print $1}')
files=$(find "$output_root/bin" "$output_root/lib" "$output_root/plugins" -type f ! -path '*/bin/charon' -print | sort | while IFS= read -r asset; do relative=${asset#"$output_root/"}; printf '%s\t%s\n' "strongswan/$relative" "$(shasum -a 256 "$asset" | awk '{print $1}')"; done | jq -Rn '[inputs | split("\t") | {(.[0]): .[1]}] | add // {}')
jq -n --arg engine "$engine_sha256" --arg architecture "$architecture" --argjson files "$files" '{id:"strongswan",protocol:"ipsec",version:"6.0.7",os:"darwin",architecture:$architecture,executable:"strongswan/bin/charon",sha256:$engine,license:"GPL-2.0-or-later",capabilities:["ikev1","ikev2","xauth","xauth-otp","eap","sha1","dh20","split-route"],files:$files}' > "$output_root/manifest.json"
