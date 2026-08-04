//go:build !darwin

package main

func nativeFortiSupported(VPNConfig) bool    { return false }
func nativeHelperReady() bool                { return false }
func nativeFortiConnected(string) bool       { return false }
func nativeFortiInterface(string) string     { return "" }
func nativeFortiConnect(VPNConfig) error     { return nil }
func nativeFortiDisconnect(string) error     { return nil }
func nativeFortiOTP(string, string) error    { return nil }
func nativeFortiLogs(string) ([]byte, error) { return nil, nil }
func nativeFortiTraffic(string) (uint64, uint64, int64, error) {
	return 0, 0, 0, nil
}
