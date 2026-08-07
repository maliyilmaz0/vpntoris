//go:build windows

package platforminfo

import "vpntoris-tray/internal/runtimepaths"

func current() Capabilities {
	paths := runtimepaths.Current()
	return Capabilities{
		Platform:          paths.Platform,
		StateDirectory:    paths.StateDirectory,
		InterfaceBackend:  "Wintun",
		RouteBackend:      "IP-Helper-API",
		DNSBackend:        "DNS-Client-NRPT",
		CredentialBackend: "Credential Manager",
		PackageFormat:     "msi",
	}
}
