//go:build darwin

package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
)

func main() {
	uid, err := strconv.Atoi(os.Getenv("VPNTORIS_USER_UID"))
	if err != nil || uid < 501 || len(os.Args) != 2 || !allowedBrowserURL(os.Args[1]) {
		fatal("invalid browser launch request")
	}
	if output, err := exec.Command("/bin/launchctl", "asuser", strconv.Itoa(uid), "/usr/bin/open", os.Args[1]).CombinedOutput(); err != nil {
		fatal(fmt.Sprintf("could not open authentication browser: %s", output))
	}
}

func allowedBrowserURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.Hostname() == "" {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	return parsed.Scheme == "http" && net.ParseIP(parsed.Hostname()) != nil && net.ParseIP(parsed.Hostname()).IsLoopback()
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
