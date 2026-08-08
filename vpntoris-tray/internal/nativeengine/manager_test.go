package nativeengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type memoryBackend struct {
	resources map[string]bool
	fail      string
	undoOrder []string
}

func (backend *memoryBackend) Apply(_ context.Context, mutation Mutation) error {
	if mutation.Resource == backend.fail {
		return errors.New("apply failed")
	}
	backend.resources[mutation.Resource] = true
	return nil
}
func (backend *memoryBackend) Undo(_ context.Context, mutation Mutation) error {
	delete(backend.resources, mutation.Resource)
	backend.undoOrder = append(backend.undoOrder, mutation.Resource)
	return nil
}
func (backend *memoryBackend) Owned(_ context.Context, mutation Mutation) (bool, error) {
	return backend.resources[mutation.Resource], nil
}
func TestManagerRollsBackInReverseOrder(t *testing.T) {
	directory := t.TempDir()
	journal, err := NewJournal(directory, "vpntoris-test")
	if err != nil {
		t.Fatal(err)
	}
	backend := &memoryBackend{resources: map[string]bool{}, fail: "route-b"}
	manager, _ := NewManager(journal, backend)
	_, err = manager.Activate(context.Background(), Plan{Profile: "office", Mutations: []Mutation{{Kind: MutationInterface, Resource: "tun-a"}, {Kind: MutationRoute, Resource: "route-a"}, {Kind: MutationRoute, Resource: "route-b"}}})
	if err == nil {
		t.Fatal("activation should fail")
	}
	if len(backend.resources) != 0 {
		t.Fatalf("resources were not rolled back: %#v", backend.resources)
	}
	if len(backend.undoOrder) != 2 || backend.undoOrder[0] != "route-a" || backend.undoOrder[1] != "tun-a" {
		t.Fatalf("unexpected rollback order: %#v", backend.undoOrder)
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 0 {
		t.Fatalf("completed rollback left journal entries: %#v", entries)
	}
}
func TestManagerRecoversPersistedResources(t *testing.T) {
	directory := t.TempDir()
	journal, _ := NewJournal(directory, "vpntoris-test")
	transaction, _ := journal.Begin("office")
	transaction.State = TransactionActive
	transaction.Mutations = []Mutation{{ID: "mutation-a", Kind: MutationRoute, State: MutationApplied, Resource: "route-a"}}
	if err := journal.Save(transaction); err != nil {
		t.Fatal(err)
	}
	backend := &memoryBackend{resources: map[string]bool{"route-a": true}}
	manager, _ := NewManager(journal, backend)
	if err := manager.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if backend.resources["route-a"] {
		t.Fatal("recovery did not remove owned route")
	}
	if _, err := os.Stat(filepath.Join(directory, transaction.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("journal was not removed: %v", err)
	}
}
func TestManagerRecoversPendingMutationAppliedBeforeCrash(t *testing.T) {
	directory := t.TempDir()
	journal, _ := NewJournal(directory, "vpntoris-test")
	transaction, _ := journal.Begin("office")
	transaction.Mutations = []Mutation{{ID: "mutation-a", Kind: MutationInterface, State: MutationPending, Resource: "tun-a"}}
	if err := journal.Save(transaction); err != nil {
		t.Fatal(err)
	}
	backend := &memoryBackend{resources: map[string]bool{"tun-a": true}}
	manager, _ := NewManager(journal, backend)
	if err := manager.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if backend.resources["tun-a"] {
		t.Fatal("pending mutation leaked a resource after recovery")
	}
}
