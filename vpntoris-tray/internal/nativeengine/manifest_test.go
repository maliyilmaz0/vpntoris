package nativeengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadEngineManifestVerifiesDigestAndPath(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "engine")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	if err := os.WriteFile(executable, []byte("engine"), 0700); err != nil {
		t.Fatal(err)
	}
	digest, _ := fileSHA256(executable)
	manifest := EngineManifest{ID: "openvpn", Protocol: "openvpn", Version: "test", OS: runtime.GOOS, Architecture: runtime.GOARCH, Executable: filepath.Base(executable), SHA256: digest, License: "GPL-2.0"}
	data, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(manifestPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, path, err := LoadEngineManifest(root, manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	resolvedExecutable, _ := filepath.EvalSymlinks(executable)
	if loaded.ID != "openvpn" || path != resolvedExecutable {
		t.Fatalf("unexpected manifest result: %#v %s", loaded, path)
	}
	manifest.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	data, _ = json.Marshal(manifest)
	_ = os.WriteFile(manifestPath, data, 0600)
	if _, _, err := LoadEngineManifest(root, manifestPath); err == nil {
		t.Fatal("digest mismatch was accepted")
	}
}
