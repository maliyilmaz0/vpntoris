package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// dialogBusy blocks menu refresh while a modal window is open so AppIndicator
// hosts do not rebuild/dismiss the tray menu (or kill the dialog) mid-action.
var dialogBusy atomic.Bool

func withDialog(fn func()) {
	dialogBusy.Store(true)
	defer dialogBusy.Store(false)
	// Let the StatusNotifier menu finish closing before stealing focus.
	// GNOME AppIndicator otherwise races the dialog and kills it instantly.
	if runtime.GOOS == "linux" {
		time.Sleep(250 * time.Millisecond)
	}
	fn()
}

// dialogTimeout bounds how long a helper dialog process may run; a hung
// zenity/kdialog must not wedge the tray forever (dialogBusy stays set and
// the refresh loop stalls).
const dialogTimeout = 5 * time.Minute

// runDialog runs a dialog helper process with the desktop environment and a
// hard timeout.
func runDialog(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), dialogTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = desktopEnv()
	return cmd.Run()
}

func desktopEnv() []string {
	env := os.Environ()
	hasDisplay := false
	for _, item := range env {
		if strings.HasPrefix(item, "DISPLAY=") || strings.HasPrefix(item, "WAYLAND_DISPLAY=") {
			hasDisplay = true
			break
		}
	}
	if !hasDisplay {
		env = append(env, "DISPLAY=:0")
	}
	return env
}

func showTextDialog(title, text string) {
	if len(text) > 200000 {
		text = text[:200000] + "\n…(truncated)"
	}
	withDialog(func() {
		switch runtime.GOOS {
		case "linux":
			if gtkShowText(title, text) == nil {
				return
			}
			// Fallback: write file, run zenity with full env, delete after exit.
			tmp, err := os.CreateTemp("", "vpntoris-log-*.txt")
			if err != nil {
				return
			}
			path := tmp.Name()
			_, _ = tmp.WriteString(text)
			_ = tmp.Close()
			_ = runDialog("zenity", "--text-info",
				"--title="+title,
				"--filename="+path,
				"--width=760",
				"--height=520",
				"--ok-label=Close",
			)
			_ = os.Remove(path)
		case "windows":
			_ = runDialog("powershell", "-NoProfile", "-Command",
				fmt.Sprintf(`[System.Windows.Forms.MessageBox]::Show('%s','%s')`, escapePS(truncate(text, 1500)), escapePS(title)))
		case "darwin":
			_ = runDialog("osascript", "-e",
				fmt.Sprintf(`display dialog %q with title %q buttons {"OK"} default button "OK"`, truncate(text, 900), title))
		}
	})
}

func gtkShowText(title, text string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("not linux")
	}
	tmp, err := os.CreateTemp("", "vpntoris-log-*.txt")
	if err != nil {
		return err
	}
	path := tmp.Name()
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		os.Remove(path)
		return err
	}
	tmp.Close()
	defer os.Remove(path)

	script := `#!/usr/bin/env python3
import sys
import gi
gi.require_version("Gtk", "3.0")
from gi.repository import Gtk, GLib, Gdk
try:
    gi.require_version("Pango", "1.0")
    from gi.repository import Pango
except Exception:
    Pango = None

title = sys.argv[1]
path = sys.argv[2]
try:
    with open(path, "r", encoding="utf-8", errors="replace") as f:
        body = f.read()
except Exception as e:
    body = "could not read log: %s" % e
if not body.strip():
    body = "(no log output yet)"

class LogWin(Gtk.Window):
    def __init__(self):
        Gtk.Window.__init__(self, title=title)
        self.set_default_size(780, 540)
        self.set_border_width(10)
        self.set_type_hint(Gdk.WindowTypeHint.NORMAL)
        self.set_keep_above(True)
        self.set_skip_taskbar_hint(False)
        self.connect("destroy", Gtk.main_quit)
        box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=8)
        self.add(box)
        scroll = Gtk.ScrolledWindow()
        scroll.set_policy(Gtk.PolicyType.AUTOMATIC, Gtk.PolicyType.AUTOMATIC)
        scroll.set_hexpand(True)
        scroll.set_vexpand(True)
        box.pack_start(scroll, True, True, 0)
        view = Gtk.TextView()
        view.set_editable(False)
        view.set_cursor_visible(False)
        view.set_wrap_mode(Gtk.WrapMode.CHAR)
        if Pango is not None:
            try:
                view.override_font(Pango.FontDescription.from_string("Monospace 10"))
            except Exception:
                pass
        view.get_buffer().set_text(body)
        scroll.add(view)
        row = Gtk.Box(spacing=8)
        row.set_halign(Gtk.Align.END)
        btn = Gtk.Button(label="Close")
        btn.connect("clicked", lambda *_: self.destroy())
        row.pack_start(btn, False, False, 0)
        box.pack_start(row, False, False, 0)

GLib.set_prgname("vpntoris-tray")
win = LogWin()
win.show_all()
GLib.timeout_add(50, lambda: (win.present(), False)[1])
Gtk.main()
`
	py, err := os.CreateTemp("", "vpntoris-logview-*.py")
	if err != nil {
		return err
	}
	scriptPath := py.Name()
	if _, err := py.WriteString(script); err != nil {
		py.Close()
		os.Remove(scriptPath)
		return err
	}
	py.Close()
	defer os.Remove(scriptPath)
	_ = os.Chmod(scriptPath, 0700)

	return runDialog("python3", scriptPath, title, path)
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "…"
}

func openConfigDir() {
	withDialog(func() {
		dir, err := os.UserConfigDir()
		if err != nil {
			return
		}
		path := filepath.Join(dir, "VPNToris")
		_ = os.MkdirAll(path, 0700)
		switch runtime.GOOS {
		case "linux":
			cmd := exec.Command("xdg-open", path)
			cmd.Env = desktopEnv()
			_ = cmd.Start()
		case "darwin":
			_ = exec.Command("open", path).Start()
		case "windows":
			_ = exec.Command("explorer", path).Start()
		}
	})
}
