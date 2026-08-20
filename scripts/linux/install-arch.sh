#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(cd "$SCRIPT_DIR/../.." && pwd)
VERSION=${VERSION:-2.1.0}
GOARCH=${GOARCH:-amd64}

UNINSTALL=false
for arg in "$@"; do
  case "$arg" in
    --uninstall|-u)
      UNINSTALL=true
      ;;
    -h|--help)
      cat <<EOF
VPNToris - Arch Linux & Arch Tabanlı Dağıtımlar Kurulum Scripti
(Arch Linux, EndeavourOS, Manjaro, Garuda vb.)

Kullanım:
  sudo ./scripts/linux/install-arch.sh             (Kurulum)
  sudo ./scripts/linux/install-arch.sh --uninstall (Kaldırma)

Gereksinimler (pacman):
  go, openvpn, openfortivpn, openconnect, ppp, python-gobject, gtk3, libsecret, kdialog
EOF
      exit 0
      ;;
  esac
done

if [[ "$UNINSTALL" == "true" ]]; then
  if [[ $EUID -ne 0 ]]; then
    echo "[-] Kaldırma işlemi root yetkisi gerektirir. 'sudo $0 --uninstall' çalıştırın." >&2
    exit 1
  fi
  echo "[*] VPNToris kaldırılıyor..."
  systemctl stop vpntoris-native.service 2>/dev/null || true
  systemctl disable vpntoris-native.service 2>/dev/null || true
  
  rm -f /usr/lib/systemd/system/vpntoris-native.service
  rm -f /usr/lib/systemd/user/vpntorisd.service
  systemctl daemon-reload 2>/dev/null || true
  
  rm -f /usr/bin/vpntorisctl /usr/bin/vpntoris-tray
  rm -rf /usr/lib/vpntoris
  rm -rf /var/lib/vpntoris/engines
  rm -f /usr/share/applications/vpntoris-tray.desktop
  rm -f /etc/xdg/autostart/vpntoris-tray.desktop
  rm -f /usr/share/pixmaps/vpntoris.png
  for size in 16 22 24 32 48 64 128 256; do
    rm -f "/usr/share/icons/hicolor/${size}x${size}/apps/vpntoris.png"
  done
  
  echo "[✓] VPNToris başarıyla kaldırıldı."
  exit 0
fi

echo "========================================="
echo "    VPNToris - Arch Linux Installer      "
echo "   Version: $VERSION (arch: $GOARCH)      "
echo "========================================="

# 1. Bağımlılık Kontrolü
echo "[1/4] Bağımlılıklar kontrol ediliyor..."
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
  echo "[!] Eksik paketler tespit edildi: ${MISSING_PKGS[*]}"
  if [[ $EUID -eq 0 ]]; then
    echo "[*] Pacman ile eksik paketler kuruluyor..."
    pacman -S --needed --noconfirm "${MISSING_PKGS[@]}"
  else
    echo "[!] Lütfen önce eksik paketleri kurun: sudo pacman -S ${MISSING_PKGS[*]}"
  fi
else
  echo "[+] Tüm temel paketler kurulu."
fi

# 2. Derleme (User space)
BUILD_DIR="$ROOT_DIR/build/arch"
BIN_DIR="$BUILD_DIR/bin"
ENGINE_BASE="$BUILD_DIR/engines/linux-$GOARCH"

echo "[2/4] Go binary'leri derleniyor..."
mkdir -p "$BIN_DIR" "$ENGINE_BASE"

cd "$ROOT_DIR/vpntoris-tray"
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "$BIN_DIR/vpntorisd" .
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_DIR/vpntoris-native-helper" ./cmd/vpntoris-native-helper
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_DIR/vpntoris-service" ./cmd/vpntoris-service
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_DIR/vpntorisctl" ./cli
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_DIR/vpntoris-tray" ./cmd/vpntoris-tray
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_DIR/vpntoris-vpnc-script" ./cmd/vpntoris-vpnc-script
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_DIR/vpntoris-browser-open" ./cmd/vpntoris-browser-open

