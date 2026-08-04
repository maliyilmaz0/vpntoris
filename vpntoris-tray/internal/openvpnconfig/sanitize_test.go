package openvpnconfig

import (
	"strings"
	"testing"
)

func TestSanitizePreservesCertificatesAndRemovesRoutes(t *testing.T) {
	configuration := "client\ndev tun\nremote vpn.example.invalid 443\nauth-user-pass\nredirect-gateway def1\nroute 198.51.100.0 255.255.255.0\n<ca>\nTEST CERTIFICATE\n</ca>\n"
	result, err := Sanitize(configuration)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"auth-user-pass\n", "redirect-gateway", "route 198.51.100.0"} {
		if strings.Contains(result, forbidden) {
			t.Fatalf("directive remained: %s", forbidden)
		}
	}
	for _, required := range []string{"<ca>", "TEST CERTIFICATE", "route-nopull", "script-security 1", "auth-nocache"} {
		if !strings.Contains(result, required) {
			t.Fatalf("missing value: %s", required)
		}
	}
}

func TestSanitizeRejectsCommandsAndIncludes(t *testing.T) {
	for _, configuration := range []string{
		"client\nup /tmp/script\n",
		"client\nplugin malicious.so\n",
		"client\nconfig another.conf\n",
		"client\nmanagement 127.0.0.1 7505\n",
		"client\n<auth-user-pass>\nuser\nsecret\n</auth-user-pass>\n",
	} {
		if _, err := Sanitize(configuration); err == nil {
			t.Fatalf("unsafe configuration accepted: %q", configuration)
		}
	}
}

func TestSanitizeRejectsTapMode(t *testing.T) {
	if _, err := Sanitize("client\ndev tap\n"); err == nil {
		t.Fatal("tap configuration was accepted")
	}
}
