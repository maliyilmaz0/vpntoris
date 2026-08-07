//go:build windows

package nativehelper

import (
	"os"
	"strconv"
)

func engineEnvironment(userID int) []string {
	systemRoot := os.Getenv("SYSTEMROOT")
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}
	return []string{
		"SystemRoot=" + systemRoot,
		"WINDIR=" + systemRoot,
		"PATH=" + systemRoot + `\System32;` + systemRoot,
		"VPNTORIS_USER_UID=" + strconv.Itoa(userID),
	}
}

func openVPNDeviceType() string {
	// OpenVPN for Windows commonly uses the wintun or tap-windows6 backend.
	// "tun" selects the modern wintun path when the DLL is available beside openvpn.
	return "tun"
}
