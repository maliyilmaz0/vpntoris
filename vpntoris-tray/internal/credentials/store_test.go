//go:build !windows

package credentials

import "testing"

func TestMemoryStoreRoundTrip(t *testing.T) {
	store := New()
	if err := store.Write("office", "password", "secret"); err != nil {
		t.Fatal(err)
	}
	value, err := store.Read("office", "password")
	if err != nil || value != "secret" {
		t.Fatalf("read = %q %v", value, err)
	}
	if err := store.Delete("office"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read("office", "password"); err == nil {
		t.Fatal("expected missing credential after delete")
	}
}
