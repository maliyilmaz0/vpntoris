//go:build windows

package nativeengine

import (
	"os"
	"os/exec"
	"syscall"
)

func configureProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func terminateProcess(process *os.Process) error {
	return process.Kill()
}
