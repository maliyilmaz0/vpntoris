//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
)

func openBrowser(uid int, target string) error {
	// Prefer running as the desktop user so xdg-open reaches the session bus.
	if uid == 0 {
		if output, err := exec.Command("xdg-open", target).CombinedOutput(); err != nil {
			return fmt.Errorf("could not open authentication browser: %s", output)
		}
		return nil
	}
	account, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return fmt.Errorf("could not resolve browser user: %w", err)
	}
	command := exec.Command("runuser", "-u", account.Username, "--", "xdg-open", target)
	command.Env = append(os.Environ(), "DISPLAY="+os.Getenv("DISPLAY"), "XDG_RUNTIME_DIR=/run/user/"+strconv.Itoa(uid))
	if output, err := command.CombinedOutput(); err != nil {
		// Fallback without runuser (minimal containers).
		if output2, err2 := exec.Command("xdg-open", target).CombinedOutput(); err2 != nil {
			return fmt.Errorf("could not open authentication browser: %s; fallback: %s", output, output2)
		}
	}
	return nil
}
