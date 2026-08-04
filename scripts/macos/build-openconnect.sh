#!/bin/zsh

set -euo pipefail

repo_root=$(cd "$(dirname "$0")/../.." && pwd)
architecture=${1:-arm64}
version=9.21
source_sha256=ef0c875f3f8d8cc00e9647f36f87f2dd7d4ccad02c47c82f2dc5ba6b37edab06
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
output_root="$repo_root/.build/native-engines/darwin-$architecture/openconnect"
work_root=$(mktemp -d /tmp/vpntoris-openconnect.XXXXXX)
trap 'rm -rf "$work_root"' EXIT
[[ "$output_root" == "$repo_root/.build/native-engines/"* ]] || exit 1
chmod -R u+w "$output_root" 2>/dev/null || true
rm -rf "$output_root"
mkdir -p "$work_root/cellar" "$work_root/bottles" "$output_root/bin" "$output_root/lib" "$output_root/licenses" "$output_root/sources"
queue="$work_root/queue"
processed="$work_root/processed"
print -r -- openconnect > "$queue"
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
  scope=$(print -r -- "$url" | sed -E 's#https://ghcr.io/v2/([^/]+/[^/]+/[^/]+)/blobs/.*#repository:\1:pull#')
  token=$(curl -fsSLG --data-urlencode service=ghcr.io --data-urlencode scope="$scope" https://ghcr.io/token | jq -r .token)
  archive="$work_root/bottles/$formula.tar.gz"
  curl -fsSL --retry 3 -H "Authorization: Bearer $token" "$url" -o "$archive"
  actual=$(shasum -a 256 "$archive" | awk '{print $1}')
  [[ "$actual" == "$digest" ]] || { echo "Bottle digest mismatch for $formula" >&2; exit 1; }
  tar -xf "$archive" -C "$work_root/cellar"
  jq -r '.dependencies[]?' "$json" >> "$queue"
  print -r -- "$formula" >> "$processed"
done
executable=$(find "$work_root/cellar" -type f -path '*/openconnect/*/bin/openconnect' -perm +111 | head -n 1)
[[ -n "$executable" ]] || { echo "OpenConnect executable was not found" >&2; exit 1; }
cp "$executable" "$output_root/bin/openconnect"
pending="$work_root/pending"
print -r -- "$output_root/bin/openconnect" > "$pending"
while [[ -s "$pending" ]]; do
  binary=$(head -n 1 "$pending")
  sed '1d' "$pending" > "$pending.next"
  mv "$pending.next" "$pending"
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
for binary in "$output_root/bin/openconnect" "$output_root/lib/"*.dylib; do
  [[ "$binary" == "$output_root/lib/"*.dylib ]] && [[ ! -e "$binary" ]] && continue
  if [[ "$binary" == "$output_root/lib/"* ]]; then
    install_name_tool -id "@loader_path/$(basename "$binary")" "$binary"
    prefix=@loader_path
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
file "$output_root/bin/openconnect" | grep -q "$output_arch" || { echo "Unexpected OpenConnect architecture" >&2; exit 1; }
source_archive="$output_root/sources/openconnect-$version.tar.gz"
curl -fsSL --retry 3 "https://gitlab.com/openconnect/openconnect/-/archive/v$version/openconnect-v$version.tar.gz" -o "$source_archive"
actual_source=$(shasum -a 256 "$source_archive" | awk '{print $1}')
[[ "$actual_source" == "$source_sha256" ]] || { echo "OpenConnect source digest mismatch" >&2; exit 1; }
tar -xOf "$source_archive" "openconnect-v$version/COPYING.LGPL" > "$output_root/licenses/openconnect.txt"
engine_sha256=$(shasum -a 256 "$output_root/bin/openconnect" | awk '{print $1}')
files=$(for library in "$output_root/lib/"*.dylib; do [[ -f "$library" ]] && printf '%s\t%s\n' "openconnect/lib/$(basename "$library")" "$(shasum -a 256 "$library" | awk '{print $1}')"; done | jq -Rn '[inputs | split("\t") | {(.[0]): .[1]}] | add // {}')
jq -n --arg engine "$engine_sha256" --arg architecture "$architecture" --argjson files "$files" '{id:"openconnect",protocol:"openconnect",version:"9.21",os:"darwin",architecture:$architecture,executable:"openconnect/bin/openconnect",sha256:$engine,license:"LGPL-2.1-or-later",capabilities:["anyconnect","gp","pulse","nc","f5","fortinet","array","otp","split-route"],files:$files}' > "$output_root/manifest.json"
