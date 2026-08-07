//go:build windows

package main

// FortiGate SSL is not part of the Windows MVP (openfortivpn PPP path is deferred).

func nativeFortiSupported(VPNConfig) bool { return false }
func nativeFortiConnect(VPNConfig) error  { return fmtWindowsUnsupported("FortiGate SSL") }
func nativeFortiTraffic(string) (uint64, uint64, int64, error) {
	return 0, 0, 0, fmtWindowsUnsupported("FortiGate SSL")
}
