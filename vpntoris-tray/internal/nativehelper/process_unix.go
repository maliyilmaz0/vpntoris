//go:build unix

package nativehelper

import (
	"os/exec"
	"syscall"
)

func configureProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
func terminateProcessGroup(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	_ = syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
}
func createOTPChannel(path string) error {
	return syscall.Mkfifo(path, 0600)
}
