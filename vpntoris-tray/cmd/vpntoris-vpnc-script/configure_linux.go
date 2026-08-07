//go:build linux

package main

import (
	"fmt"
	"os/exec"
	"strconv"
)

func configureInterface(interfaceName, address string, mtu int) error {
	// OpenConnect on Linux typically creates the tun device; we only assign addressing.
	if output, err := exec.Command("ip", "link", "set", "dev", interfaceName, "mtu", strconv.Itoa(mtu), "up").CombinedOutput(); err != nil {
		return fmt.Errorf("could not bring OpenConnect interface up: %s", output)
	}
	// Point-to-point style /32 on the tunnel endpoint.
	if output, err := exec.Command("ip", "addr", "replace", address+"/32", "dev", interfaceName).CombinedOutput(); err != nil {
		return fmt.Errorf("could not configure OpenConnect address: %s", output)
	}
	return nil
}
