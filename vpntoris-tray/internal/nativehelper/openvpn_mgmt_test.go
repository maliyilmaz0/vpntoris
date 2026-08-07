package nativehelper

import (
	"runtime"
	"strings"
	"testing"
)

func TestOpenVPNManagementArgsShape(t *testing.T) {
	args, dial, err := openVPNManagementArgs("/tmp/run", "profile-a", "/tmp/run/profile-a.ovpn")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if len(args) < 3 || args[0] != "--management" || args[1] != "127.0.0.1" {
			t.Fatalf("windows args = %#v", args)
		}
		if !strings.HasPrefix(dial, "tcp:127.0.0.1:") {
			t.Fatalf("windows dial = %q", dial)
		}
		return
	}
	if len(args) < 3 || args[0] != "--management" || args[2] != "unix" {
		t.Fatalf("unix args = %#v", args)
	}
	if !strings.HasSuffix(dial, ".sock") {
		t.Fatalf("unix dial = %q", dial)
	}
}
