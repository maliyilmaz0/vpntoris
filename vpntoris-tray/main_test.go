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
	routes, err := parseRoutes("10.68.0.0/16, 192.168.50.0/24")
	if err != nil {
		t.Fatalf("parseRoutes() error: %v", err)
	}
	if len(routes) != 2 || routes[0].network != "10.68.0.0" || routes[0].mask != "255.255.0.0" {
		t.Fatalf("unexpected routes: %#v", routes)
	}
	if _, err := parseRoutes("10.68.0.1"); err == nil {
		t.Fatal("parseRoutes() accepted an address without CIDR prefix")
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
