//go:build linux

package netbackend

import (
	"strings"
	"testing"
)

func TestApplyLinuxSplitDNS(t *testing.T) {
	var calls []string
	run := func(name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	if err := ApplyLinuxSplitDNS(run, "tun0", "corp.example.com", []string{"10.0.0.53"}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %#v", calls)
	}
	if calls[0] != "resolvectl dns tun0 10.0.0.53" && !strings.HasPrefix(calls[0], "resolvectl dns tun0 ") {
		t.Fatalf("dns call = %q", calls[0])
	}
	if calls[0] != "resolvectl dns tun0 10.0.0.53" {
		t.Fatalf("dns call = %q", calls[0])
	}
	if calls[1] != "resolvectl domain tun0 ~corp.example.com" {
		t.Fatalf("domain call = %q", calls[1])
	}
}
