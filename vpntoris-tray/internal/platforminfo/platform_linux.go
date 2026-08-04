//go:build linux

package platforminfo

func current() Capabilities {
	return Capabilities{Platform: "linux", StateDirectory: "/var/lib/vpntoris/state", InterfaceBackend: "/dev/net/tun", RouteBackend: "rtnetlink", DNSBackend: "systemd-resolved-or-resolvconf", CredentialBackend: "Secret Service", PackageFormat: "deb-rpm-appimage"}
}
