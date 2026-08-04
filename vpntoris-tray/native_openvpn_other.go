//go:build !darwin

package main

func nativeOpenVPNSupported(VPNConfig) bool { return false }
func nativeOpenVPNConnect(VPNConfig) error  { return nil }
