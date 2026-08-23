#!/bin/bash
set -eu
export PATH="/usr/bin:/bin:${PATH:-}"

if [[ -z ${DBUS_SESSION_BUS_ADDRESS:-} && -n ${XDG_RUNTIME_DIR:-} && -S ${XDG_RUNTIME_DIR}/bus ]]; then
  export DBUS_SESSION_BUS_ADDRESS="unix:path=${XDG_RUNTIME_DIR}/bus"
fi

systemctl --user start vpntorisd.service 2>/dev/null || true

wait_for_watcher() {
  local i
  for i in $(seq 1 90); do
    if busctl --user status org.kde.StatusNotifierWatcher >/dev/null 2>&1; then
      return 0
    fi
    if busctl --user list 2>/dev/null | grep -q 'org\.kde\.StatusNotifierWatcher'; then
      return 0
    fi
    sleep 1
  done
  return 1
}

if ! wait_for_watcher; then
  echo "vpntoris-tray: StatusNotifierWatcher not available after 90s" >&2
  echo "  GNOME: enable the bundled extension, then re-login:" >&2
  echo "         gnome-extensions enable vpntoris-appindicator@vpntoris.local" >&2
fi

exec /usr/bin/vpntoris-tray "$@"
