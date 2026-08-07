package main

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
)

var interfacePattern = regexp.MustCompile(`^(utun|tun|ppp)[0-9]+$`)

func main() {
	reason := os.Getenv("reason")
	if reason != "connect" && reason != "reconnect" {
		return
	}
	interfaceName := os.Getenv("TUNDEV")
	address := os.Getenv("INTERNAL_IP4_ADDRESS")
	if !interfacePattern.MatchString(interfaceName) || net.ParseIP(address).To4() == nil {
		fatal("invalid OpenConnect interface environment")
	}
	mtu := 1412
	for _, name := range []string{"INTERNAL_IP4_MTU", "MTU"} {
		if value, err := strconv.Atoi(os.Getenv(name)); err == nil && value >= 576 && value <= 9000 {
			mtu = value
			break
		}
	}
	if err := configureInterface(interfaceName, address, mtu); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("VPNTORIS_INTERFACE=%s\n", interfaceName)
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
