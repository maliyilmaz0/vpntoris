//go:build !darwin && !linux

package main

import (
	"fmt"
	"runtime"
)

func openBrowser(int, string) error {
	return fmt.Errorf("browser open is not implemented on %s", runtime.GOOS)
}
