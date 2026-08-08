package netbackend

import (
	"context"
	"path/filepath"
	"testing"
	"vpntoris-tray/internal/nativeengine"
)

func TestMutationBackendWithManagerActivatesAndRollsBack(t *testing.T) {
	router := &memoryRouter{}
	backend := MutationBackend{Router: router}
	journal, err := nativeengine.NewJournal(t.TempDir(), "vpntoris-test")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := nativeengine.NewManager(journal, backend)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := nativeengine.BuildNetworkPlan(nativeengine.ProfileNetwork{
		Profile:   "office",
		Interface: "tun0",
		ProcessID: 42,
		Routes:    []string{"10.38.0.0/16", "10.68.236.0/24"},
	})
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := manager.Activate(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(router.added) != 2 {
		t.Fatalf("added = %#v", router.added)
	}
	if err := manager.Deactivate(context.Background(), transaction); err != nil {
		t.Fatal(err)
	}
	if len(router.deleted) != 2 {
		t.Fatalf("deleted = %#v", router.deleted)
	}
	if entries, listErr := journal.List(); listErr != nil {
		t.Fatal(listErr)
	} else if len(entries) != 0 {
		t.Fatalf("expected empty journal, got %d", len(entries))
	}
}
func TestBuildNetworkPlanRejectsDefaultInManagerPath(t *testing.T) {
	_, err := nativeengine.BuildNetworkPlan(nativeengine.ProfileNetwork{
		Profile:   "office",
		Interface: "tun0",
		ProcessID: 1,
		Routes:    []string{"0.0.0.0/0"},
	})
	if err == nil {
		t.Fatal("expected default route rejection")
	}
	_ = filepath.Separator
}
