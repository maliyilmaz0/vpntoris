//go:build darwin || linux

package nativeengine

import (
	"os"
	"os/exec"
	"syscall"
)

func configureProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
func terminateProcess(process *os.Process) error {
	return syscall.Kill(-process.Pid, syscall.SIGTERM)
}
