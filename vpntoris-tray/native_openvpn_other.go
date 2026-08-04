//go:build !darwin

package main

func nativeOpenVPNSupported(VPNConfig) bool { return false }
func nativeOpenVPNConnect(VPNConfig) error  { return nil }
func nativeOpenVPNNeedsOTP(string) bool     { return false }
func nativeOpenVPNTraffic(string) (uint64, uint64, int64, error) {
	return 0, 0, 0, nil
}
