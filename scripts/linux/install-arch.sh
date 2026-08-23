#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(cd "$SCRIPT_DIR/../.." && pwd)
VERSION=${VERSION:-2.1.4}
GOARCH=${GOARCH:-amd64}

record_desktop_user() {
  local desktop_uid=""
  if [[ -n "${SUDO_USER:-}" && "$SUDO_USER" != "root" ]]; then
    desktop_uid=$(id -u "$SUDO_USER" 2>/dev/null || true)
  fi
  if [[ -z "$desktop_uid" ]] && command -v loginctl >/dev/null 2>&1; then
    desktop_uid=$(loginctl list-sessions --no-legend 2>/dev/null | awk '{print $2}' | grep -E '^[0-9]+$' | head -1 || true)
  fi
  if [[ -n "$desktop_uid" ]]; then
    mkdir -p /etc/vpntoris
    touch /etc/vpntoris/helper-users.conf
    grep -qx "$desktop_uid" /etc/vpntoris/helper-users.conf 2>/dev/null || echo "$desktop_uid" >> /etc/vpntoris/helper-users.conf
    chmod 644 /etc/vpntoris/helper-users.conf
  fi
}

UNINSTALL=false
for arg in "$@"; do
  case "$arg" in
    --uninstall|-u)
      UNINSTALL=true
      ;;
    -h|--help)
      cat <<EOF
VPNToris - Arch Linux installer
(Arch Linux, EndeavourOS, Manjaro, Garuda etc.)

Usage:
  sudo ./scripts/linux/install-arch.sh             (install)
  sudo ./scripts/linux/install-arch.sh --uninstall (remove)

Requirements (pacman):
  go, openvpn, openfortivpn, openconnect, ppp, python-gobject, gtk3, libsecret, kdialog
EOF
      exit 0
      ;;
  esac
done

if [[ "$UNINSTALL" == "true" ]]; then
  if [[ $EUID -ne 0 ]]; then
    echo "[-] Uninstall requires root. Run 'sudo $0 --uninstall'." >&2
    exit 1
  fi
  echo "[*] Removing VPNToris..."
  systemctl stop vpntoris-native.service 2>/dev/null || true
  systemctl disable vpntoris-native.service 2>/dev/null || true

  rm -f /usr/lib/systemd/system/vpntoris-native.service
  rm -f /usr/lib/systemd/user/vpntorisd.service
  systemctl daemon-reload 2>/dev/null || true

  rm -f /usr/bin/vpntorisctl /usr/bin/vpntoris-tray
  rm -rf /usr/lib/vpntoris
  rm -rf /var/lib/vpntoris/engines
  rm -f /etc/vpntoris/helper-users.conf
  rm -f /usr/share/applications/vpntoris-tray.desktop
  rm -f /etc/xdg/autostart/vpntoris-tray.desktop
  rm -f /usr/share/pixmaps/vpntoris.png
  for size in 16 22 24 32 48 64 128 256; do
    rm -f "/usr/share/icons/hicolor/${size}x${size}/apps/vpntoris.png"
  done

  echo "[✓] VPNToris removed."
  exit 0
fi

echo "========================================="
echo "    VPNToris - Arch Linux Installer      "
echo "   Version: $VERSION (arch: $GOARCH)      "
echo "========================================="

echo "[1/4] Checking dependencies..."
MISSING_PKGS=()
check_pkg() {
  if ! pacman -Q "$1" >/dev/null 2>&1; then
    MISSING_PKGS+=("$1")
  fi
}

check_pkg "go"
check_pkg "openvpn"
check_pkg "openfortivpn"
check_pkg "openconnect"
check_pkg "ppp"
check_pkg "python-gobject"
check_pkg "gtk3"
check_pkg "libsecret"

