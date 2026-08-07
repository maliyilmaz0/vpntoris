//go:build darwin

package platforminfo

import "vpntoris-tray/internal/runtimepaths"

func current() Capabilities {
	paths := runtimepaths.Current()
	return Capabilities{
		Platform:          paths.Platform,
		StateDirectory:    paths.StateDirectory,
		InterfaceBackend:  "utun",
		RouteBackend:      "routing-socket",
		DNSBackend:        "SystemConfiguration-scoped-resolver",
		CredentialBackend: "Keychain",
		PackageFormat:     "pkg",
	}
}
