//go:build !windows

package credentials

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
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
	if path := filepath.Join(tmp, "config", "VPNToris", "credentials.json"); fileExists(path) {
		info, _ := os.Stat(path)
		if info != nil && info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("credentials file mode too open: %v", info.Mode())
		}
	}
}
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
