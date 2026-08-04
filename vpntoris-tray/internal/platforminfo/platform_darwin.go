//go:build darwin

package platforminfo

func current() Capabilities {
	return Capabilities{Platform: "darwin", StateDirectory: "/Library/Application Support/VPNToris/State", InterfaceBackend: "utun", RouteBackend: "routing-socket", DNSBackend: "SystemConfiguration-scoped-resolver", CredentialBackend: "Keychain", PackageFormat: "pkg"}
}
