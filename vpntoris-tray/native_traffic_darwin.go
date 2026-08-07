//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func readInterfaceCounters(interfaceName string) (uint64, uint64, error) {
	output, err := exec.Command("/usr/sbin/netstat", "-bI", interfaceName).Output()
	if err != nil {
		return 0, 0, err
	}
	return parseDarwinInterfaceCounters(string(output), interfaceName)
}

func parseDarwinInterfaceCounters(output string, interfaceName string) (uint64, uint64, error) {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 9 || fields[0] != interfaceName || !strings.HasPrefix(fields[2], "<Link#") {
			continue
		}
		received, receivedErr := strconv.ParseUint(fields[5], 10, 64)
		sent, sentErr := strconv.ParseUint(fields[8], 10, 64)
		if receivedErr == nil && sentErr == nil {
			return received, sent, nil
		}
	}
	return 0, 0, fmt.Errorf("interface counters are unavailable")
}
