//go:build darwin

package runtimepaths

import "path/filepath"

func current() Paths {
	runtimeDirectory := "/var/run/vpntoris-native"
	return Paths{
		Platform:         "darwin",
		Architecture:     architecture(),
		StateDirectory:   "/Library/Application Support/VPNToris/State",
		RuntimeDirectory: runtimeDirectory,
		LogDirectory:     "/var/log/vpntoris",
		EngineDirectory:  "/Library/Application Support/VPNToris/Engines",
		HelperSocket:     filepath.Join(runtimeDirectory, "helper.sock"),
		RouterSocket:     "/var/run/vpntoris/router.sock",
	}
}
