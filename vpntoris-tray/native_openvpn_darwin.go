//go:build darwin

package main

import (
	"fmt"
	"strings"
	"time"

	"vpntoris-tray/internal/fortihelper"
	"vpntoris-tray/internal/openvpnconfig"
)

func nativeOpenVPNSupported(config VPNConfig) bool {
	return config.Type == "openvpn" && nativeHelperReady()
}

func nativeOpenVPNConnect(config VPNConfig) error {
	if config.TwoFactor {
		return fmt.Errorf("native OpenVPN interactive OTP is not supported yet")
	}
	configuration := config.Config
	if strings.TrimSpace(config.Host) != "" {
		configuration = overrideOpenVPNRemote(configuration, config.Host, config.Port)
	}
	configuration, err := openvpnconfig.Sanitize(configuration)
	if err != nil {
		return err
	}
	routes, err := parseRoutes(config.Routes)
	if err != nil {
		return err
	}
	values := make([]string, 0, len(routes))
	for _, route := range routes {
		values = append(values, fmt.Sprintf("%s/%d", route.network, route.prefix))
	}
	request := fortihelper.Request{Action: fortihelper.ActionStart, Profile: nativeProfileID(config.Name), Protocol: fortihelper.ProtocolOpenVPN, Configuration: configuration, Username: config.User, Password: config.Password, Routes: values}
	response, err := nativeFortiRequest(request)
	request.Password = ""
	request.Configuration = ""
	if err != nil {
		return err
	}
	if response.Error != "" {
		return fmt.Errorf("%s", response.Error)
	}
	deadline := time.Now().Add(50 * time.Second)
	for time.Now().Before(deadline) {
		status, statusError := nativeFortiStatus(config.Name)
		if statusError != nil {
			return statusError
		}
		if status.State == "connected" {
			return nil
		}
		if status.State == "failed" {
			return fmt.Errorf("native OpenVPN tunnel failed: %s", status.Error)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("native OpenVPN tunnel did not become ready before timeout")
}
