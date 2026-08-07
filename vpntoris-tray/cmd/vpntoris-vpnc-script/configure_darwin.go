//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"strconv"
)

func configureInterface(interfaceName, address string, mtu int) error {
	command := exec.Command("/sbin/ifconfig", interfaceName, "inet", address, address, "netmask", "255.255.255.255", "mtu", strconv.Itoa(mtu), "up")
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("could not configure OpenConnect interface: %s", output)
	}
	return nil
}
