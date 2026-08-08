//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSysfsCounter(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "rx_bytes")
	if err := os.WriteFile(path, []byte("4096\n"), 0600); err != nil {
		t.Fatal(err)
	}
	value, err := readSysfsCounter(path)
	if err != nil {
		t.Fatal(err)
	}
	if value != 4096 {
		t.Fatalf("value = %d", value)
	}
}
func TestReadInterfaceCountersRejectsTraversal(t *testing.T) {
	if _, _, err := readInterfaceCounters("../escape"); err == nil {
		t.Fatal("expected invalid interface name")
	}
}
