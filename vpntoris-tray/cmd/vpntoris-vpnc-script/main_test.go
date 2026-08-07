package main

import "testing"

func TestInterfacePattern(t *testing.T) {
	for _, name := range []string{"utun9", "tun0", "ppp17"} {
		if !interfacePattern.MatchString(name) {
			t.Fatalf("expected %q to match", name)
		}
	}
	for _, name := range []string{"eth0", "tun", "utun", "pppX", "../tun0"} {
		if interfacePattern.MatchString(name) {
			t.Fatalf("did not expect %q to match", name)
		}
	}
}
