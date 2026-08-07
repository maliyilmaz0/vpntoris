//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"vpntoris-tray/internal/helperipc"
	"vpntoris-tray/internal/nativehelper"
	"vpntoris-tray/internal/netbackend"
	"vpntoris-tray/internal/runtimepaths"
)

func main() {
	if len(os.Args) < 2 {
		fatal("usage: vpntoris-native-helper daemon engine-root | service engine-root")
	}
	mode := os.Args[1]
	if mode != "daemon" && mode != "service" {
		fatal("usage: vpntoris-native-helper daemon engine-root | service engine-root")
	}
	if len(os.Args) != 3 {
		fatal("usage: vpntoris-native-helper daemon engine-root | service engine-root")
	}
	engineRoot, err := filepath.Abs(os.Args[2])
	if err != nil {
		fatal(err.Error())
	}
	paths := runtimepaths.Current()
	if err := os.MkdirAll(paths.RuntimeDirectory, 0700); err != nil {
		fatal(err.Error())
	}
	if err := os.MkdirAll(paths.LogDirectory, 0755); err != nil {
		fatal(err.Error())
	}
	if err := os.MkdirAll(paths.StateDirectory, 0700); err != nil {
		fatal(err.Error())
	}
	service, err := nativehelper.New(nativehelper.Config{
		Paths:      paths,
		EngineRoot: runtimepaths.EngineBundle(engineRoot),
		UserID:     -1,
		Router:     netbackend.New(),
	})
	if err != nil {
		fatal(err.Error())
	}
	if err := service.PrepareRuntime(); err != nil {
		fatal(err.Error())
	}
	// service mode currently shares the same foreground loop; a future
	// golang.org/x/sys/windows/svc wrapper can host this entrypoint.
	_ = mode
	if err := helperipc.ServePipe(service, paths); err != nil {
		fatal(err.Error())
	}
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
