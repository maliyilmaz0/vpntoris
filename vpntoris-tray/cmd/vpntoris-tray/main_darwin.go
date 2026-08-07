//go:build darwin

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "vpntoris-tray is the Linux/Windows system tray client.")
	fmt.Fprintln(os.Stderr, "On macOS use the SwiftUI menu bar app (VPNToris.app).")
	os.Exit(2)
}
