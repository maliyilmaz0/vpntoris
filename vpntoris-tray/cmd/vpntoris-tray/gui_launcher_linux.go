//go:build linux

package main

import (
	_ "embed"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

//go:embed gui_linux.py
var guiLinuxScript []byte

var (
	guiCmdMu sync.Mutex
	guiCmd   *exec.Cmd
)

func openMainWindow() {
	guiCmdMu.Lock()
	defer guiCmdMu.Unlock()

	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = "/tmp"
	}
	sockPath := filepath.Join(runtimeDir, fmt.Sprintf("vpntoris-gui-%d.sock", os.Getuid()))

	if conn, err := net.DialTimeout("unix", sockPath, 200*time.Millisecond); err == nil {
		_, _ = conn.Write([]byte("show\n"))
		_ = conn.Close()
		return
	}

	if info, err := os.Stat("/usr/lib/vpntoris/vpntoris-gui"); err == nil && !info.IsDir() {
		cmd := exec.Command("python3", "/usr/lib/vpntoris/vpntoris-gui")
		cmd.Env = desktopEnv()
		if err := cmd.Start(); err == nil {
			guiCmd = cmd
			return
		}
	}

	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("vpntoris-gui-%d", os.Getuid()))
	_ = os.MkdirAll(tmpDir, 0700)
	scriptPath := filepath.Join(tmpDir, "vpntoris_gui.py")
	_ = os.WriteFile(scriptPath, guiLinuxScript, 0700)

	cmd := exec.Command("python3", scriptPath)
	cmd.Env = desktopEnv()
	if err := cmd.Start(); err == nil {
		guiCmd = cmd
	}
}
