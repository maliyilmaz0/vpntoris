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
	return "tun"
}
