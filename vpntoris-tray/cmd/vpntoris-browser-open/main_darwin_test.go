//go:build darwin

package main

import "testing"

func TestAllowedBrowserURL(t *testing.T) {
	for _, value := range []string{"https://login.example.invalid/sso", "http://127.0.0.1:29786/callback", "http://[::1]:29786/callback"} {
		if !allowedBrowserURL(value) {
			t.Fatalf("rejected URL %q", value)
		}
	}
	for _, value := range []string{"http://login.example.invalid", "file:///tmp/token", "https://user:pass@example.invalid"} {
		if allowedBrowserURL(value) {
			t.Fatalf("accepted URL %q", value)
		}
	}
}
