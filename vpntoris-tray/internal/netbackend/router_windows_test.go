//go:build windows

package netbackend

import (
	"strings"
	"testing"
)

func TestNewWindowsRouterCommandShape(t *testing.T) {
	var calls []string
	router := NewWindowsRouter(func(name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil, nil
	}, func(string) (int, error) { return 42, nil })
	if err := router.AddRoutes("Ethernet 2", []string{"10.1.0.0/16"}); err != nil {
		t.Fatal(err)
	}
	router.DeleteRoutes("Ethernet 2", []string{"10.1.0.0/16"})
	if len(calls) != 2 {
		t.Fatalf("calls = %#v", calls)
	}
	if calls[0] != "route ADD 10.1.0.0 MASK 255.255.0.0 0.0.0.0 IF 42" {
		t.Fatalf("add = %q", calls[0])
	}
	if calls[1] != "route DELETE 10.1.0.0 MASK 255.255.0.0 IF 42" {
		t.Fatalf("delete = %q", calls[1])
	}
}
