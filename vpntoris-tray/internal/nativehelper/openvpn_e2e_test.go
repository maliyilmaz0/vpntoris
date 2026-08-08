package nativehelper

import (
	"crypto/sha256"
	"encoding/hex"
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

func TestOpenVPNSessionConnectStopResetsRoutes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping openvpn e2e in short mode")
	}
	fakeBinary := buildFakeOpenVPN(t)
	engineRoot := t.TempDir()
	openvpnDir := filepath.Join(engineRoot, "openvpn")
	if err := os.MkdirAll(filepath.Join(openvpnDir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	enginePath := filepath.Join(openvpnDir, "bin", "openvpn")
	if data, err := os.ReadFile(fakeBinary); err != nil {
		t.Fatal(err)
	} else if err := os.WriteFile(enginePath, data, 0755); err != nil {
		t.Fatal(err)
	}
	digest, err := fileSHA256(enginePath)
	if err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"id":           "openvpn",
		"protocol":     "openvpn",
		"version":      "fake-1",
		"os":           runtime.GOOS,
		"architecture": runtime.GOARCH,
		"executable":   "openvpn/bin/openvpn",
		"sha256":       digest,
		"license":      "test",
		"capabilities": []string{"tun", "userpass", "challenge", "split-route"},
	}
	manifestData, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(openvpnDir, "manifest.json"), manifestData, 0644); err != nil {
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
	previousLookup := lookupInterface
	lookupInterface = func(name string) bool { return name == "tun9" }
	defer func() { lookupInterface = previousLookup }()
	start := service.Start(fortihelper.Request{
		Action:   fortihelper.ActionStart,
		Profile:  "profile-office",
		Protocol: fortihelper.ProtocolOpenVPN,
		Configuration: `client
dev tun
remote 203.0.113.10 1194
proto udp
`,
		Username: "test-user",
		Password: "test-password",
		Routes:   []string{"10.38.0.0/16", "10.68.236.0/24"},
	})
	if start.State != "connecting" {
		t.Fatalf("start = %#v", start)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		status := service.Status("profile-office")
		if status.State == "connected" && status.Interface == "tun9" {
			break
		}
		if status.State == "failed" {
			logPath := service.paths.ProfileLog("profile-office")
			logData, _ := os.ReadFile(logPath)
			t.Fatalf("session failed: %#v\nlog:\n%s", status, logData)
		}
		time.Sleep(50 * time.Millisecond)
	}
	status := service.Status("profile-office")
	if status.State != "connected" || status.Interface != "tun9" {
		t.Fatalf("expected connected tun9, got %#v", status)
	}
	if len(router.added) != 2 {
		t.Fatalf("expected two routes added, got %#v", router.added)
	}
	if stop := service.Stop("profile-office"); stop.State != "stopped" {
		t.Fatalf("stop = %#v", stop)
	}
	time.Sleep(200 * time.Millisecond)
	if len(router.deleted) == 0 {
		t.Fatalf("expected routes to be deleted on stop, got %#v", router.deleted)
	}
	if reset := service.Reset(); reset.State != "stopped" {
		t.Fatalf("reset = %#v", reset)
	}
	if got := service.Status("profile-office"); got.State != "stopped" {
		t.Fatalf("status after reset = %#v", got)
	}
}
func buildFakeOpenVPN(t *testing.T) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "fake-openvpn")
	source := filepath.Join("testdata", "fake_openvpn")
	command := exec.Command("go", "build", "-o", output, ".")
	command.Dir = source
	command.Env = append(os.Environ(), "CGO_ENABLED=0")
	if outputBytes, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build fake openvpn: %v\n%s", err, outputBytes)
	}
	return output
}
func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
