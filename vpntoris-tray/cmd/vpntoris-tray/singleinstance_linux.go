//go:build linux

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

func acquireSingleInstance() (func(), error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}
	sockPath := filepath.Join(runtimeDir, fmt.Sprintf("vpntoris-tray-%d.sock", os.Getuid()))

	if conn, err := net.DialTimeout("unix", sockPath, 200*time.Millisecond); err == nil {
		_ = conn.Close()
		return nil, fmt.Errorf("vpntoris-tray is already running")
	}

	_ = os.Remove(sockPath)
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("could not acquire tray instance lock: %w", err)
	}

	cleanup := func() {
		_ = listener.Close()
		_ = os.Remove(sockPath)
	}
	return cleanup, nil
}
