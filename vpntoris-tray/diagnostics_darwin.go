//go:build darwin

package main

import (
	"archive/zip"
	"path/filepath"

	"vpntoris-tray/internal/runtimepaths"
)

func writePlatformDiagnostics(archive *zip.Writer) {
	paths := runtimepaths.Current()
	writeDiagnosticCommand(archive, "system.txt", "/usr/bin/uname", "-a")
	writeDiagnosticCommand(archive, "routes.txt", "/usr/sbin/netstat", "-rn", "-f", "inet")
	writeDiagnosticCommand(archive, "dns.txt", "/usr/sbin/scutil", "--dns")
	writeDiagnosticCommand(archive, "route-helper-launchd.txt", "/bin/launchctl", "print", "system/com.vpntoris.router")
	writeDiagnosticCommand(archive, "route-helper-socket.txt", "/usr/bin/stat", "-f", "%Sp %Su:%Sg %N",
		filepath.Dir(paths.RouterSocket),
		paths.RouterSocket,
		"/Library/PrivilegedHelperTools/com.vpntoris.router",
		"/Library/PrivilegedHelperTools/com.vpntoris.tun2socks",
	)
	writeDiagnosticCommand(archive, "native-helper-socket.txt", "/usr/bin/stat", "-f", "%Sp %Su:%Sg %N",
		paths.RuntimeDirectory,
		paths.HelperSocket,
	)
}
