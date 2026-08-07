//go:build windows

package runtimepaths

import (
	"os"
	"path/filepath"
)

func current() Paths {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	root := filepath.Join(programData, "VPNToris")
	runtimeDirectory := filepath.Join(root, "Run")
	return Paths{
		Platform:         "windows",
		Architecture:     architecture(),
		StateDirectory:   filepath.Join(root, "State"),
		RuntimeDirectory: runtimeDirectory,
		LogDirectory:     filepath.Join(root, "Logs"),
		EngineDirectory:  filepath.Join(root, "Engines"),
		// Named pipe path used by the future Windows privileged service.
		HelperSocket: `\\.\pipe\vpntoris-native-helper`,
		RouterSocket: "",
	}
}
