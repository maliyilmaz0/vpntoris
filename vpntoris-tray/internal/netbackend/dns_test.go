package netbackend

import (
	"strings"
	"testing"
)

func TestCommandDNSAddScopedValidatesInput(t *testing.T) {
	dns := CommandDNS{
		Run: func(string, ...string) ([]byte, error) { return nil, nil },
		Add: func(interfaceName, domain string, servers []string) (string, []string) {
			return "resolvectl", []string{"dns", interfaceName, strings.Join(servers, " ")}
		},
		Remove: func(interfaceName, domain string) (string, []string) {
			return "resolvectl", []string{"revert", interfaceName}
		},
	}
	if err := dns.AddScoped("", "corp.example.com", []string{"10.0.0.53"}); err == nil {
		t.Fatal("expected missing interface error")
	}
	if err := dns.AddScoped("tun0", "", []string{"10.0.0.53"}); err == nil {
		t.Fatal("expected invalid domain error")
	}
	if err := dns.AddScoped("tun0", "corp.example.com", nil); err == nil {
		t.Fatal("expected missing servers error")
	}
	if err := dns.AddScoped("tun0", "corp.example.com", []string{"not-an-ip"}); err == nil {
		t.Fatal("expected invalid server error")
	}
}

func TestCommandDNSAddScopedBuildsCommand(t *testing.T) {
	var calls []string
	dns := CommandDNS{
		Run: func(name string, args ...string) ([]byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil, nil
		},
		Add: func(interfaceName, domain string, servers []string) (string, []string) {
			return "resolvectl", append([]string{"dns", interfaceName}, servers...)
		},
		Remove: func(interfaceName, domain string) (string, []string) {
			return "resolvectl", []string{"revert", interfaceName}
		},
	}
	if err := dns.AddScoped("tun0", "Corp.Example.com", []string{"10.0.0.53", "10.0.0.54"}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != "resolvectl dns tun0 10.0.0.53 10.0.0.54" {
		t.Fatalf("calls = %#v", calls)
	}
	_ = dns.RemoveScoped("tun0", "corp.example.com")
	if len(calls) != 2 || calls[1] != "resolvectl revert tun0" {
		t.Fatalf("calls after remove = %#v", calls)
	}
}
