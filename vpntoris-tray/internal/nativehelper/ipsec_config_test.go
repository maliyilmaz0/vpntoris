package nativehelper

import (
	"runtime"
	"strings"
	"testing"
)

func TestBuildCharonConfigurationIncludesPlatformKernelPlugins(t *testing.T) {
	configuration := buildCharonConfiguration("/opt/plugins", "/run/charon.vici", "/run/charon.pid")
	if !strings.Contains(configuration, "pid_file = /run/charon.pid") {
		t.Fatalf("missing pid file: %s", configuration)
	}
	if !strings.Contains(configuration, "socket = unix:///run/charon.vici") {
		t.Fatalf("missing vici socket: %s", configuration)
	}
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(configuration, "kernel-pfkey") || !strings.Contains(configuration, "kernel-pfroute") {
			t.Fatalf("expected darwin kernel plugins, got %s", configuration)
		}
		if strings.Contains(configuration, "kernel-netlink") {
			t.Fatal("darwin config should not load kernel-netlink")
		}
	case "linux":
		if !strings.Contains(configuration, "kernel-netlink") {
			t.Fatalf("expected linux kernel-netlink, got %s", configuration)
		}
		if strings.Contains(configuration, "kernel-pfkey") {
			t.Fatal("linux config should not load kernel-pfkey")
		}
	}
}
