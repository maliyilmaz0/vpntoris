//go:build !darwin && !linux

package main

import (
	"fmt"
	"runtime"
)

func configureInterface(string, string, int) error {
	return fmt.Errorf("OpenConnect interface helper is not implemented on %s", runtime.GOOS)
}
