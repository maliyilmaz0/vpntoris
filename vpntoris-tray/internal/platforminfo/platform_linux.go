//go:build linux

package platforminfo

import "vpntoris-tray/internal/runtimepaths"

func current() Capabilities {
	paths := runtimepaths.Current()
	return Capabilities{
		Platform:          paths.Platform,
		StateDirectory:    paths.StateDirectory,
		InterfaceBackend:  "/dev/net/tun",
		RouteBackend:      "rtnetlink",
		DNSBackend:        "systemd-resolved-or-resolvconf",
		CredentialBackend: "Secret Service",
		PackageFormat:     "deb-rpm-appimage",
	}
}
