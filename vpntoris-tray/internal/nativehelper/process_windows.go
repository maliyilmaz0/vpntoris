//go:build windows

package nativehelper

import (
	"fmt"
	"os/exec"
	"syscall"
)

func configureProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func terminateProcessGroup(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	_ = command.Process.Kill()
}

func createOTPChannel(string) error {
	return fmt.Errorf("IPsec OTP channel is unavailable on windows")
}
