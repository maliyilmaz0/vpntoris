//go:build windows

package main

import "golang.org/x/sys/windows"

func parentProcessAlive(processID int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(processID))
	if err != nil {
		return false
	}
	windows.CloseHandle(handle)
	return true
}
