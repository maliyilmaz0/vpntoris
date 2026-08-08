package nativehelper

import (
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"vpntoris-tray/internal/fortihelper"
	"vpntoris-tray/internal/runtimepaths"
)

func TestHandleOverUnixSocketRoundTrip(t *testing.T) {
	root := t.TempDir()
	service, err := New(Config{
		EngineRoot: root,
		Router:     memoryRouter{},
		Paths: runtimepaths.Paths{
			RuntimeDirectory: filepath.Join(root, "run"),
			LogDirectory:     filepath.Join(root, "log"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.PrepareRuntime(); err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(t.TempDir(), "h.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan fortihelper.Response, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			t.Errorf("accept: %v", acceptErr)
			return
		}
		defer connection.Close()
		var request fortihelper.Request
		if decodeErr := json.NewDecoder(connection).Decode(&request); decodeErr != nil {
			t.Errorf("decode: %v", decodeErr)
			return
		}
		response := service.Handle(request)
		_ = json.NewEncoder(connection).Encode(response)
		done <- response
	}()
	client, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := json.NewEncoder(client).Encode(fortihelper.Request{Action: fortihelper.ActionStatus, Profile: "profile-demo"}); err != nil {
		t.Fatal(err)
	}
	var response fortihelper.Response
	if err := json.NewDecoder(client).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.State != "stopped" {
		t.Fatalf("response = %#v", response)
	}
	<-done
}
func TestHandleStartFailsWithoutEngineManifest(t *testing.T) {
	service, err := New(Config{EngineRoot: t.TempDir(), Router: memoryRouter{}})
	if err != nil {
		t.Fatal(err)
	}
	response := service.Handle(fortihelper.Request{
		Action:   fortihelper.ActionStart,
		Profile:  "profile-demo",
		Protocol: fortihelper.ProtocolOpenVPN,
		Configuration: `client
dev tun
remote vpn.example.invalid 1194
proto udp
`,
		Username: "user",
		Password: "secret",
		Routes:   []string{"10.0.0.0/8"},
	})
	if response.State != "failed" || response.Error == "" {
		t.Fatalf("expected missing engine failure, got %#v", response)
	}
}
