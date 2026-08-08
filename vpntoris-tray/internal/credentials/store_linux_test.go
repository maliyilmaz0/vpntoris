//go:build linux

package credentials

import (
	"os"
	"path/filepath"
	"testing"
)

func fakeSecretTool(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	backing := filepath.Join(dir, "backing")
	script := filepath.Join(dir, "secret-tool")
	content := `#!/bin/sh
backing="` + backing + `"
cmd="$1"; shift
profile=""; field=""
while [ $# -gt 0 ]; do
  case "$1" in
    profile) profile="$2"; shift 2 ;;
    field) field="$2"; shift 2 ;;
    *) shift ;;
  esac
done
key="$profile/$field"
touch "$backing"
case "$cmd" in
  store)
    read -r value
    grep -v "^$key=" "$backing" > "$backing.tmp" || true
    echo "$key=$value" >> "$backing.tmp"
    mv "$backing.tmp" "$backing"
    ;;
  lookup)
    line=$(grep "^$key=" "$backing") || exit 1
    echo "${line#*=}"
    ;;
  clear)
    grep -v "^$profile/" "$backing" > "$backing.tmp" || true
    mv "$backing.tmp" "$backing"
    ;;
esac
`
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	return script
}
func TestSecretToolStoreRoundTrip(t *testing.T) {
	store := &secretToolStore{binary: fakeSecretTool(t)}
	if err := store.Write("office", "password", "s3cret"); err != nil {
		t.Fatal(err)
	}
	value, err := store.Read("office", "password")
	if err != nil || value != "s3cret" {
		t.Fatalf("read = %q, %v", value, err)
	}
	if err := store.Write("office", "password", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read("office", "password"); err == nil {
		t.Fatal("expected missing credential after empty write")
	}
}
func TestHybridFallsBackToFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	hybrid := &hybridStore{
		secret: &secretToolStore{binary: filepath.Join(tmp, "missing-secret-tool")},
		file:   newFileStore(),
	}
	if err := hybrid.Write("office", "psk", "key"); err != nil {
		t.Fatal(err)
	}
	value, err := hybrid.Read("office", "psk")
	if err != nil || value != "key" {
		t.Fatalf("read = %q, %v", value, err)
	}
	if err := hybrid.Delete("office"); err != nil {
		t.Fatal(err)
	}
}
