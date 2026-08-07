//go:build !darwin && !linux && !windows

package main

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	fmt.Fprintf(os.Stderr, "vpntoris-native-helper is not implemented on %s/%s yet\n", runtime.GOOS, runtime.GOARCH)
	os.Exit(1)
}
