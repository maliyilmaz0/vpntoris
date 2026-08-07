//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"strconv"
)

func openBrowser(uid int, target string) error {
	if uid < 501 {
		return fmt.Errorf("invalid browser launch request")
	}
	if output, err := exec.Command("/bin/launchctl", "asuser", strconv.Itoa(uid), "/usr/bin/open", target).CombinedOutput(); err != nil {
		return fmt.Errorf("could not open authentication browser: %s", output)
	}
	return nil
}
