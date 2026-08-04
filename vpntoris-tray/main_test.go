package main

import (
	"strings"
	"testing"
)

func TestContainerName(t *testing.T) {
	tests := map[string]string{
		"Office VPN":     "vpntoris-office-vpn",
		"  Demo / Test ": "vpntoris-demo-test",
		"":               "vpntoris-vpn",
	}
	for input, want := range tests {
		if got := containerName(input); got != want {
			t.Errorf("containerName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestContainerNameDoesNotAllowDockerNameInjection(t *testing.T) {
	got := containerName("../../VPN; docker rm")
	if strings.ContainsAny(got, "/; ") {
		t.Fatalf("unsafe container name: %q", got)
	}
}

func TestParseRoutes(t *testing.T) {
	routes, err := parseRoutes("198.51.100.0/24, 203.0.113.0/24")
	if err != nil {
		t.Fatalf("parseRoutes() error: %v", err)
	}
	if len(routes) != 2 || routes[0].network != "198.51.100.0" || routes[0].mask != "255.255.255.0" {
		t.Fatalf("unexpected routes: %#v", routes)
	}
	if _, err := parseRoutes("198.51.100.1"); err == nil {
		t.Fatal("parseRoutes() accepted an address without CIDR prefix")
	}
}

func TestParseByteSize(t *testing.T) {
	tests := map[string]uint64{"12B": 12, "1.5kB": 1500, "2MiB": 2 * 1024 * 1024, "3.2GB": 3200000000}
	for input, expected := range tests {
		value, err := parseByteSize(input)
		if err != nil || value != expected {
			t.Fatalf("parseByteSize(%q) = %d, %v; want %d", input, value, err, expected)
		}
	}
}

func TestBuildIPSecProposals(t *testing.T) {
	got, err := buildProposals("aes256,aes256gcm16", "sha256", "prfsha256", "14,20", true)
	if err != nil {
		t.Fatal(err)
	}
	want := "aes256-sha256-prfsha256-modp2048,aes256-sha256-prfsha256-ecp384,aes256gcm16-prfsha256-modp2048,aes256gcm16-prfsha256-ecp384"
	if got != want {
		t.Fatalf("proposals = %q, want %q", got, want)
	}
}

func TestRenderSwanctlConfig(t *testing.T) {
	profile := VPNConfig{Name: "office", Type: "ipsec", Host: "vpn.example.com", User: "alice", Password: "login-secret", Routes: "10.68.0.0/16", IPSec: &IPSecConfig{
		IKEVersion: 2, AuthMode: "eap", PreSharedKey: "shared-secret", ModeConfig: true,
		IKEEncryption: "aes256", IKEIntegrity: "sha256", IKEPRF: "prfsha256", DHGroups: "20",
		ESPEncryption: "aes256gcm16", PFS: true, PFSGroups: "20", LocalSelectors: "0.0.0.0/0",
	}}
	configuration, err := renderSwanctlConfig(profile)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"version = 2", "aes256-sha256-prfsha256-ecp384", "local_ts = dynamic", "remote_ts = 0.0.0.0/0", "auth = eap"} {
		if !strings.Contains(configuration, expected) {
			t.Errorf("missing %q in configuration", expected)
		}
	}
}

func TestOpenConnectProtocolDefaultsAndValidation(t *testing.T) {
	if value := openConnectProtocol(VPNConfig{Type: "openconnect"}); value != "anyconnect" {
		t.Fatalf("unexpected default protocol: %s", value)
	}
	if value := openConnectProtocol(VPNConfig{Type: "openconnect", OpenConnectProtocol: "gp"}); value != "gp" {
		t.Fatalf("unexpected selected protocol: %s", value)
	}
	if value := openConnectProtocol(VPNConfig{Type: "openconnect", OpenConnectProtocol: "invalid"}); value != "" {
		t.Fatalf("accepted unsupported protocol: %s", value)
	}
}

func TestSanitizeDiagnosticText(t *testing.T) {
	secrets := []string{"super-" + "secret", "another-" + "secret", "visible-" + "token", "123" + "456"}
	input := "password=" + secrets[0] + " psk:" + secrets[1] + " --token " + secrets[2] + " otp " + secrets[3] + " safe=value"
	output := sanitizeDiagnosticText(input)
	for _, secret := range secrets {
		if strings.Contains(output, secret) {
			t.Fatalf("diagnostic output contains %q: %s", secret, output)
		}
	}
	if !strings.Contains(output, "safe=value") {
		t.Fatalf("unrelated diagnostic content was removed: %s", output)
	}
}

func TestGatewayFailoverState(t *testing.T) {
	previousPath := configPath
	configPath = t.TempDir() + "/configs.json"
	defer func() { configPath = previousPath }()
	gatewayState.Lock()
	gatewayState.loaded = false
	gatewayState.items = make(map[string]gatewayRecord)
	gatewayState.Unlock()
	profile := VPNConfig{Name: "office", Host: "vpn-a.example.com", BackupGateways: "vpn-b.example.com\nvpn-c.example.com, vpn-b.example.com", FailoverLimit: 2}
	gateways := gatewayCandidates(profile)
	if strings.Join(gateways, ",") != "vpn-a.example.com,vpn-b.example.com,vpn-c.example.com" {
		t.Fatalf("unexpected gateway list: %v", gateways)
	}
	if next := setGatewayResult(profile, gateways[0], false, false); next != gateways[0] {
		t.Fatalf("gateway rotated before threshold: %s", next)
	}
	if next := setGatewayResult(profile, gateways[0], false, false); next != gateways[1] {
		t.Fatalf("gateway did not rotate at threshold: %s", next)
	}
	if got := orderedGateways(profile); strings.Join(got, ",") != "vpn-b.example.com,vpn-c.example.com,vpn-a.example.com" {
		t.Fatalf("unexpected persisted gateway order: %v", got)
	}
}

func TestOverrideOpenVPNRemote(t *testing.T) {
	configuration := "client\nremote vpn-a.example.com 1194 udp\nremote vpn-b.example.com 443 tcp\nauth-user-pass\n"
	result := overrideOpenVPNRemote(configuration, "vpn-c.example.com", "1194")
	if strings.Count(result, "remote ") != 1 || !strings.Contains(result, "remote vpn-c.example.com 1194 udp") {
		t.Fatalf("unexpected OpenVPN gateway override: %s", result)
	}
	if !strings.Contains(result, "auth-user-pass") {
		t.Fatalf("OpenVPN configuration content was lost: %s", result)
	}
}

func TestDockerCommandIncludesCredentialHelper(t *testing.T) {
	command := dockerCommand("version")
	path := ""
	for _, value := range command.Env {
		if strings.HasPrefix(value, "PATH=") {
			path = strings.TrimPrefix(value, "PATH=")
			break
		}
	}
	if !strings.Contains(path, "/Applications/Docker.app/Contents/Resources/bin") {
		t.Fatalf("Docker credential helper directory missing from PATH: %s", path)
	}
}
