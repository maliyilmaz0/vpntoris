#!/bin/bash
# Stage Windows native engines under .build/native-engines/windows-amd64/
# for the complete MSI (same role as scripts/macos/build-*-engine.sh + package).
#
# Run on the maintainer macOS host (downloads official redistributables).
# Engines are never resolved from PATH at runtime; each package has manifest.json.
#
# Layout:
#   .build/native-engines/windows-amd64/
#     openvpn/{bin/openvpn.exe,lib/*.dll,manifest.json,licenses/}
#     openconnect/{bin/openconnect.exe,helpers,manifest.json}  (best-effort)
#
# Usage:
#   ./scripts/windows/build-engines.sh
#   SKIP_OPENCONNECT=1 ./scripts/windows/build-engines.sh
#
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
CACHE_DIR="$ROOT_DIR/.build/cache/windows-engines"
ENGINE_ROOT="$ROOT_DIR/.build/native-engines/windows-amd64"
SKIP_OPENCONNECT=${SKIP_OPENCONNECT:-0}

# OpenVPN Community Windows client (ships openvpn.exe + OpenSSL/LZO + often wintun)
OPENVPN_VERSION=${OPENVPN_VERSION:-2.7.5}
OPENVPN_MSI_NAME=${OPENVPN_MSI_NAME:-OpenVPN-${OPENVPN_VERSION}-I001-amd64.msi}
OPENVPN_MSI_URL=${OPENVPN_MSI_URL:-https://swupdate.openvpn.org/community/releases/${OPENVPN_MSI_NAME}}

# Official signed Wintun (required for --dev tun on modern OpenVPN)
WINTUN_VERSION=${WINTUN_VERSION:-0.14.1}
WINTUN_URL=${WINTUN_URL:-https://www.wintun.net/builds/wintun-${WINTUN_VERSION}.zip}
WINTUN_SHA256=${WINTUN_SHA256:-07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51}

# MSYS2 mingw64 openconnect package (best-effort; package revision may lag).
# See https://packages.msys2.org/package/mingw-w64-x86_64-openconnect
OPENCONNECT_MSYS_URL=${OPENCONNECT_MSYS_URL:-}
OPENCONNECT_VERSION_LABEL=${OPENCONNECT_VERSION_LABEL:-9.12}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "error: missing prerequisite: $1" >&2
    exit 1
  }
}

require_cmd curl
require_cmd unzip
require_cmd shasum
require_cmd python3
require_cmd msiextract

mkdir -p "$CACHE_DIR" "$ENGINE_ROOT"
rm -rf "${ENGINE_ROOT:?}/openvpn" "${ENGINE_ROOT:?}/openconnect"
mkdir -p "$ENGINE_ROOT/openvpn/bin" "$ENGINE_ROOT/openvpn/lib" "$ENGINE_ROOT/openvpn/licenses"
mkdir -p "$ENGINE_ROOT/openconnect/bin" "$ENGINE_ROOT/openconnect/lib" "$ENGINE_ROOT/openconnect/licenses"

download() {
  local url=$1
  local dest=$2
  if [[ -f $dest && -s $dest ]]; then
    echo "[cache] $(basename "$dest")"
    return 0
  fi
  echo "[download] $url"
  curl -fL --retry 3 --retry-delay 2 -o "$dest.partial" "$url"
  mv "$dest.partial" "$dest"
}

sha256_file() {
  shasum -a 256 "$1" | awk '{print $1}'
}

write_manifest() {
  # Args: out_dir id protocol version executable_relpath [files.json path]
  # executable is relative to engine root (windows-amd64), e.g. openvpn/bin/openvpn.exe
  local out_dir=$1 id=$2 protocol=$3 version=$4 executable=$5
  local files_path=${6:-}
  local abs="$ENGINE_ROOT/$executable"
  if [[ ! -f $abs ]]; then
    echo "error: missing engine executable for manifest: $abs" >&2
    exit 1
  fi
  local sha
  sha=$(sha256_file "$abs")
  python3 - "$out_dir/manifest.json" "$id" "$protocol" "$version" "$executable" "$sha" "$files_path" <<'PY'
import json, sys
from pathlib import Path
out, engine_id, protocol, version, executable, sha, files_path = sys.argv[1:8]
files = {}
if files_path:
    files = json.loads(Path(files_path).read_text(encoding="utf-8"))
manifest = {
    "id": engine_id,
    "protocol": protocol,
    "version": version,
    "os": "windows",
    "architecture": "amd64",
    "executable": executable,
    "sha256": sha,
    "license": {
        "openvpn": "GPL-2.0-only WITH OpenSSL-exception",
        "openconnect": "LGPL-2.1-or-later",
    }.get(engine_id, "unknown"),
    "capabilities": {
        "openvpn": ["tun", "userpass", "challenge", "split-route", "wintun"],
        "openconnect": ["anyconnect", "gp", "pulse", "nc", "otp", "split-route"],
    }.get(engine_id, []),
    "files": files,
}
with open(out, "w", encoding="utf-8") as fh:
    json.dump(manifest, fh, indent=2)
    fh.write("\n")
print(f"wrote {out}")
PY
}

