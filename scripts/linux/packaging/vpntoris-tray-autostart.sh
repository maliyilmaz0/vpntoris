#!/bin/bash
# Wait for a StatusNotifier host (GNOME AppIndicator / KDE / etc.) then start the tray.
# Without a watcher, fyne systray registers on D-Bus but nothing appears in the panel.
set -eu
export PATH="/usr/bin:/bin:${PATH:-}"

wait_for_watcher() {
  local i
  for i in $(seq 1 90); do
    if busctl --user status org.kde.StatusNotifierWatcher >/dev/null 2>&1; then
      return 0
    fi
    # Some desktops expose the name without a full systemd unit status.
    if busctl --user list 2>/dev/null | grep -q 'org\.kde\.StatusNotifierWatcher'; then
      return 0
    fi
    sleep 1
  done
  return 1
}

# Prefer the logged-in graphical session bus.
if [[ -z ${DBUS_SESSION_BUS_ADDRESS:-} && -n ${XDG_RUNTIME_DIR:-} && -S ${XDG_RUNTIME_DIR}/bus ]]; then
  export DBUS_SESSION_BUS_ADDRESS="unix:path=${XDG_RUNTIME_DIR}/bus"
fi

if ! wait_for_watcher; then
  echo "vpntoris-tray: StatusNotifierWatcher not available after 90s" >&2
  echo "  GNOME: enable the bundled extension, then re-login:" >&2
  echo "         gnome-extensions enable vpntoris-appindicator@vpntoris.local" >&2
  # Still start — stayRegistered() will attach if the watcher appears later.
fi

exec /usr/bin/vpntoris-tray "$@"
