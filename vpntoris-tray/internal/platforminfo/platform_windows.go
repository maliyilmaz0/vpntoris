//go:build windows

package platforminfo

import "os"

func current() Capabilities {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return Capabilities{Platform: "windows", StateDirectory: programData + `\VPNToris\State`, InterfaceBackend: "Wintun", RouteBackend: "IP-Helper-API", DNSBackend: "DNS-Client-NRPT", CredentialBackend: "Credential Manager", PackageFormat: "msi"}
}