# ─── OpenVPN + Wintun ─────────────────────────────────────────────
stage_openvpn() {
  local msi="$CACHE_DIR/$OPENVPN_MSI_NAME"
  local extract="$CACHE_DIR/openvpn-msi-extract"
  download "$OPENVPN_MSI_URL" "$msi"
  rm -rf "$extract"
  mkdir -p "$extract"
  echo "[openvpn] extracting $OPENVPN_MSI_NAME ..."
  msiextract -C "$extract" "$msi" >/dev/null

  # Community MSI layout varies slightly by release; search for openvpn.exe.
  local found
  found=$(find "$extract" -type f -iname 'openvpn.exe' | head -1 || true)
  if [[ -z $found ]]; then
    echo "error: openvpn.exe not found inside $OPENVPN_MSI_NAME" >&2
    find "$extract" -type f | head -50 >&2 || true
    exit 1
  fi
  local bin_dir
  bin_dir=$(dirname "$found")
  echo "[openvpn] using binary tree: $bin_dir"
  cp -f "$found" "$ENGINE_ROOT/openvpn/bin/openvpn.exe"
  # Companion DLLs (OpenSSL, LZO, LZ4, …) live next to openvpn.exe in the MSI.
  find "$bin_dir" -maxdepth 1 -type f \( -iname '*.dll' -o -iname '*.exe' \) ! -iname 'openvpn.exe' | while read -r f; do
    base=$(basename "$f")
    case "$base" in
      openvpn-gui.exe|openvpnserv*.exe|tapctl.exe|ovpnhelper*.exe) continue ;;
    esac
    cp -f "$f" "$ENGINE_ROOT/openvpn/lib/$base"
  done
  # Also copy license/readme if present
  find "$extract" -type f \( -iname '*license*' -o -iname 'COPYING*' -o -iname 'README*' \) | head -20 | while read -r f; do
    cp -f "$f" "$ENGINE_ROOT/openvpn/licenses/$(basename "$f")" 2>/dev/null || true
  done

  # Wintun: prefer MSI-bundled copy, else official zip (amd64).
  local wintun_src=""
  wintun_src=$(find "$extract" -type f -iname 'wintun.dll' | head -1 || true)
  if [[ -z $wintun_src ]]; then
    local wzip="$CACHE_DIR/wintun-${WINTUN_VERSION}.zip"
    download "$WINTUN_URL" "$wzip"
    local got
    got=$(sha256_file "$wzip")
    if [[ $got != "$WINTUN_SHA256" ]]; then
      echo "error: wintun zip sha256 mismatch" >&2
      echo "  expected: $WINTUN_SHA256" >&2
      echo "  got:      $got" >&2
      exit 1
    fi
    local wextract="$CACHE_DIR/wintun-extract"
    rm -rf "$wextract"
    mkdir -p "$wextract"
    unzip -q -o "$wzip" -d "$wextract"
    wintun_src=$(find "$wextract" -type f -path '*/amd64/wintun.dll' | head -1 || true)
  fi
  if [[ -z $wintun_src || ! -f $wintun_src ]]; then
    echo "error: wintun.dll not found" >&2
    exit 1
  fi
  # OpenVPN loads wintun.dll from its directory; also keep a copy under lib/ for manifest.
  cp -f "$wintun_src" "$ENGINE_ROOT/openvpn/bin/wintun.dll"
  cp -f "$wintun_src" "$ENGINE_ROOT/openvpn/lib/wintun.dll"

  # PATH for the supervised process is System32-only; ensure DLL search works by
  # placing runtime DLLs next to openvpn.exe as well.
  if [[ -d $ENGINE_ROOT/openvpn/lib ]]; then
    find "$ENGINE_ROOT/openvpn/lib" -maxdepth 1 -type f -iname '*.dll' ! -iname 'wintun.dll' | while read -r dll; do
      cp -f "$dll" "$ENGINE_ROOT/openvpn/bin/$(basename "$dll")"
    done
  fi

  local files_path="$ENGINE_ROOT/openvpn/.files.json"
  python3 - "$ENGINE_ROOT/openvpn" "$files_path" <<'PY'
