package nativehelper

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"vpntoris-tray/internal/fortihelper"
	"vpntoris-tray/internal/runtimepaths"
)

func TestOpenConnectSessionConnectStop(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	runProtocolSession(t, protocolFixture{
		engineID:   "openconnect",
		sourceDir:  "testdata/fake_openconnect",
		protocol:   fortihelper.ProtocolOpenConnect,
		iface:      "tun8",
		executable: "openconnect",
		helpers:    []string{"vpntoris-vpnc-script"},
		request: fortihelper.Request{
			Action:          fortihelper.ActionStart,
			Profile:         "profile-oc",
			Protocol:        fortihelper.ProtocolOpenConnect,
			GatewayProtocol: "anyconnect",
			Host:            "vpn.example.invalid",
			Port:            443,
			Username:        "test-user",
			Password:        "test-password",
			Routes:          []string{"10.20.0.0/16"},
		},
	})
}
func TestFortiGateSSLSessionConnectStop(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	runProtocolSession(t, protocolFixture{
		engineID:   "openfortivpn",
		sourceDir:  "testdata/fake_openfortivpn",
		protocol:   fortihelper.ProtocolFortiGateSSL,
		iface:      "ppp17",
		executable: "openfortivpn",
		request: fortihelper.Request{
			Action:      fortihelper.ActionStart,
			Profile:     "profile-forti",
			Protocol:    fortihelper.ProtocolFortiGateSSL,
			Host:        "vpn.example.invalid",
			Port:        443,
			Username:    "test-user",
			Password:    "test-password",
			TrustedCert: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			Routes:      []string{"10.30.0.0/16"},
		},
	})
}

type protocolFixture struct {
	engineID   string
	sourceDir  string
	protocol   string
	iface      string
	executable string
	helpers    []string
	request    fortihelper.Request
}

func runProtocolSession(t *testing.T, fixture protocolFixture) {
	t.Helper()
	fakeBinary := buildFakeEngine(t, fixture.sourceDir, fixture.executable)
	engineRoot := t.TempDir()
	engineDir := filepath.Join(engineRoot, fixture.engineID)
	if err := os.MkdirAll(filepath.Join(engineDir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	enginePath := filepath.Join(engineDir, "bin", fixture.executable)
	data, err := os.ReadFile(fakeBinary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(enginePath, data, 0755); err != nil {
		t.Fatal(err)
	}
	for _, helper := range fixture.helpers {
		if err := os.WriteFile(filepath.Join(engineDir, "bin", helper), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	digest, err := fileSHA256(enginePath)
	if err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"id": fixture.engineID, "protocol": fixture.protocol, "version": "fake-1",
		"os": runtime.GOOS, "architecture": runtime.GOARCH,
		"executable": fixture.engineID + "/bin/" + fixture.executable,
		"sha256":     digest, "license": "test", "capabilities": []string{"split-route"},
	}
	manifestData, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(engineDir, "manifest.json"), manifestData, 0644); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp("/tmp", "vte")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	router := &recordingRouter{}
	service, err := New(Config{
		EngineRoot: engineRoot,
		UserID:     -1,
		Router:     router,
		Paths: runtimepaths.Paths{
			RuntimeDirectory: filepath.Join(root, "r"),
			LogDirectory:     filepath.Join(root, "l"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.PrepareRuntime(); err != nil {
		t.Fatal(err)
	}
	previous := lookupInterface
	lookupInterface = func(name string) bool { return name == fixture.iface }
	defer func() { lookupInterface = previous }()
	start := service.Start(fixture.request)
	if start.State != "connecting" && start.State != "waiting-otp" {
		t.Fatalf("start = %#v", start)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		status := service.Status(fixture.request.Profile)
		if status.State == "connected" && status.Interface == fixture.iface {
			break
		}
		if status.State == "failed" {
			logData, _ := os.ReadFile(service.paths.ProfileLog(fixture.request.Profile))
			t.Fatalf("failed: %#v\nlog:\n%s", status, logData)
		}
		time.Sleep(50 * time.Millisecond)
	}
	status := service.Status(fixture.request.Profile)
	if status.State != "connected" || status.Interface != fixture.iface {
		t.Fatalf("expected connected %s, got %#v", fixture.iface, status)
	}
	if len(router.added) == 0 {
		t.Fatal("expected routes to be added")
	}
	if stop := service.Stop(fixture.request.Profile); stop.State != "stopped" {
		t.Fatalf("stop = %#v", stop)
	}
	time.Sleep(150 * time.Millisecond)
	if len(router.deleted) == 0 {
		t.Fatal("expected routes to be deleted")
	}
}
func buildFakeEngine(t *testing.T, sourceDir, name string) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), name)
	command := exec.Command("go", "build", "-o", output, ".")
	command.Dir = sourceDir
	command.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, out)
	}
	return output
}
