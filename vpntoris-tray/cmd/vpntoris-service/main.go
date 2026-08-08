package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"vpntoris-tray/internal/nativeengine"
	"vpntoris-tray/internal/netbackend"
	"vpntoris-tray/internal/platforminfo"
	"vpntoris-tray/internal/runtimepaths"
)

const owner = "com.vpntoris.native-engine"

func main() {
	if len(os.Args) != 2 {
		fatal("usage: vpntoris-service doctor | journal | repair")
	}
	paths := runtimepaths.Current()
	switch os.Args[1] {
	case "doctor":
		writeJSON(map[string]any{
			"capabilities": platforminfo.Current(),
			"paths":        paths,
		})
	case "journal":
		journal, err := nativeengine.NewJournal(paths.StateDirectory, owner)
		if err != nil {
			fatal(err.Error())
		}
		transactions, err := journal.List()
		if err != nil {
			fatal(err.Error())
		}
		writeJSON(transactions)
	case "repair":
		if err := repair(paths); err != nil {
			fatal(err.Error())
		}
		writeJSON(map[string]string{"state": "repaired", "stateDirectory": paths.StateDirectory})
	default:
		fatal("unknown command")
	}
}
func repair(paths runtimepaths.Paths) error {
	journal, err := nativeengine.NewJournal(paths.StateDirectory, owner)
	if err != nil {
		return err
	}
	backend := netbackend.MutationBackend{Router: netbackend.New(), DNS: netbackend.NewDNS()}
	manager, err := nativeengine.NewManager(journal, backend)
	if err != nil {
		return err
	}
	return manager.Recover(context.Background())
}
func writeJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fatal(err.Error())
	}
}
func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
