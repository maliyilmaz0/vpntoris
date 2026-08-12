package nativehelper

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSpaceFreeBinaryPassthrough(t *testing.T) {
	target := "/usr/lib/vpntoris/engines/linux-amd64/openconnect/bin/vpntoris-vpnc-script"
	got, err := spaceFreeBinary(t.TempDir(), target)
	if err != nil {
		t.Fatal(err)
	}
	if got != target {
		t.Fatalf("expected passthrough, got %q", got)
	}
}

func TestSpaceFreeBinarySymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test requires unix")
	}
	dir := t.TempDir()
	spaced := filepath.Join(dir, "Application Support", "Engines", "bin")
	if err := os.MkdirAll(spaced, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(spaced, "vpntoris-vpnc-script")
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	got, err := spaceFreeBinary(dir, target)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, " ") {
		t.Fatalf("link path still contains a space: %q", got)
	}
	resolved, err := os.Readlink(got)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != target {
		t.Fatalf("link points at %q, want %q", resolved, target)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("link does not resolve: %v", err)
	}
}
