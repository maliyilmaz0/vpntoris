package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// promptSecret asks the user for a short secret (password/OTP) using a native dialog when available.
// Secrets are never written to application logs by this function.
func promptSecret(title, message string) (string, error) {
	switch runtime.GOOS {
	case "linux":
		if value, err := runPrompt("zenity", "--password", "--title="+title, "--text="+message); err == nil {
			return value, nil
		}
		if value, err := runPrompt("kdialog", "--title", title, "--password", message); err == nil {
			return value, nil
		}
	case "windows":
		script := fmt.Sprintf(
			`Add-Type -AssemblyName Microsoft.VisualBasic; [Microsoft.VisualBasic.Interaction]::InputBox('%s','%s','')`,
			escapePS(message),
			escapePS(title),
		)
		if value, err := runPrompt("powershell", "-NoProfile", "-NonInteractive", "-Command", script); err == nil {
			return value, nil
		}
	case "darwin":
		script := fmt.Sprintf(`display dialog %q default answer "" with title %q with hidden answer`, message, title)
		if value, err := runPrompt("osascript", "-e", script, "-e", "text returned of result"); err == nil {
			return value, nil
		}
	}
	return "", fmt.Errorf("no secret dialog available on %s (install zenity/kdialog on Linux)", runtime.GOOS)
}

// promptConfirm asks a yes/no question.
func promptConfirm(title, message string) bool {
	switch runtime.GOOS {
	case "linux":
		if err := exec.Command("zenity", "--question", "--title="+title, "--text="+message).Run(); err == nil {
			return true
		}
		if err := exec.Command("kdialog", "--title", title, "--yesno", message).Run(); err == nil {
			return true
		}
	case "windows":
		script := fmt.Sprintf(
			`$r = [System.Windows.Forms.MessageBox]::Show('%s','%s','YesNo','Question'); if ($r -eq 'Yes') { exit 0 } else { exit 1 }`,
			escapePS(message),
			escapePS(title),
		)
		cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", "Add-Type -AssemblyName System.Windows.Forms; "+script)
		if err := cmd.Run(); err == nil {
			return true
		}
	case "darwin":
		script := fmt.Sprintf(`display dialog %q with title %q buttons {"Cancel","OK"} default button "OK"`, message, title)
		if err := exec.Command("osascript", "-e", script).Run(); err == nil {
			return true
		}
	}
	return false
}

func runPrompt(name string, args ...string) (string, error) {
	command := exec.Command(name, args...)
	command.Env = os.Environ()
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func escapePS(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
