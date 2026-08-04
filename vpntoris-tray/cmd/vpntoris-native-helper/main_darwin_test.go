//go:build darwin

package main

import "testing"

func TestPPPReadyPatternIdentifiesOwnedInterface(t *testing.T) {
	data := []byte("INFO: Tunnel established.\nINFO: Interface ppp17 is UP.\n")
	matches := pppReadyPattern.FindSubmatch(data)
	if len(matches) != 2 || string(matches[1]) != "ppp17" {
		t.Fatalf("unexpected match: %#v", matches)
	}
}

func TestPPPReadyPatternRejectsUnrelatedText(t *testing.T) {
	values := [][]byte{
		[]byte("Interface utun4 is UP."),
		[]byte("Interface pppx is UP."),
		[]byte("Interface ppp2 is DOWN."),
	}
	for _, value := range values {
		if pppReadyPattern.Match(value) {
			t.Fatalf("unexpected match: %q", value)
		}
	}
}

func TestUTUNReadyPatternIdentifiesOwnedInterface(t *testing.T) {
	data := []byte("2026-08-04 Opened utun device utun12\n2026-08-04 Initialization Sequence Completed\n")
	matches := utunReadyPattern.FindSubmatch(data)
	if len(matches) != 2 || string(matches[1]) != "utun12" {
		t.Fatalf("unexpected match: %#v", matches)
	}
}

func TestOpenConnectReadyPatternIdentifiesOwnedInterface(t *testing.T) {
	matches := openConnectReadyPattern.FindSubmatch([]byte("VPNTORIS_INTERFACE=utun9\n"))
	if len(matches) != 2 || string(matches[1]) != "utun9" {
		t.Fatalf("unexpected match: %#v", matches)
	}
}

func TestOpenVPNChallengeCredentials(t *testing.T) {
	tests := []struct {
		challenge string
		state     string
		password  string
		otp       string
		expected  string
	}{
		{challenge: "static", password: "test-password", otp: "123456", expected: "SCRV1:dGVzdC1wYXNzd29yZA==:MTIzNDU2"},
		{challenge: "dynamic", state: "opaque-state", password: "discarded", otp: "123456", expected: "CRV1::opaque-state::123456"},
		{challenge: "append", password: "test-password", otp: "123456", expected: "test-password123456"},
	}
	for _, test := range tests {
		username, password := openVPNChallengeCredentials(test.challenge, test.state, "test-user", test.password, test.otp)
		if username != "test-user" || password != test.expected {
			t.Fatalf("challenge %s produced %q and %q", test.challenge, username, password)
		}
	}
}

func TestManagementEscape(t *testing.T) {
	if value := managementEscape(`test\"value`); value != `test\\\"value` {
		t.Fatalf("unexpected escaped value: %q", value)
	}
}
