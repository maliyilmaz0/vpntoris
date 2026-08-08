//go:build linux

package main

import (
	"archive/zip"
	"os/exec"
	"vpntoris-tray/internal/runtimepaths"
)

func writePlatformDiagnostics(archive *zip.Writer) {
	paths := runtimepaths.Current()
	writeDiagnosticCommand(archive, "system.txt", "uname", "-a")
	writeDiagnosticCommand(archive, "routes.txt", "ip", "-4", "route", "show")
	writeDiagnosticCommand(archive, "interfaces.txt", "ip", "-brief", "link")
	if _, err := exec.LookPath("resolvectl"); err == nil {
		writeDiagnosticCommand(archive, "dns.txt", "resolvectl", "status")
	} else {
		writeDiagnosticCommand(archive, "dns.txt", "cat", "/etc/resolv.conf")
	}
	writeDiagnosticFile(archive, "native-paths.txt", []byte(
		"runtime="+paths.RuntimeDirectory+"\n"+
			"helper="+paths.HelperSocket+"\n"+
			"logs="+paths.LogDirectory+"\n"+
			"engines="+paths.EngineDirectory+"\n"+
			"state="+paths.StateDirectory+"\n",
	))
}
