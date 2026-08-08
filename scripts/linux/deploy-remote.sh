#!/bin/bash
# Deploy VPNToris Linux packages to a remote host (e.g. ESXi guest with a desktop).
#
# Use this when you have real VMs with UI (Ubuntu/GNOME, Rocky, etc.) on ESXi
# and want to install the built DEB/RPM over SSH for tray testing.
#
# Usage:
#   ./scripts/linux/deploy-remote.sh user@10.10.10.50
#   ./scripts/linux/deploy-remote.sh user@10.10.10.50 path/to/vpntoris_2.0.0_amd64.deb
#   ARCH=arm64 ./scripts/linux/deploy-remote.sh user@host
#   ./scripts/linux/deploy-remote.sh --start-tray user@host
#
# Prerequisites on the guest:
#   - SSH + sudo
#   - Desktop session for tray (GNOME/KDE/XFCE + AppIndicator)
#   - Matching package arch (amd64 DEB on x86_64 guest, arm64 on aarch64)
#
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
START_TRAY=false
TARGET=""
PACKAGE=""

for arg in "$@"; do
  case "$arg" in
    --start-tray) START_TRAY=true ;;
    -h | --help)
      sed -n '2,25p' "$0"
      exit 0
      ;;
    -*)
      echo "unknown flag: $arg" >&2
      exit 1
      ;;
    *)
      if [[ -z $TARGET ]]; then
        TARGET=$arg
      elif [[ -z $PACKAGE ]]; then
        PACKAGE=$arg
      else
        echo "unexpected arg: $arg" >&2
        exit 1
      fi
      ;;
  esac
done

if [[ -z $TARGET ]]; then
  echo "usage: $0 [--start-tray] user@host [package.deb|package.rpm]" >&2
  exit 1
fi

if [[ ! $TARGET =~ @ ]]; then
  echo "error: target must look like user@ip (got: $TARGET)" >&2
  exit 1
fi

# Probe guest arch if package not given.
remote_arch() {
  ssh -o BatchMode=yes -o ConnectTimeout=8 "$TARGET" 'uname -m' 2>/dev/null || true
}

pick_package() {
  local arch=${ARCH:-}
  if [[ -z $arch ]]; then
    local m
    m=$(remote_arch)
    case "$m" in
      x86_64 | amd64) arch=amd64 ;;
      aarch64 | arm64) arch=arm64 ;;
      *)
        echo "error: could not detect guest arch (ssh $TARGET uname -m). Pass package path explicitly." >&2
        exit 1
        ;;
    esac
  fi
  case "$arch" in
    amd64 | x86_64)
      ls -t "$ROOT_DIR"/versions/*/linux/vpntoris_*_amd64.deb \
        "$ROOT_DIR"/.build/linux/amd64/vpntoris_*_amd64.deb \
        "$ROOT_DIR"/versions/*/linux/vpntoris-*-1.*.x86_64.rpm \
        "$ROOT_DIR"/.build/linux/amd64/vpntoris-*-1.*.x86_64.rpm \
        2>/dev/null | head -1
      ;;
    arm64 | aarch64)
      ls -t "$ROOT_DIR"/versions/*/linux/vpntoris_*_arm64.deb \
        "$ROOT_DIR"/.build/linux/arm64/vpntoris_*_arm64.deb \
        "$ROOT_DIR"/versions/*/linux/vpntoris-*-1.*.aarch64.rpm \
        "$ROOT_DIR"/.build/linux/arm64/vpntoris-*-1.*.aarch64.rpm \
        2>/dev/null | head -1
      ;;
    *)
      echo "error: unsupported ARCH=$arch" >&2
      exit 1
      ;;
  esac
}

if [[ -z $PACKAGE ]]; then
  PACKAGE=$(pick_package || true)
