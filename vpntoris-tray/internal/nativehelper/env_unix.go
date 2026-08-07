//go:build unix

package nativehelper

import "strconv"

func engineEnvironment(userID int) []string {
	return []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LANG=C", "LC_ALL=C", "VPNTORIS_USER_UID=" + strconv.Itoa(userID)}
}

func openVPNDeviceType() string {
	return "tun"
}
