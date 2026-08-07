//go:build windows

package main

func nativeIPSecSupported(VPNConfig) bool { return false }
func nativeIPSecNeedsOTP(string) bool     { return false }
func nativeIPSecConnect(VPNConfig) error {
	return fmtWindowsUnsupported("IPsec")
}