echo "[+] Binary'ler hazır: $BIN_DIR"

# 3. VPN Motorları ve Manifest Hazırlığı
echo "[3/4] VPN motorları (OpenVPN, OpenFortiVPN, OpenConnect) paketleniyor..."

# OpenVPN
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

# OpenFortiVPN
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

# OpenConnect
if command -v openconnect >/dev/null 2>&1; then
  mkdir -p "$ENGINE_BASE/openconnect/bin"
  cp "$(command -v openconnect)" "$ENGINE_BASE/openconnect/bin/openconnect"
  cp "$BIN_DIR/vpntoris-vpnc-script" "$ENGINE_BASE/openconnect/bin/vpntoris-vpnc-script"
  cp "$BIN_DIR/vpntoris-browser-open" "$ENGINE_BASE/openconnect/bin/vpntoris-browser-open"
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

# strongSwan (IPsec)
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
  python3 - <<PY
import hashlib, json, pathlib
root = pathlib.Path("$ENGINE_BASE/strongswan")
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
  "architecture": "$GOARCH",
  "executable": "strongswan/bin/charon",
  "sha256": "$CHARON_SHA",
  "license": "GPL-2.0-or-later",
  "capabilities": ["ikev1", "ikev2", "xauth", "xauth-otp", "eap", "sha1", "dh20", "split-route"],
  "files": files,
}
(root / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
PY
fi

echo "[+] Motorlar hazırlandı."

# 4. Sistem Kurulumu (Root yetkisi kontrolü)
if [[ $EUID -ne 0 ]]; then
  echo
  echo "=========================================================="
  echo "   Derleme tamamlandı! Sistem dosyalarını yüklemek için: "
  echo "   sudo ./scripts/linux/install-arch.sh                 "
  echo "=========================================================="
  exit 0
fi

echo "[4/4] Dosyalar sistem dizinlerine kopyalanıyor..."
# Çalışan servisleri geçici olarak durdur (ETXTBSY önlemek için)
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

systemctl daemon-reload
systemctl enable --now vpntoris-native.service

# Aktif kullanıcı oturumu varsa vpntorisd.service'i yeniden başlat
if [[ -n "${SUDO_USER:-}" ]]; then
  USER_UID=$(id -u "$SUDO_USER")
  sudo -u "$SUDO_USER" XDG_RUNTIME_DIR="/run/user/$USER_UID" DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/$USER_UID/bus" systemctl --user daemon-reload 2>/dev/null || true
  sudo -u "$SUDO_USER" XDG_RUNTIME_DIR="/run/user/$USER_UID" DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/$USER_UID/bus" systemctl --user enable --now vpntorisd.service 2>/dev/null || true
  sudo -u "$SUDO_USER" XDG_RUNTIME_DIR="/run/user/$USER_UID" DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/$USER_UID/bus" systemctl --user restart vpntorisd.service 2>/dev/null || true
fi

echo
echo "=========================================================="
echo "   [✓] VPNToris kurulumu başarıyla tamamlandı!           "
echo "=========================================================="
echo "Servis Durumu:"
systemctl is-active vpntoris-native.service && echo " -> vpntoris-native.service: AKTİF / ÇALIŞIYOR"
echo
echo "Kullanım:"
echo " 1. Tepsi uygulamasını başlatmak için:"
echo "    vpntoris-tray &"
echo "    (veya Uygulama Menüsünden 'VPNToris' aratarak açabilirsiniz)"
echo
echo " 2. CLI üzerinden profil ve durum kontrolü:"
echo "    vpntorisctl status"
echo "    vpntorisctl profiles"
echo "    vpntorisctl connect <profil_adi>"
echo
echo " 3. Kaldırmak isterseniz:"
echo "    sudo $0 --uninstall"
echo "=========================================================="