import hashlib, json, pathlib, sys
root = pathlib.Path(sys.argv[1])
out = pathlib.Path(sys.argv[2])
files = {}
for path in sorted(root.rglob("*")):
    if not path.is_file() or path.name in ("manifest.json", ".files.json"):
        continue
    rel = path.relative_to(root).as_posix()
    if rel == "bin/openvpn.exe":
        continue
    files[f"openvpn/{rel}"] = hashlib.sha256(path.read_bytes()).hexdigest()
out.write_text(json.dumps(files), encoding="utf-8")
PY
  write_manifest "$ENGINE_ROOT/openvpn" openvpn openvpn "$OPENVPN_VERSION" "openvpn/bin/openvpn.exe" "$files_path"
  rm -f "$files_path"
  echo "[openvpn] staged $(du -sh "$ENGINE_ROOT/openvpn" | awk '{print $1}')"
}

# ─── OpenConnect (MSYS2 redistributable, best-effort) ─────────────
stage_openconnect_helpers() {
  echo "[openconnect] building Windows helpers (vpnc-script, browser-open)..."
  (
    cd "$ROOT_DIR/vpntoris-tray"
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" \
      -o "$ENGINE_ROOT/openconnect/bin/vpntoris-vpnc-script.exe" ./cmd/vpntoris-vpnc-script
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" \
      -o "$ENGINE_ROOT/openconnect/bin/vpntoris-browser-open.exe" ./cmd/vpntoris-browser-open
  )
}

stage_openconnect_engine() {
  stage_openconnect_helpers

  if [[ $SKIP_OPENCONNECT == 1 ]]; then
    echo "[openconnect] SKIP_OPENCONNECT=1 — helpers only, no engine binary"
    write_openconnect_placeholder
    return 0
  fi

  # Try MSYS2 package if URL provided or probe a known mirror path.
  local pkg="$CACHE_DIR/mingw-w64-x86_64-openconnect.pkg.tar.zst"
  if [[ -n $OPENCONNECT_MSYS_URL ]]; then
    download "$OPENCONNECT_MSYS_URL" "$pkg"
  else
    # Probe recent MSYS2 package name pattern; fail soft if unavailable.
    local probe_urls=(
      "https://repo.msys2.org/mingw/mingw64/mingw-w64-x86_64-openconnect-9.12-3-any.pkg.tar.zst"
      "https://mirror.msys2.org/mingw/mingw64/mingw-w64-x86_64-openconnect-9.12-3-any.pkg.tar.zst"
    )
    local ok=0
    for url in "${probe_urls[@]}"; do
      if curl -fL --retry 2 -o "$pkg.partial" "$url" 2>/dev/null; then
        mv "$pkg.partial" "$pkg"
        ok=1
        echo "[openconnect] fetched $(basename "$url")"
        break
      fi
      rm -f "$pkg.partial"
    done
    if [[ $ok -ne 1 ]]; then
      echo "[openconnect] warning: MSYS2 package not found — shipping helpers only"
      write_openconnect_placeholder
      return 0
    fi
  fi

  if ! command -v zstd >/dev/null 2>&1 && ! command -v tar >/dev/null 2>&1; then
    echo "[openconnect] warning: cannot extract .pkg.tar.zst (need tar+zstd)"
    write_openconnect_placeholder
    return 0
  fi

  local oc_extract="$CACHE_DIR/openconnect-msys-extract"
  rm -rf "$oc_extract"
  mkdir -p "$oc_extract"
  if ! tar -C "$oc_extract" -xf "$pkg" 2>/dev/null; then
    echo "[openconnect] warning: failed to extract MSYS2 package"
    write_openconnect_placeholder
    return 0
  fi

  local oc_bin
  oc_bin=$(find "$oc_extract" -type f -iname 'openconnect.exe' | head -1 || true)
  if [[ -z $oc_bin ]]; then
    echo "[openconnect] warning: openconnect.exe missing from package"
    write_openconnect_placeholder
    return 0
  fi
  cp -f "$oc_bin" "$ENGINE_ROOT/openconnect/bin/openconnect.exe"
  # Collect DLLs from the package tree (mingw64/bin/*.dll)
  local mingw_bin
  mingw_bin=$(dirname "$oc_bin")
  find "$mingw_bin" -maxdepth 1 -type f -iname '*.dll' | while read -r dll; do
    cp -f "$dll" "$ENGINE_ROOT/openconnect/bin/$(basename "$dll")"
    cp -f "$dll" "$ENGINE_ROOT/openconnect/lib/$(basename "$dll")"
  done

  # Alias .exe helpers without extension expectation is Windows-native (.exe).
  # nativehelper looks for vpntoris-vpnc-script without .exe on Unix; Windows needs adjustment.
  # Keep both names: script.exe is primary; also copy bare name for Unix-style lookup if we fix later.
  cp -f "$ENGINE_ROOT/openconnect/bin/vpntoris-vpnc-script.exe" "$ENGINE_ROOT/openconnect/bin/vpntoris-vpnc-script" 2>/dev/null || true
  cp -f "$ENGINE_ROOT/openconnect/bin/vpntoris-browser-open.exe" "$ENGINE_ROOT/openconnect/bin/vpntoris-browser-open" 2>/dev/null || true

  local files_path="$ENGINE_ROOT/openconnect/.files.json"
  python3 - "$ENGINE_ROOT/openconnect" "$files_path" <<'PY'
import hashlib, json, pathlib, sys
root = pathlib.Path(sys.argv[1])
out = pathlib.Path(sys.argv[2])
files = {}
for path in sorted(root.rglob("*")):
    if not path.is_file() or path.name in ("manifest.json", ".files.json"):
        continue
    rel = path.relative_to(root).as_posix()
    if rel == "bin/openconnect.exe":
        continue
    files[f"openconnect/{rel}"] = hashlib.sha256(path.read_bytes()).hexdigest()
out.write_text(json.dumps(files), encoding="utf-8")
PY
  write_manifest "$ENGINE_ROOT/openconnect" openconnect openconnect "$OPENCONNECT_VERSION_LABEL" "openconnect/bin/openconnect.exe" "$files_path"
  rm -f "$files_path"
  echo "[openconnect] staged $(du -sh "$ENGINE_ROOT/openconnect" | awk '{print $1}')"
}

