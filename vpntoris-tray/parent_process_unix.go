//go:build !windows

package main

import "syscall"

func parentProcessAlive(processID int) bool {
	return syscall.Kill(processID, 0) == nil
}
