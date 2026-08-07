//go:build windows

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"vpntoris-tray/internal/fortihelper"
	"vpntoris-tray/internal/helperipc"
	"vpntoris-tray/internal/runtimepaths"
)

func nativeHelperReady() bool {
	return helperipc.Ready()
}

func nativeFortiDisconnect(name string) error {
	response, err := nativeHelperRequest(fortihelper.Request{Action: fortihelper.ActionStop, Profile: nativeProfileID(name)})
	if err != nil {
		return err
	}
	if response.Error != "" {
		return fmt.Errorf("%s", response.Error)
	}
	return nil
}

func nativeFortiReset() error {
	response, err := nativeHelperRequest(fortihelper.Request{Action: fortihelper.ActionReset, Profile: "reset"})
	if err != nil {
		return err
	}
	if response.Error != "" {
		return fmt.Errorf("%s", response.Error)
	}
	return nil
}

func nativeFortiOTP(name, otp string) error {
	response, err := nativeHelperRequest(fortihelper.Request{Action: fortihelper.ActionOTP, Profile: nativeProfileID(name), OTP: strings.TrimSpace(otp)})
	if err != nil {
		return err
	}
	if response.Error != "" {
		return fmt.Errorf("%s", response.Error)
	}
	return nil
}

func nativeFortiStatus(name string) (*fortihelper.Response, error) {
	return nativeHelperRequest(fortihelper.Request{Action: fortihelper.ActionStatus, Profile: nativeProfileID(name)})
}

func nativeFortiConnected(name string) bool {
	response, err := nativeFortiStatus(name)
	return err == nil && response.State == "connected"
}

func nativeFortiInterface(name string) string {
	response, err := nativeFortiStatus(name)
	if err != nil || response.State != "connected" {
		return ""
	}
	return response.Interface
}

func nativeFortiLogs(name string) ([]byte, error) {
	path := runtimepaths.Current().ProfileLog(nativeProfileID(name))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 300 {
		lines = lines[len(lines)-300:]
	}
	return []byte(strings.Join(lines, "\n")), nil
}

func nativeHelperRequest(request fortihelper.Request) (*fortihelper.Response, error) {
	return helperipc.Call(request)
}

func waitNativeConnected(name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := nativeFortiStatus(name)
		if err != nil {
			return err
		}
		if status.State == "connected" {
			return nil
		}
		if status.State == "failed" {
			return fmt.Errorf("native VPN tunnel failed: %s", status.Error)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("native VPN tunnel did not become ready before timeout")
}