fi
if [[ -z ${PACKAGE:-} || ! -f $PACKAGE ]]; then
  echo "error: package not found." >&2
  echo "  Build first, e.g.:" >&2
  echo "    VERSION=2.0.0 GOARCH=amd64 ./scripts/linux/build-packages.sh" >&2
  echo "  Or pass the file:" >&2
  echo "    $0 $TARGET /path/to/vpntoris_….deb" >&2
  exit 1
fi

base=$(basename "$PACKAGE")
remote_path="/tmp/$base"

echo "[deploy] target:  $TARGET"
echo "[deploy] package: $PACKAGE"
echo "[deploy] copying..."
scp -o ConnectTimeout=15 "$PACKAGE" "$TARGET:$remote_path"

echo "[deploy] installing (sudo on guest)..."
ssh -o ConnectTimeout=15 "$TARGET" "bash -s" -- "$remote_path" "$START_TRAY" <<'REMOTE'
set -euo pipefail
pkg=$1
start_tray=$2
export DEBIAN_FRONTEND=noninteractive

case "$pkg" in
  *.deb)
    if command -v apt-get >/dev/null 2>&1; then
      sudo apt-get install -y "$pkg" || { sudo dpkg -i "$pkg" || sudo apt-get install -f -y; }
    else
      sudo dpkg -i "$pkg"
    fi
    ;;
  *.rpm)
    # Prefer dnf so Requires (ppp, iproute, ...) resolve from base repos.
    if command -v dnf >/dev/null 2>&1; then
      sudo dnf install -y "$pkg" || sudo rpm -Uvh --force "$pkg"
    elif command -v yum >/dev/null 2>&1; then
      sudo yum install -y "$pkg" || sudo rpm -Uvh --force "$pkg"
    else
      sudo rpm -Uvh --force "$pkg"
    fi
    ;;
  *)
    echo "unknown package type: $pkg" >&2
    exit 1
    ;;
esac

sudo systemctl daemon-reload || true
sudo systemctl enable vpntoris-native.service 2>/dev/null || true
sudo systemctl restart vpntoris-native.service 2>/dev/null \
  || sudo systemctl start vpntoris-native.service 2>/dev/null \
  || true

# User-session controller if not already running. Over SSH there is usually
# no user bus, so fall back to nohup when systemctl --user is unavailable.
if ! pgrep -x vpntorisd >/dev/null 2>&1; then
  systemctl --user start vpntorisd.service 2>/dev/null \
    || { [[ -x /usr/lib/vpntoris/vpntorisd ]] && nohup /usr/lib/vpntoris/vpntorisd >/tmp/vpntorisd.log 2>&1 & sleep 0.5; }
fi

echo "[guest] package:"
dpkg -l vpntoris 2>/dev/null | tail -1 || rpm -q vpntoris 2>/dev/null || true
echo "[guest] helper: $(systemctl is-active vpntoris-native.service 2>/dev/null || echo n/a)"
echo "[guest] vpntorisd: $(pgrep -x vpntorisd >/dev/null && echo running || echo not-running)"

if [[ $start_tray == true ]]; then
  if [[ -n ${DISPLAY:-} || -n ${WAYLAND_DISPLAY:-} ]]; then
    nohup vpntoris-tray >/tmp/vpntoris-tray.log 2>&1 &
    echo "[guest] tray started (DISPLAY=${DISPLAY:-} WAYLAND=${WAYLAND_DISPLAY:-})"
  else
    echo "[guest] no DISPLAY — log into the desktop GUI and run: vpntoris-tray"
  fi
else
  echo "[guest] start tray in the desktop session:  vpntoris-tray"
fi
REMOTE

echo
echo "[deploy] done. On the ESXi guest desktop (console / RDP / VMware tools):"
echo "  1. Log out and log back into the GNOME session (so the bundled AppIndicator extension loads)."
echo "  2. Tray auto-starts; or run:  vpntoris-tray"
echo "  3. Enable extension if needed: gnome-extensions enable vpntoris-appindicator@vpntoris.local"
echo "  4. Helper: systemctl status vpntoris-native.service"
echo "  5. Doctor:  vpntoris-service doctor"
