package fortihelper

import (
	"strings"
	"testing"
)

func TestStartRequestIsStrictlyScoped(t *testing.T) {
	request := Request{Action: ActionStart, Profile: "test-profile", Host: "vpn.example.invalid", Port: 443, Username: "test-user", Password: "test-password", Routes: []string{"198.51.100.42/32"}}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	expected := []string{"vpn.example.invalid:443", "--username=test-user", "--no-routes", "--no-dns", "--pppd-no-peerdns", "--pppd-ipparam=vpntoris-test-profile"}
	arguments := request.Arguments()
	if len(arguments) != len(expected) {
		t.Fatalf("arguments = %#v", arguments)
	}
	for index := range expected {
		if arguments[index] != expected[index] {
			t.Fatalf("argument %d = %q", index, arguments[index])
		}
	}
}

func TestStartRequestRejectsUnsafeValues(t *testing.T) {
	tests := []Request{
		{Action: ActionStart, Profile: "Invalid Profile", Host: "vpn.example.invalid", Port: 443, Username: "test-user", Password: "test-password", Routes: []string{"198.51.100.42/32"}},
		{Action: ActionStart, Profile: "test-profile", Host: "vpn.example.invalid;id", Port: 443, Username: "test-user", Password: "test-password", Routes: []string{"198.51.100.42/32"}},
		{Action: ActionStart, Profile: "test-profile", Host: "vpn.example.invalid", Port: 443, Username: "test-user", Password: "test-password", Routes: []string{"0.0.0.0/0"}},
		{Action: ActionStart, Profile: "test-profile", Host: "vpn.example.invalid", Port: 443, Username: "test-user", Password: "test-password", Routes: []string{"198.51.100.7/24"}},
		{Action: ActionStart, Profile: "test-profile", Host: "vpn.example.invalid", Port: 443, Username: "test-user\nroot", Password: "test-password", Routes: []string{"198.51.100.42/32"}},
	}
	for _, request := range tests {
		if err := request.Validate(); err == nil {
			t.Fatalf("accepted unsafe request: %#v", request)
		}
	}
}

func TestOTPRequestDoesNotAcceptLineInjection(t *testing.T) {
	if err := (Request{Action: ActionOTP, Profile: "test-profile", OTP: "123456"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Request{Action: ActionOTP, Profile: "test-profile", OTP: "123456\nsecret"}).Validate(); err == nil {
		t.Fatal("accepted line injection")
	}
}

func TestOpenVPNStartAcceptsSanitizedConfiguration(t *testing.T) {
	request := Request{Action: ActionStart, Profile: "test-profile", Protocol: ProtocolOpenVPN, Configuration: "client\ndev tun\nremote vpn.example.invalid 1194\n", Username: "test-user", Password: "test-password", Routes: []string{"198.51.100.42/32"}}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenVPNStartRejectsExecutableConfiguration(t *testing.T) {
	request := Request{Action: ActionStart, Profile: "test-profile", Protocol: ProtocolOpenVPN, Configuration: "client\ndev tun\nremote vpn.example.invalid 1194\nup /tmp/script\n", Routes: []string{"198.51.100.42/32"}}
	if err := request.Validate(); err == nil {
		t.Fatal("accepted executable OpenVPN configuration")
	}
}

func TestOpenConnectStartValidatesGatewayProtocol(t *testing.T) {
	request := Request{Action: ActionStart, Profile: "test-profile", Protocol: ProtocolOpenConnect, GatewayProtocol: "gp", Host: "vpn.example.invalid", Port: 443, Username: "test-user", Password: "test-password", Routes: []string{"198.51.100.42/32"}}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	request.GatewayProtocol = "unsupported"
	if err := request.Validate(); err == nil {
		t.Fatal("accepted unsupported OpenConnect protocol")
	}
}

func TestOpenConnectBrowserAuthenticationDoesNotRequireStoredCredentials(t *testing.T) {
	request := Request{Action: ActionStart, Profile: "test-profile", Protocol: ProtocolOpenConnect, GatewayProtocol: "gp", ExternalBrowser: true, Host: "vpn.example.invalid", Port: 443, Routes: []string{"198.51.100.42/32"}}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestIPSecConfigurationPreservesExplicitSHA1AndDH20(t *testing.T) {
	request := Request{
		Action: ActionStart, Profile: "test-profile", Protocol: ProtocolIPSec,
		Host: "vpn.example.invalid", Username: "test-user", Password: "test-password",
		Routes: []string{"198.51.100.0/24", "203.0.113.0/24"},
		IPSec: &IPSecRequest{
			Version: 1, AuthMode: "xauth", PreSharedKey: "test-shared-key", ModeConfig: true,
			Fragmentation: "yes", DPDAction: "restart", DPDDelay: 30, DPDTimeout: 150,
			IKELifetime: 28800, ChildLifetime: 3600, ReplayWindow: 32,
			IKEProposals: "aes128-sha1-ecp384", ESPProposals: "aes128-sha1-ecp384,aes128-sha256-ecp384",
		},
	}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	configuration := request.IPSecConfiguration()
	for _, expected := range []string{
		"version = 1", "aes128-sha1-ecp384", "aes128-sha256-ecp384",
		"remote_ts = 198.51.100.0/24,203.0.113.0/24", "auth = xauth", "vips = 0.0.0.0",
	} {
		if !strings.Contains(configuration, expected) {
			t.Errorf("missing %q in IPsec configuration", expected)
		}
	}
	if strings.Contains(configuration, "remote_ts = 0.0.0.0/0") {
		t.Fatal("IPsec configuration replaced split routes with a default route")
	}
}

func TestIPSecRequestRejectsConfigurationInjection(t *testing.T) {
	base := Request{
		Action: ActionStart, Profile: "test-profile", Protocol: ProtocolIPSec,
		Host: "vpn.example.invalid", Routes: []string{"198.51.100.0/24"},
		IPSec: &IPSecRequest{
			Version: 2, AuthMode: "none", PreSharedKey: "test-shared-key",
			Fragmentation: "yes", DPDAction: "clear", DPDDelay: 30, DPDTimeout: 150,
			IKELifetime: 28800, ChildLifetime: 3600, ReplayWindow: 32,
			IKEProposals: "aes256-sha256-ecp384", ESPProposals: "aes256-sha256-ecp384",
		},
	}
	unsafeIdentity := base
	unsafeSettings := *base.IPSec
	unsafeSettings.LocalID = "test-user\nconnections { injected"
	unsafeIdentity.IPSec = &unsafeSettings
	if err := unsafeIdentity.Validate(); err == nil {
		t.Fatal("accepted an injected IPsec identity")
	}
	unsafeProposal := base
	unsafeProposalSettings := *base.IPSec
	unsafeProposalSettings.IKEProposals = "aes256-sha256-ecp384\nload = yes"
	unsafeProposal.IPSec = &unsafeProposalSettings
	if err := unsafeProposal.Validate(); err == nil {
		t.Fatal("accepted an injected IPsec proposal")
	}
}
