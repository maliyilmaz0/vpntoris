//go:build darwin

package main

import "testing"

func TestInterfacePattern(t *testing.T) {
	for _, value := range []string{"utun12", "tun3", "ppp7"} {
		if !interfacePattern.MatchString(value) {
			t.Fatalf("rejected interface %q", value)
		}
	}
	for _, value := range []string{"en0", "utun", "utun1;id", "../utun1"} {
		if interfacePattern.MatchString(value) {
			t.Fatalf("accepted interface %q", value)
		}
	}
}
