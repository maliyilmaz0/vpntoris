//go:build darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"vpntoris-tray/internal/helperipc"
	"vpntoris-tray/internal/nativehelper"
	"vpntoris-tray/internal/netbackend"
	"vpntoris-tray/internal/runtimepaths"
)

func main() {
	if os.Geteuid() != 0 {
		fatal("native helper must run as root")
	}
	if len(os.Args) != 4 || os.Args[1] != "daemon" {
		fatal("usage: vpntoris-native-helper daemon uid engine-root")
	}
	uid, err := strconv.Atoi(os.Args[2])
	if err != nil || uid < 0 {
		fatal("invalid user id")
	}
	engineRoot, err := filepath.Abs(os.Args[3])
	if err != nil {
		fatal(err.Error())
	}
	paths := runtimepaths.Current()
	// Explicit full path layout enables journal recovery under StateDirectory.
	service, err := nativehelper.New(nativehelper.Config{
		Paths:      paths,
		EngineRoot: runtimepaths.EngineBundle(engineRoot),
		UserID:     uid,
		Router:     netbackend.New(),
	})
	if err != nil {
		fatal(err.Error())
	}
	if err := service.PrepareRuntime(); err != nil {
		fatal(err.Error())
	}
	if err := helperipc.ServeUnix(service, paths, uid); err != nil {
		fatal(err.Error())
	}
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