if [[ ${#MISSING_PKGS[@]} -gt 0 ]]; then
  echo "[!] Missing packages: ${MISSING_PKGS[*]}"
  if [[ $EUID -eq 0 ]]; then
    echo "[*] Installing missing packages with pacman..."
    pacman -S --needed --noconfirm "${MISSING_PKGS[@]}"
  else
    echo "[!] Install them first: sudo pacman -S ${MISSING_PKGS[*]}"
  fi
else
  echo "[+] All required packages present."
fi

BUILD_DIR="$ROOT_DIR/build/arch"
BIN_DIR="$BUILD_DIR/bin"
ENGINE_BASE="$BUILD_DIR/engines/linux-$GOARCH"

echo "[2/4] Building Go binaries..."
mkdir -p "$BIN_DIR" "$ENGINE_BASE"

cd "$ROOT_DIR/vpntoris-tray"
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "$BIN_DIR/vpntorisd" .
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_DIR/vpntoris-native-helper" ./cmd/vpntoris-native-helper
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_DIR/vpntoris-service" ./cmd/vpntoris-service
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_DIR/vpntorisctl" ./cli
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_DIR/vpntoris-tray" ./cmd/vpntoris-tray
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_DIR/vpntoris-vpnc-script" ./cmd/vpntoris-vpnc-script
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_DIR/vpntoris-browser-open" ./cmd/vpntoris-browser-open

echo "[+] Binaries ready: $BIN_DIR"

echo "[3/4] Staging VPN engines (OpenVPN, OpenFortiVPN, OpenConnect, strongSwan)..."
bash "$ROOT_DIR/scripts/linux/stage-engines.sh" "$ENGINE_BASE" "$GOARCH" "$BIN_DIR"
echo "[+] Engines staged."

if [[ $EUID -ne 0 ]]; then
  echo
  echo "=========================================================="
  echo "   Build complete. To install system files, run:          "
  echo "   sudo ./scripts/linux/install-arch.sh                   "
  echo "=========================================================="
  exit 0
fi

echo "[4/4] Installing system files..."
systemctl stop vpntoris-native.service 2>/dev/null || true
pkill -f /usr/lib/vpntoris/ 2>/dev/null || true
pkill -f /usr/bin/vpntoris 2>/dev/null || true
sleep 1

mkdir -p \
  /usr/lib/vpntoris \
  /usr/bin \
  /usr/lib/systemd/system \
  /usr/lib/systemd/user \
  /usr/share/applications \
  /etc/xdg/autostart \
  /var/lib/vpntoris/engines/linux-$GOARCH \
  /var/lib/vpntoris/state \
  /var/log/vpntoris \
  /run/vpntoris-native

install -m755 "$BIN_DIR/vpntorisd" /usr/lib/vpntoris/vpntorisd
install -m755 "$BIN_DIR/vpntoris-native-helper" /usr/lib/vpntoris/vpntoris-native-helper
install -m755 "$BIN_DIR/vpntoris-service" /usr/lib/vpntoris/vpntoris-service
install -m755 "$BIN_DIR/vpntorisctl" /usr/bin/vpntorisctl
install -m755 "$BIN_DIR/vpntoris-tray" /usr/bin/vpntoris-tray
install -m755 "$ROOT_DIR/vpntoris-tray/cmd/vpntoris-tray/gui_linux.py" /usr/lib/vpntoris/vpntoris-gui
install -m755 "$ROOT_DIR/scripts/linux/packaging/vpntoris-tray-autostart.sh" /usr/lib/vpntoris/vpntoris-tray-autostart

chmod 755 /var/log/vpntoris /var/lib/vpntoris

cp --remove-destination -a "$ENGINE_BASE/." "/var/lib/vpntoris/engines/linux-$GOARCH/"

cp "$ROOT_DIR/scripts/linux/packaging/vpntoris-tray.desktop" /usr/share/applications/
cp "$ROOT_DIR/scripts/linux/packaging/vpntoris-tray-autostart.desktop" /etc/xdg/autostart/vpntoris-tray.desktop

for size in 16 22 24 32 48 64 128 256; do
  src="$ROOT_DIR/assets/icons/vpntoris-${size}.png"
  if [[ -f "$src" ]]; then
    mkdir -p "/usr/share/icons/hicolor/${size}x${size}/apps"
    cp "$src" "/usr/share/icons/hicolor/${size}x${size}/apps/vpntoris.png"
  fi
done
if [[ -f "$ROOT_DIR/assets/icons/vpntoris-128.png" ]]; then
  cp "$ROOT_DIR/assets/icons/vpntoris-128.png" /usr/share/pixmaps/vpntoris.png
elif [[ -f "$ROOT_DIR/assets/vpntoris-logo.png" ]]; then
  cp "$ROOT_DIR/assets/vpntoris-logo.png" /usr/share/pixmaps/vpntoris.png
fi

cp "$ROOT_DIR/scripts/linux/packaging/vpntoris-native.service" /usr/lib/systemd/system/
cp "$ROOT_DIR/scripts/linux/packaging/vpntorisd.user.service" /usr/lib/systemd/user/vpntorisd.service

record_desktop_user

systemctl daemon-reload
systemctl enable --now vpntoris-native.service

if [[ -n "${SUDO_USER:-}" ]]; then
  USER_UID=$(id -u "$SUDO_USER")
  sudo -u "$SUDO_USER" XDG_RUNTIME_DIR="/run/user/$USER_UID" DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/$USER_UID/bus" systemctl --user daemon-reload 2>/dev/null || true
  sudo -u "$SUDO_USER" XDG_RUNTIME_DIR="/run/user/$USER_UID" DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/$USER_UID/bus" systemctl --user enable --now vpntorisd.service 2>/dev/null || true
fi

echo
echo "=========================================================="
echo "   [✓] VPNToris installed successfully.                   "
echo "=========================================================="
echo "Service status:"
systemctl is-active vpntoris-native.service && echo " -> vpntoris-native.service: active"
echo
echo "Usage:"
echo " 1. Start the tray app:"
echo "    vpntoris-tray &"
echo "    (or search for 'VPNToris' in the application menu)"
echo
echo " 2. CLI profile and status control:"
echo "    vpntorisctl status"
echo "    vpntorisctl profiles"
echo "    vpntorisctl connect <profile_name>"
echo
echo " 3. To uninstall:"
echo "    sudo $0 --uninstall"
echo "=========================================================="
