//go:build windows

package main

import "fmt"

func fmtWindowsUnsupported(protocol string) error {
	return fmt.Errorf("%s is not supported on Windows in this release", protocol)
}
