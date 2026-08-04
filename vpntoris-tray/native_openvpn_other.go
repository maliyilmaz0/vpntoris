//go:build !darwin

package main

func nativeOpenVPNSupported(VPNConfig) bool { return false }
func nativeOpenVPNConnect(VPNConfig) error  { return nil }
func nativeOpenVPNNeedsOTP(string) bool     { return false }
