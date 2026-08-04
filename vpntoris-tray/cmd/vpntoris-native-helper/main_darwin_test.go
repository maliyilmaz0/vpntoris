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