write_openconnect_placeholder() {
  cat >"$ENGINE_ROOT/openconnect/README.txt" <<'EOF'
OpenConnect engine binary was not packaged in this build (MSYS2 package unavailable
or SKIP_OPENCONNECT=1). Helper binaries may still be present for future use.

OpenVPN + Wintun are the supported Windows engines in this release.
FortiGate SSL (openfortivpn) and strongSwan IPsec remain deferred on Windows.
EOF
  # Remove empty engine expectation so MSI still installs helpers if any.
  if [[ ! -f $ENGINE_ROOT/openconnect/bin/openconnect.exe ]]; then
    # Do not write a fake manifest pointing at a missing executable.
    rm -f "$ENGINE_ROOT/openconnect/manifest.json"
  fi
}

# ─── Protocol limit notes ─────────────────────────────────────────
write_limits() {
  cat >"$ENGINE_ROOT/ENGINE_NOTES.txt" <<EOF
VPNToris Windows engines (windows-amd64)
========================================
Built: $(date -u '+%Y-%m-%dT%H:%M:%SZ')
Host:  $(uname -s)/$(uname -m)

Included:
  - openvpn ${OPENVPN_VERSION} (+ wintun ${WINTUN_VERSION})
  - openconnect: see openconnect/ (best-effort MSYS2 binary + Go helpers)

Not packaged / not supported on Windows yet:
  - openfortivpn (FortiGate SSL PPP path)
  - strongSwan / charon (IPsec)

Runtime layout after MSI install:
  %ProgramData%\\VPNToris\\Engines\\windows-amd64\\{openvpn,openconnect}\\...
  Helper service: vpntoris-native-helper.exe service <EngineRoot>
EOF
}

echo "=== VPNToris Windows engines ==="
echo "out: $ENGINE_ROOT"
stage_openvpn
stage_openconnect_engine
write_limits
echo
echo "=== Engines ready ==="
find "$ENGINE_ROOT" -type f | sed 's|^|  |' | head -80
echo "  …"
du -sh "$ENGINE_ROOT"/* 2>/dev/null || true
