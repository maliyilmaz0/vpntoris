//go:build windows

package main

import (
	"archive/zip"

	"vpntoris-tray/internal/runtimepaths"
)

func writePlatformDiagnostics(archive *zip.Writer) {
	paths := runtimepaths.Current()
	writeDiagnosticCommand(archive, "system.txt", "cmd", "/c", "ver")
	writeDiagnosticCommand(archive, "routes.txt", "route", "print", "-4")
	writeDiagnosticCommand(archive, "interfaces.txt", "netsh", "interface", "ipv4", "show", "interfaces")
	writeDiagnosticFile(archive, "native-paths.txt", []byte(
		"runtime="+paths.RuntimeDirectory+"\r\n"+
			"helper="+paths.HelperSocket+"\r\n"+
			"logs="+paths.LogDirectory+"\r\n"+
			"engines="+paths.EngineDirectory+"\r\n"+
			"state="+paths.StateDirectory+"\r\n",
	))
}
