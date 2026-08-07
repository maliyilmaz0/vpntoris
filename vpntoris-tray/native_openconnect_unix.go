//go:build unix

package main

import (
	"fmt"
	"strconv"
	"time"

	"vpntoris-tray/internal/fortihelper"
)

func nativeOpenConnectSupported(config VPNConfig) bool {
	return config.Type == "openconnect" && openConnectProtocol(config) != "" && nativeHelperReady()
}

func nativeOpenConnectNeedsOTP(name string) bool {
	status, err := nativeFortiStatus(name)
	return err == nil && status.State == "waiting-otp"
}

func nativeOpenConnectTraffic(name string) (uint64, uint64, int64, error) {
	return nativeFortiTraffic(name)
}

func nativeOpenConnectConnect(config VPNConfig) error {
	port, err := strconv.Atoi(config.Port)
	if err != nil {
		return fmt.Errorf("invalid VPN port")
	}
	routes, err := parseRoutes(config.Routes)
	if err != nil {
		return err
	}
	values := make([]string, 0, len(routes))
	for _, route := range routes {
		values = append(values, fmt.Sprintf("%s/%d", route.network, route.prefix))
	}
	request := fortihelper.Request{Action: fortihelper.ActionStart, Profile: nativeProfileID(config.Name), Protocol: fortihelper.ProtocolOpenConnect, GatewayProtocol: openConnectProtocol(config), Host: config.Host, Port: port, Username: config.User, Password: config.Password, TwoFactor: config.TwoFactor, ExternalBrowser: config.ExternalBrowser, Routes: values}
	response, err := nativeFortiRequest(request)
	request.Password = ""
	if err != nil {
		return err
	}
	if response.Error != "" {
		return fmt.Errorf("%s", response.Error)
	}
	deadline := time.Now().Add(50 * time.Second)
	if config.TwoFactor {
		deadline = time.Now().Add(190 * time.Second)
	}
	for time.Now().Before(deadline) {
		status, statusError := nativeFortiStatus(config.Name)
		if statusError != nil {
			return statusError
		}
		if status.State == "connected" {
			return nil
		}
		if status.State == "failed" {
			return fmt.Errorf("native OpenConnect tunnel failed: %s", status.Error)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("native OpenConnect tunnel did not become ready before timeout")
}
