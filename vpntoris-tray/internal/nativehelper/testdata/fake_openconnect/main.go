package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	fmt.Fprintln(os.Stdout, "VPNTORIS_INTERFACE=tun8")
	_ = os.Stdout.Sync()
	if len(os.Args) > 1 {
		_ = os.Args
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	select {
	case <-signals:
	case <-time.After(2 * time.Minute):
	}
}
