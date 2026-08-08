package nativehelper

import (
	"os"
	"path/filepath"
	"testing"
	"vpntoris-tray/internal/fortihelper"
	"vpntoris-tray/internal/runtimepaths"
)

func TestPPPReadyPatternIdentifiesOwnedInterface(t *testing.T) {
	data := []byte("INFO: Tunnel established.\nINFO: Interface ppp17 is UP.\n")
	if name := InterfaceFromLogData(data, fortihelper.ProtocolFortiGateSSL); name != "ppp17" {
		t.Fatalf("got %q", name)
	}
}
func TestPPPReadyPatternRejectsUnrelatedText(t *testing.T) {
	values := [][]byte{
		[]byte("Interface utun4 is UP."),
		[]byte("Interface pppx is UP."),
		[]byte("Interface ppp2 is DOWN."),
	}
	for _, value := range values {
		if name := InterfaceFromLogData(value, fortihelper.ProtocolFortiGateSSL); name != "" {
			t.Fatalf("unexpected match %q from %q", name, value)
		}
	}
}
func TestOpenVPNReadyPatterns(t *testing.T) {
	tests := []struct {
		log  string
		want string
	}{
		{"2026-08-04 Opened utun device utun12\nInitialization Sequence Completed\n", "utun12"},
		{"TUN/TAP device tun0 opened\nInitialization Sequence Completed\n", "tun0"},
		{"Opened tun device tun3\n", "tun3"},
		{"TAP-WIN32 device [Local Area Connection 2] opened\n", "Local Area Connection 2"},
		{"Opened tun device [OpenVPN Data Channel Offload]\n", "OpenVPN Data Channel Offload"},
	}
	for _, test := range tests {
		if name := InterfaceFromLogData([]byte(test.log), fortihelper.ProtocolOpenVPN); name != test.want {
			t.Fatalf("log %q => %q, want %q", test.log, name, test.want)
		}
	}
}
func TestOpenConnectReadyPatternIdentifiesOwnedInterface(t *testing.T) {
	if name := InterfaceFromLogData([]byte("VPNTORIS_INTERFACE=utun9\n"), fortihelper.ProtocolOpenConnect); name != "utun9" {
		t.Fatalf("got %q", name)
	}
	if name := InterfaceFromLogData([]byte("VPNTORIS_INTERFACE=tun2\n"), fortihelper.ProtocolOpenConnect); name != "tun2" {
		t.Fatalf("got %q", name)
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
func TestNewRequiresEngineRoot(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected missing engine root to fail")
	}
}
func TestPrepareRuntimeCreatesDirectories(t *testing.T) {
	root := t.TempDir()
	logDirectory := filepath.Join(root, "log")
	runtimeDirectory := filepath.Join(root, "run")
	service, err := New(Config{
		EngineRoot: root,
		Router:     memoryRouter{},
		Paths: runtimepaths.Paths{
			RuntimeDirectory: runtimeDirectory,
			LogDirectory:     logDirectory,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.PrepareRuntime(); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{logDirectory, runtimeDirectory} {
		info, statErr := os.Stat(directory)
		if statErr != nil || !info.IsDir() {
			t.Fatalf("directory missing: %s (%v)", directory, statErr)
		}
	}
	if got := service.paths.ProfileLog("profile-test"); got != filepath.Join(logDirectory, "profile-test.log") {
		t.Fatalf("ProfileLog = %q", got)
	}
}
func TestHandleRejectsInvalidRequest(t *testing.T) {
	service, err := New(Config{EngineRoot: t.TempDir(), Router: memoryRouter{}})
	if err != nil {
		t.Fatal(err)
	}
	response := service.Handle(fortihelper.Request{Action: fortihelper.ActionStart, Profile: "Bad Name"})
	if response.State != "failed" || response.Error == "" {
		t.Fatalf("expected validation failure, got %#v", response)
	}
}
func TestStopAndStatusForUnknownProfile(t *testing.T) {
	service, err := New(Config{EngineRoot: t.TempDir(), Router: memoryRouter{}})
	if err != nil {
		t.Fatal(err)
	}
	if response := service.Status("missing"); response.State != "stopped" {
		t.Fatalf("status = %#v", response)
	}
	if response := service.Stop("missing"); response.State != "stopped" {
		t.Fatalf("stop = %#v", response)
	}
}
func TestResetClearsSessions(t *testing.T) {
	service, err := New(Config{EngineRoot: t.TempDir(), Router: memoryRouter{}})
	if err != nil {
		t.Fatal(err)
	}
	service.sessions["profile-a"] = &session{state: "stopped", request: fortihelper.Request{Profile: "profile-a"}}
	if response := service.Reset(); response.State != "stopped" {
		t.Fatalf("reset = %#v", response)
	}
	if len(service.sessions) != 0 {
		t.Fatalf("sessions not cleared: %#v", service.sessions)
	}
}
func TestSendOTPRequiresWaitingSession(t *testing.T) {
	service, err := New(Config{EngineRoot: t.TempDir(), Router: memoryRouter{}})
	if err != nil {
		t.Fatal(err)
	}
	response := service.SendOTP(fortihelper.Request{Action: fortihelper.ActionOTP, Profile: "profile-a", OTP: "123456"})
	if response.State != "failed" {
		t.Fatalf("expected failure, got %#v", response)
	}
}
func TestStopDeletesOwnedRoutes(t *testing.T) {
	router := &recordingRouter{}
	service, err := New(Config{EngineRoot: t.TempDir(), Router: router})
	if err != nil {
		t.Fatal(err)
	}
	service.sessions["profile-a"] = &session{
		state:         "connected",
		interfaceName: "tun0",
		request: fortihelper.Request{
			Profile:  "profile-a",
			Protocol: fortihelper.ProtocolOpenVPN,
			Routes:   []string{"10.0.0.0/8"},
		},
	}
	if response := service.Stop("profile-a"); response.State != "stopped" {
		t.Fatalf("stop = %#v", response)
	}
	if len(router.deleted) != 1 || router.deleted[0] != "tun0:10.0.0.0/8" {
		t.Fatalf("deleted = %#v", router.deleted)
	}
}

type memoryRouter struct{}

func (memoryRouter) AddRoutes(string, []string) error { return nil }
func (memoryRouter) DeleteRoutes(string, []string)    {}

type recordingRouter struct {
	added   []string
	deleted []string
}

func (router *recordingRouter) AddRoutes(interfaceName string, routes []string) error {
	for _, route := range routes {
		router.added = append(router.added, interfaceName+":"+route)
	}
	return nil
}
func (router *recordingRouter) DeleteRoutes(interfaceName string, routes []string) {
	for _, route := range routes {
		router.deleted = append(router.deleted, interfaceName+":"+route)
	}
}
