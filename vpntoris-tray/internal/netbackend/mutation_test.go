package netbackend

import (
	"context"
	"strings"
	"testing"

	"vpntoris-tray/internal/nativeengine"
)

type memoryRouter struct {
	added   []string
	deleted []string
	failAdd bool
}

func (router *memoryRouter) AddRoutes(interfaceName string, routes []string) error {
	if router.failAdd {
		return context.Canceled
	}
	for _, route := range routes {
		router.added = append(router.added, interfaceName+":"+route)
	}
	return nil
}

func (router *memoryRouter) DeleteRoutes(interfaceName string, routes []string) {
	for _, route := range routes {
		router.deleted = append(router.deleted, interfaceName+":"+route)
	}
}

func TestMutationBackendAppliesAndUndoesRoutes(t *testing.T) {
	router := &memoryRouter{}
	backend := MutationBackend{Router: router}
	mutation := nativeengine.Mutation{Kind: nativeengine.MutationRoute, Interface: "utun12", CIDR: "10.0.0.0/8"}
	if err := backend.Apply(context.Background(), mutation); err != nil {
		t.Fatal(err)
	}
	if len(router.added) != 1 || router.added[0] != "utun12:10.0.0.0/8" {
		t.Fatalf("unexpected added routes: %#v", router.added)
	}
	owned, err := backend.Owned(context.Background(), mutation)
	if err != nil || !owned {
		t.Fatalf("owned = %v, %v", owned, err)
	}
	if err := backend.Undo(context.Background(), mutation); err != nil {
		t.Fatal(err)
	}
	if len(router.deleted) != 1 || router.deleted[0] != "utun12:10.0.0.0/8" {
		t.Fatalf("unexpected deleted routes: %#v", router.deleted)
	}
}

func TestMutationBackendRejectsIncompleteRoute(t *testing.T) {
	backend := MutationBackend{Router: &memoryRouter{}}
	if err := backend.Apply(context.Background(), nativeengine.Mutation{Kind: nativeengine.MutationRoute, Interface: "utun12"}); err == nil {
		t.Fatal("expected incomplete route mutation to fail")
	}
}

type memoryDNS struct {
	added   []string
	removed []string
}

func (dns *memoryDNS) AddScoped(interfaceName, domain string, servers []string) error {
	dns.added = append(dns.added, interfaceName+":"+domain+":"+strings.Join(servers, ","))
	return nil
}

func (dns *memoryDNS) RemoveScoped(interfaceName, domain string) error {
	dns.removed = append(dns.removed, interfaceName+":"+domain)
	return nil
}

func TestMutationBackendAppliesDNS(t *testing.T) {
	dns := &memoryDNS{}
	backend := MutationBackend{Router: &memoryRouter{}, DNS: dns}
	mutation := nativeengine.Mutation{
		Kind:      nativeengine.MutationDNS,
		Interface: "tun0",
		Domain:    "corp.example.com",
		Values:    map[string]string{"servers": "10.0.0.53, 10.0.0.54"},
	}
	if err := backend.Apply(context.Background(), mutation); err != nil {
		t.Fatal(err)
	}
	if len(dns.added) != 1 || dns.added[0] != "tun0:corp.example.com:10.0.0.53,10.0.0.54" {
		t.Fatalf("added = %#v", dns.added)
	}
	if err := backend.Undo(context.Background(), mutation); err != nil {
		t.Fatal(err)
	}
	if len(dns.removed) != 1 || dns.removed[0] != "tun0:corp.example.com" {
		t.Fatalf("removed = %#v", dns.removed)
	}
}
