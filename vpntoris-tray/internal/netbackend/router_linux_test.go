//go:build linux

package netbackend

import (
	"strings"
	"testing"
)

func TestNewLinuxRouterCommandShape(t *testing.T) {
	var calls []string
	router := NewLinuxRouter(func(name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil, nil
	})
	if err := router.AddRoutes("tun0", []string{"10.1.0.0/16"}); err != nil {
		t.Fatal(err)
	}
	router.DeleteRoutes("tun0", []string{"10.1.0.0/16"})
	if len(calls) != 2 {
		t.Fatalf("calls = %#v", calls)
	}
	if calls[0] != "ip -4 route replace 10.1.0.0/16 dev tun0" {
		t.Fatalf("add = %q", calls[0])
	}
	if calls[1] != "ip -4 route del 10.1.0.0/16 dev tun0" {
		t.Fatalf("delete = %q", calls[1])
	}
}
