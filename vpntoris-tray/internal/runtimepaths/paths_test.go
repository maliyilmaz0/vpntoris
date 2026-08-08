package runtimepaths

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCurrentHasRequiredFields(t *testing.T) {
	paths := Current()
	if paths.Platform != runtime.GOOS {
		t.Fatalf("platform = %q, want %q", paths.Platform, runtime.GOOS)
	}
	if paths.Architecture != runtime.GOARCH {
		t.Fatalf("architecture = %q, want %q", paths.Architecture, runtime.GOARCH)
	}
	for name, value := range map[string]string{
		"StateDirectory":   paths.StateDirectory,
		"RuntimeDirectory": paths.RuntimeDirectory,
		"LogDirectory":     paths.LogDirectory,
		"EngineDirectory":  paths.EngineDirectory,
		"HelperSocket":     paths.HelperSocket,
	} {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("%s is empty", name)
		}
	}
}
func TestEngineBundleUsesGOOSAndGOARCH(t *testing.T) {
	bundle := EngineBundle("/opt/vpntoris/engines")
	expected := filepath.Join("/opt/vpntoris/engines", runtime.GOOS+"-"+runtime.GOARCH)
	if bundle != expected {
		t.Fatalf("EngineBundle = %q, want %q", bundle, expected)
	}
}
func TestProfileLog(t *testing.T) {
	paths := Paths{LogDirectory: "/var/log/vpntoris"}
	got := paths.ProfileLog("profile-abc")
	want := filepath.Join("/var/log/vpntoris", "profile-abc.log")
	if got != want {
		t.Fatalf("ProfileLog = %q, want %q", got, want)
	}
}
func TestHelperSocketIsUnderRuntimeDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		paths := Current()
		if !strings.Contains(paths.HelperSocket, `pipe`) {
			t.Fatalf("windows helper socket should be a named pipe, got %q", paths.HelperSocket)
		}
		return
	}
	paths := Current()
	if !strings.HasPrefix(paths.HelperSocket, paths.RuntimeDirectory) {
		t.Fatalf("helper socket %q not under runtime %q", paths.HelperSocket, paths.RuntimeDirectory)
	}
}
