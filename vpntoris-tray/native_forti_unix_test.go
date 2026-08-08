//go:build unix

package main

import "testing"

func TestNativeHelperReadyDoesNotPanic(t *testing.T) {
	_ = nativeHelperReady()
}
func TestNativeFortiLogsMissingProfile(t *testing.T) {
	_, err := nativeFortiLogs("unit-test-missing-profile")
	if err == nil {
		return
	}
}
