package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func promptSecret(title, message string) (string, error) {
	var (
		value string
		err   error
	)
	withDialog(func() {
		switch runtime.GOOS {
		case "linux":
			if v, e := runPrompt("zenity", "--password", "--title="+title, "--text="+message); e == nil {
				value, err = v, nil
				return
			}
			if v, e := runPrompt("kdialog", "--title", title, "--password", message); e == nil {
				value, err = v, nil
				return
			}
			err = fmt.Errorf("no secret dialog available on linux (install zenity/kdialog)")
		case "windows":
			script := fmt.Sprintf(
				`Add-Type -AssemblyName Microsoft.VisualBasic; [Microsoft.VisualBasic.Interaction]::InputBox('%s','%s','')`,
				escapePS(message),
				escapePS(title),
			)
			if v, e := runPrompt("powershell", "-NoProfile", "-NonInteractive", "-Command", script); e == nil {
				value, err = v, nil
				return
			}
			err = eor(err, fmt.Errorf("password dialog failed"))
		case "darwin":
			script := fmt.Sprintf(`display dialog %q default answer "" with title %q with hidden answer`, message, title)
			if v, e := runPrompt("osascript", "-e", script, "-e", "text returned of result"); e == nil {
				value, err = v, nil
				return
			}
			err = fmt.Errorf("password dialog cancelled")
		default:
			err = fmt.Errorf("no secret dialog available on %s", runtime.GOOS)
		}
	})
	return value, err
}
func eor(err, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
}
func promptEntry(title, message, defaultValue string) (string, error) {
	switch runtime.GOOS {
	case "linux":
		args := []string{"--entry", "--title=" + title, "--text=" + message}
		if defaultValue != "" {
			args = append(args, "--entry-text="+defaultValue)
		}
		if value, err := runPrompt("zenity", args...); err == nil {
			return value, nil
		}
		if value, err := runPrompt("kdialog", "--title", title, "--inputbox", message, defaultValue); err == nil {
			return value, nil
		}
	case "windows":
		script := fmt.Sprintf(
			`Add-Type -AssemblyName Microsoft.VisualBasic; [Microsoft.VisualBasic.Interaction]::InputBox('%s','%s','%s')`,
			escapePS(message), escapePS(title), escapePS(defaultValue),
		)
		if value, err := runPrompt("powershell", "-NoProfile", "-NonInteractive", "-Command", script); err == nil {
			return value, nil
		}
	case "darwin":
		script := fmt.Sprintf(`display dialog %q default answer %q with title %q`, message, defaultValue, title)
		if value, err := runPrompt("osascript", "-e", script, "-e", "text returned of result"); err == nil {
			return value, nil
		}
	}
	return "", fmt.Errorf("entry dialog cancelled")
}
func promptList(title, message string, options []string, selected string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no options")
	}
	switch runtime.GOOS {
	case "linux":
		args := []string{"--list", "--title=" + title, "--text=" + message, "--column=Option", "--hide-header"}
		args = append(args, options...)
		if value, err := runPrompt("zenity", args...); err == nil && value != "" {
			return value, nil
		}
		if value, err := runPrompt("kdialog", append([]string{"--title", title, "--menu", message}, flattenMenu(options)...)...); err == nil {
			return value, nil
		}
	case "darwin":
		choices := strings.Join(options, `","`)
		script := fmt.Sprintf(`choose from list {"%s"} with prompt %q with title %q`, choices, message, title)
		if selected != "" {
			script += fmt.Sprintf(` default items {"%s"}`, selected)
		}
		if value, err := runPrompt("osascript", "-e", script); err == nil && value != "false" {
			return value, nil
		}
	case "windows":
		hint := message + " [" + strings.Join(options, "|") + "]"
		if value, err := promptEntry(title, hint, selected); err == nil {
			return value, nil
		}
	}
	return "", fmt.Errorf("list dialog cancelled")
}
func promptFile(title, message string) (string, error) {
	switch runtime.GOOS {
	case "linux":
		if value, err := runPrompt("zenity", "--file-selection", "--title="+title); err == nil {
			return value, nil
		}
		if value, err := runPrompt("kdialog", "--title", title, "--getopenfilename", "."); err == nil {
			return value, nil
		}
	case "darwin":
		script := `POSIX path of (choose file with prompt ` + fmt.Sprintf("%q", message) + `)`
		if value, err := runPrompt("osascript", "-e", script); err == nil {
			return value, nil
		}
	case "windows":
		script := `
Add-Type -AssemblyName System.Windows.Forms
$f = New-Object System.Windows.Forms.OpenFileDialog
$f.Filter = 'OpenVPN|*.ovpn;*.conf|All|*.*'
if ($f.ShowDialog() -eq 'OK') { $f.FileName }
`
		if value, err := runPrompt("powershell", "-NoProfile", "-Command", script); err == nil {
			return value, nil
		}
	}
	_ = message
	return "", fmt.Errorf("file dialog cancelled")
}
func promptConfirm(title, message string) bool {
	var ok bool
	withDialog(func() {
		switch runtime.GOOS {
		case "linux":
			if err := runDialog("zenity", "--question", "--title="+title, "--text="+message, "--width=360"); err == nil {
				ok = true
				return
			}
			if err := runDialog("kdialog", "--title", title, "--yesno", message); err == nil {
				ok = true
			}
		case "windows":
			script := fmt.Sprintf(
				`$r = [System.Windows.Forms.MessageBox]::Show('%s','%s','YesNo','Question'); if ($r -eq 'Yes') { exit 0 } else { exit 1 }`,
				escapePS(message),
				escapePS(title),
			)
			if err := runDialog("powershell", "-NoProfile", "-NonInteractive", "-Command", "Add-Type -AssemblyName System.Windows.Forms; "+script); err == nil {
				ok = true
			}
		case "darwin":
			script := fmt.Sprintf(`display dialog %q with title %q buttons {"Cancel","OK"} default button "OK"`, message, title)
			if err := runDialog("osascript", "-e", script); err == nil {
				ok = true
			}
		}
	})
	return ok
}
func flattenMenu(options []string) []string {
	out := make([]string, 0, len(options)*2)
	for _, option := range options {
		out = append(out, option, option)
	}
	return out
}
func runPrompt(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dialogTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	command.Env = desktopEnv()
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
func escapePS(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
