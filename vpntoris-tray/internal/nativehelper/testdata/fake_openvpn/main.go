package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	managementPath := ""
	for index := 0; index < len(os.Args); index++ {
		if os.Args[index] == "--management" && index+2 < len(os.Args) && os.Args[index+2] == "unix" {
			managementPath = os.Args[index+1]
			break
		}
	}
	if managementPath == "" {
		fmt.Fprintln(os.Stderr, "missing --management path unix")
		os.Exit(2)
	}
	_ = os.Remove(managementPath)
	if err := os.MkdirAll(filepath.Dir(managementPath), 0700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	listener, err := net.Listen("unix", managementPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer listener.Close()
	_ = os.Chmod(managementPath, 0600)
	fmt.Fprintln(os.Stdout, "TUN/TAP device tun9 opened")
	fmt.Fprintln(os.Stdout, "Initialization Sequence Completed")
	_ = os.Stdout.Sync()
	go serveManagement(listener)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	<-signals
	_ = os.Remove(managementPath)
}
func serveManagement(listener net.Listener) {
	for {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		go handleManagement(connection)
	}
}
func handleManagement(connection net.Conn) {
	defer connection.Close()
	_, _ = connection.Write([]byte(">INFO:OpenVPN Management Interface Version 1\r\n"))
	_, _ = connection.Write([]byte(">PASSWORD:Need 'Auth' username/password\r\n"))
	scanner := bufio.NewScanner(connection)
	authenticated := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "username ") || strings.HasPrefix(line, "password ") {
			authenticated = true
		}
		if strings.Contains(line, "hold release") || strings.Contains(line, "state on") || strings.Contains(line, "bytecount") {
		}
		if authenticated {
			for {
				_, _ = fmt.Fprintf(connection, ">BYTECOUNT:%d,%d\r\n", 1000, 2000)
				time.Sleep(500 * time.Millisecond)
			}
		}
	}
}
