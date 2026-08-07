//go:build darwin

package netbackend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinDNSWritesResolverFile(t *testing.T) {
	root := t.TempDir()
	dns := darwinDNS{resolverRoot: root}
	if err := dns.AddScoped("utun12", "corp.example.com", []string{"10.0.0.53", "10.0.0.54"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "corp.example.com"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "nameserver 10.0.0.53") || !strings.Contains(text, "nameserver 10.0.0.54") {
		t.Fatalf("resolver file = %q", text)
	}
	if err := dns.RemoveScoped("utun12", "corp.example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "corp.example.com")); !os.IsNotExist(err) {
		t.Fatalf("resolver file should be removed, err=%v", err)
	}
}
