//go:build linux

package runtimepaths

import "path/filepath"

func current() Paths {
	runtimeDirectory := "/run/vpntoris-native"
	return Paths{
		Platform:         "linux",
		Architecture:     architecture(),
		StateDirectory:   "/var/lib/vpntoris/state",
		RuntimeDirectory: runtimeDirectory,
		LogDirectory:     "/var/log/vpntoris",
		EngineDirectory:  "/var/lib/vpntoris/engines",
		HelperSocket:     filepath.Join(runtimeDirectory, "helper.sock"),
		RouterSocket:     "/run/vpntoris/router.sock",
	}
}
