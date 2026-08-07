package main

import "testing"

func TestNativeProfileIDIsStableAndOpaque(t *testing.T) {
	first := nativeProfileID("Office VPN")
	second := nativeProfileID("Office VPN")
	if first != second {
		t.Fatalf("profile id is not stable: %q vs %q", first, second)
	}
	if first == "Office VPN" || len(first) < 10 {
		t.Fatalf("profile id should be opaque hash, got %q", first)
	}
	if nativeProfileID("Other") == first {
		t.Fatal("different profile names produced the same id")
	}
}
