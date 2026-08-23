#!/usr/bin/env bash
set -euo pipefail

ENGINE_BASE=${1:?usage: stage-engines.sh <engine-base> <goarch> <side-bin-dir>}
GOARCH=${2:?usage: stage-engines.sh <engine-base> <goarch> <side-bin-dir>}
SIDE_BIN_DIR=${3:?usage: stage-engines.sh <engine-base> <goarch> <side-bin-dir>}

mkdir -p "$ENGINE_BASE"

if command -v openvpn >/dev/null 2>&1; then
  mkdir -p "$ENGINE_BASE/openvpn/bin"
  cp "$(command -v openvpn)" "$ENGINE_BASE/openvpn/bin/openvpn"
  chmod 755 "$ENGINE_BASE/openvpn/bin/openvpn"
  OVPN_VERSION=$(openvpn --version 2>&1 | head -n1 | awk '{print $2}' || echo "2.7.x")
  OVPN_SHA=$(sha256sum "$ENGINE_BASE/openvpn/bin/openvpn" | awk '{print $1}')
  cat > "$ENGINE_BASE/openvpn/manifest.json" <<EOF
{
  "id": "openvpn",
  "protocol": "openvpn",
  "version": "$OVPN_VERSION",
  "os": "linux",
  "architecture": "$GOARCH",
  "executable": "openvpn/bin/openvpn",
  "sha256": "$OVPN_SHA",
  "license": "GPL-2.0-only WITH OpenSSL-exception",
  "capabilities": ["tun", "userpass", "challenge", "split-route"]
}
EOF
fi

if command -v openfortivpn >/dev/null 2>&1; then
  mkdir -p "$ENGINE_BASE/openfortivpn/bin"
  cp "$(command -v openfortivpn)" "$ENGINE_BASE/openfortivpn/bin/openfortivpn"
  chmod 755 "$ENGINE_BASE/openfortivpn/bin/openfortivpn"
  FORTI_VERSION=$(openfortivpn --version 2>&1 | head -n1 | awk '{print $NF}' || echo "1.24.x")
  FORTI_SHA=$(sha256sum "$ENGINE_BASE/openfortivpn/bin/openfortivpn" | awk '{print $1}')
  cat > "$ENGINE_BASE/openfortivpn/manifest.json" <<EOF
{
  "id": "openfortivpn",
  "protocol": "fortigate-ssl",
  "version": "$FORTI_VERSION",
  "os": "linux",
  "architecture": "$GOARCH",
  "executable": "openfortivpn/bin/openfortivpn",
  "sha256": "$FORTI_SHA",
  "license": "GPL-3.0-or-later WITH OpenSSL-exception",
  "capabilities": ["ppp", "otp", "split-route"]
}
EOF
fi

if command -v openconnect >/dev/null 2>&1; then
  mkdir -p "$ENGINE_BASE/openconnect/bin"
  cp "$(command -v openconnect)" "$ENGINE_BASE/openconnect/bin/openconnect"
  cp "$SIDE_BIN_DIR/vpntoris-vpnc-script" "$ENGINE_BASE/openconnect/bin/vpntoris-vpnc-script"
  cp "$SIDE_BIN_DIR/vpntoris-browser-open" "$ENGINE_BASE/openconnect/bin/vpntoris-browser-open"
  chmod 755 "$ENGINE_BASE/openconnect/bin/"*
  OC_VERSION=$(openconnect --version 2>&1 | head -n1 | awk '{print $NF}' || echo "9.x")
  OC_SHA=$(sha256sum "$ENGINE_BASE/openconnect/bin/openconnect" | awk '{print $1}')
  cat > "$ENGINE_BASE/openconnect/manifest.json" <<EOF
{
  "id": "openconnect",
  "protocol": "openconnect",
  "version": "$OC_VERSION",
  "os": "linux",
  "architecture": "$GOARCH",
  "executable": "openconnect/bin/openconnect",
  "sha256": "$OC_SHA",
  "license": "LGPL-2.1-or-later",
  "capabilities": ["anyconnect", "gp", "pulse", "nc", "f5", "fortinet", "array", "otp", "split-route"]
}
EOF
fi

CHARON_BIN=""
for candidate in /usr/lib/ipsec/charon /usr/libexec/ipsec/charon /usr/sbin/charon /usr/bin/charon; do
  if [[ -x "$candidate" ]]; then
    CHARON_BIN="$candidate"
    break
  fi
done
if [[ -n "$CHARON_BIN" ]]; then
  mkdir -p "$ENGINE_BASE/strongswan/bin" "$ENGINE_BASE/strongswan/plugins"
  cp "$CHARON_BIN" "$ENGINE_BASE/strongswan/bin/charon"
  if command -v swanctl >/dev/null 2>&1; then
    cp "$(command -v swanctl)" "$ENGINE_BASE/strongswan/bin/swanctl"
  fi
  chmod 755 "$ENGINE_BASE/strongswan/bin/"*
  for pdir in /usr/lib/ipsec/plugins /usr/lib/strongswan/plugins; do
    if [[ -d "$pdir" ]]; then
      cp -a "$pdir"/. "$ENGINE_BASE/strongswan/plugins/" || true
    fi
  done
  CHARON_SHA=$(sha256sum "$ENGINE_BASE/strongswan/bin/charon" | awk '{print $1}')
  python3 - "$ENGINE_BASE" "$GOARCH" "$CHARON_SHA" <<'PY'
import hashlib, json, pathlib, sys
root = pathlib.Path(sys.argv[1]) / "strongswan"
files = {}
for path in sorted((root / "plugins").rglob("*")):
    if path.is_file():
        rel = path.relative_to(root)
        files[f"strongswan/{rel.as_posix()}"] = hashlib.sha256(path.read_bytes()).hexdigest()
manifest = {
  "id": "strongswan",
  "protocol": "ipsec",
  "version": "distro",
  "os": "linux",
  "architecture": sys.argv[2],
  "executable": "strongswan/bin/charon",
  "sha256": sys.argv[3],
  "license": "GPL-2.0-or-later",
  "capabilities": ["ikev1", "ikev2", "xauth", "xauth-otp", "eap", "sha1", "dh20", "split-route"],
  "files": files,
}
(root / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
PY
fi
