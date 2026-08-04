//go:build !darwin

package main

func nativeIPSecSupported(VPNConfig) bool { return false }
func nativeIPSecConnect(VPNConfig) error  { return nil }
