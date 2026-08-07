//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func readInterfaceCounters(interfaceName string) (uint64, uint64, error) {
	if interfaceName == "" || strings.Contains(interfaceName, "/") || strings.Contains(interfaceName, "..") {
		return 0, 0, fmt.Errorf("invalid interface name")
	}
	received, err := readSysfsCounter(filepath.Join("/sys/class/net", interfaceName, "statistics/rx_bytes"))
	if err != nil {
		return 0, 0, err
	}
	sent, err := readSysfsCounter(filepath.Join("/sys/class/net", interfaceName, "statistics/tx_bytes"))
	if err != nil {
		return 0, 0, err
	}
	return received, sent, nil
}

func readSysfsCounter(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
}
