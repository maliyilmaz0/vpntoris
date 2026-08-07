//go:build darwin

package netbackend

import (
	"strings"
	"testing"
)

func TestNewDarwinRouterCommandShape(t *testing.T) {
	var calls []string
	router := NewDarwinRouter(func(name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil, nil
	})
	if err := router.AddRoutes("utun12", []string{"10.1.0.0/16"}); err != nil {
		t.Fatal(err)
	}
	router.DeleteRoutes("utun12", []string{"10.1.0.0/16"})
	if len(calls) != 2 {
		t.Fatalf("calls = %#v", calls)
	}
	if !strings.HasPrefix(calls[0], "/sbin/route -n add -net 10.1.0.0/16 -interface utun12") {
		t.Fatalf("add = %q", calls[0])
	}
	if !strings.HasPrefix(calls[1], "/sbin/route -n delete -net 10.1.0.0/16 -interface utun12") {
		t.Fatalf("delete = %q", calls[1])
	}
}
