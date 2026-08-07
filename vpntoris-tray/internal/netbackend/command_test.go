package netbackend

import (
	"fmt"
	"strings"
	"testing"
)

func TestCommandRouterAddsAndDeletesInOrder(t *testing.T) {
	var calls []string
	router := commandRouter{
		run: func(name string, args ...string) ([]byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil, nil
		},
		add: func(interfaceName, route string) (string, []string) {
			return "ip", []string{"route", "replace", route, "dev", interfaceName}
		},
		remove: func(interfaceName, route string) (string, []string) {
			return "ip", []string{"route", "del", route, "dev", interfaceName}
		},
	}
	if err := router.AddRoutes("tun0", []string{"10.38.0.0/16", "10.68.236.0/24"}); err != nil {
		t.Fatal(err)
	}
	router.DeleteRoutes("tun0", []string{"10.38.0.0/16", "10.68.236.0/24"})
	want := []string{
		"ip route replace 10.38.0.0/16 dev tun0",
		"ip route replace 10.68.236.0/24 dev tun0",
		"ip route del 10.68.236.0/24 dev tun0",
		"ip route del 10.38.0.0/16 dev tun0",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %#v", calls)
	}
	for index := range want {
		if calls[index] != want[index] {
			t.Fatalf("call %d = %q, want %q", index, calls[index], want[index])
		}
	}
}

func TestCommandRouterRejectsDefaultRoute(t *testing.T) {
	router := commandRouter{
		run: func(string, ...string) ([]byte, error) {
			t.Fatal("run should not be called for default routes")
			return nil, nil
		},
		add: func(interfaceName, route string) (string, []string) {
			return "ip", []string{"route", "replace", route, "dev", interfaceName}
		},
		remove: func(interfaceName, route string) (string, []string) {
			return "ip", []string{"route", "del", route, "dev", interfaceName}
		},
	}
	if err := router.AddRoutes("tun0", []string{"0.0.0.0/0"}); err == nil {
		t.Fatal("expected default route rejection")
	}
}

func TestCommandRouterRollsBackOnFailure(t *testing.T) {
	var calls []string
	router := commandRouter{
		run: func(name string, args ...string) ([]byte, error) {
			line := name + " " + strings.Join(args, " ")
			calls = append(calls, line)
			if strings.Contains(line, "10.68.0.0/16") && strings.Contains(line, "replace") {
				return []byte("RTNETLINK answers: Network is unreachable"), fmt.Errorf("exit 2")
			}
			return nil, nil
		},
		add: func(interfaceName, route string) (string, []string) {
			return "ip", []string{"route", "replace", route, "dev", interfaceName}
		},
		remove: func(interfaceName, route string) (string, []string) {
			return "ip", []string{"route", "del", route, "dev", interfaceName}
		},
	}
	err := router.AddRoutes("tun0", []string{"10.38.0.0/16", "10.68.0.0/16"})
	if err == nil {
		t.Fatal("expected failure")
	}
	// first add succeeds, second fails, then first is deleted
	if len(calls) < 3 || !strings.Contains(calls[2], "del 10.38.0.0/16") {
		t.Fatalf("expected rollback delete, got %#v", calls)
	}
}

func TestCommandRouterRequiresInterface(t *testing.T) {
	router := commandRouter{
		run: func(string, ...string) ([]byte, error) { return nil, nil },
		add: func(string, string) (string, []string) { return "ip", nil },
	}
	if err := router.AddRoutes("", []string{"10.0.0.0/8"}); err == nil {
		t.Fatal("expected missing interface error")
	}
}

func TestValidateRouteCIDR(t *testing.T) {
	if err := validateRouteCIDR("10.0.0.0/8"); err != nil {
		t.Fatal(err)
	}
	if err := validateRouteCIDR("not-a-cidr"); err == nil {
		t.Fatal("expected invalid cidr")
	}
	if err := validateRouteCIDR("2001:db8::/32"); err == nil {
		t.Fatal("expected IPv6 rejection")
	}
}
