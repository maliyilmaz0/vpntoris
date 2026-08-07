//go:build windows

package main

func nativeOpenConnectSupported(VPNConfig) bool { return false }
func nativeOpenConnectNeedsOTP(string) bool     { return false }
func nativeOpenConnectConnect(VPNConfig) error {
	return fmtWindowsUnsupported("OpenConnect")
}
func nativeOpenConnectTraffic(string) (uint64, uint64, int64, error) {
	return 0, 0, 0, fmtWindowsUnsupported("OpenConnect")
}
