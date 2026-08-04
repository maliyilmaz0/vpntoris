package main

import (
	"encoding/json"
	"fmt"
	"os"

	"vpntoris-tray/internal/nativeengine"
	"vpntoris-tray/internal/platforminfo"
)

const owner = "com.vpntoris.native-engine"

func main() {
	capabilities := platforminfo.Current()
	if len(os.Args) != 2 {
		fatal("usage: vpntoris-service doctor | journal")
	}
	switch os.Args[1] {
	case "doctor":
		writeJSON(capabilities)
	case "journal":
		journal, err := nativeengine.NewJournal(capabilities.StateDirectory, owner)
		if err != nil {
			fatal(err.Error())
		}
		transactions, err := journal.List()
		if err != nil {
			fatal(err.Error())
		}
		writeJSON(transactions)
	default:
		fatal("unknown command")
	}
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
