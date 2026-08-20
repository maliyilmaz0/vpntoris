//go:build linux

package main

import (
	"net/http"
	"os"
	"os/exec"
	"time"
)

func ensureDaemonRunning() {
	client := http.Client{Timeout: 400 * time.Millisecond}
	if resp, err := client.Get("http://127.0.0.1:17984/api/profiles"); err == nil {
		_ = resp.Body.Close()
		return
	}

	_ = exec.Command("systemctl", "--user", "start", "vpntorisd.service").Run()

	for i := 0; i < 6; i++ {
		time.Sleep(150 * time.Millisecond)
		if resp, err := client.Get("http://127.0.0.1:17984/api/profiles"); err == nil {
			_ = resp.Body.Close()
			return
		}
	}

	binCandidates := []string{
		"/usr/lib/vpntoris/vpntorisd",
		"/usr/local/lib/vpntoris/vpntorisd",
	}
	for _, bin := range binCandidates {
		if info, err := os.Stat(bin); err == nil && !info.IsDir() {
			cmd := exec.Command(bin)
			_ = cmd.Start()
			break
		}
	}

	for i := 0; i < 10; i++ {
		time.Sleep(150 * time.Millisecond)
		if resp, err := client.Get("http://127.0.0.1:17984/api/profiles"); err == nil {
			_ = resp.Body.Close()
			return
		}
	}
}
