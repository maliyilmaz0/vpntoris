package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	go func() { _, _ = io.Copy(io.Discard, os.Stdin) }()
	fmt.Fprintln(os.Stdout, "INFO: Tunnel established.")
	fmt.Fprintln(os.Stdout, "INFO: Interface ppp17 is UP.")
	_ = os.Stdout.Sync()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	select {
	case <-signals:
	case <-time.After(2 * time.Minute):
	}
}
