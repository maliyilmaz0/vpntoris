//go:build windows

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
func nativeOpenVPNNeedsOTP(name string) bool {
	status, err := nativeFortiStatus(name)
	return err == nil && status.State == "waiting-otp"
}
func nativeOpenVPNTraffic(name string) (uint64, uint64, int64, error) {
	status, err := nativeFortiStatus(name)
	if err != nil {
		return 0, 0, 0, err
	}
	if status.State != "connected" {
		return 0, 0, 0, fmt.Errorf("native OpenVPN tunnel is not connected")
	}
	return status.Received, status.Sent, status.Duration, nil
}
func nativeOpenVPNConnect(config VPNConfig) error {
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
	domains, dnsServers := nativeSplitDNS(config)
	values = withDNSServerRoutes(values, dnsServers)
	request := fortihelper.Request{
		Action:        fortihelper.ActionStart,
		Profile:       nativeProfileID(config.Name),
		Protocol:      fortihelper.ProtocolOpenVPN,
		Configuration: configuration,
		Username:      config.User,
		Password:      config.Password,
		TwoFactor:     config.TwoFactor,
		Routes:        values,
		Domains:       domains,
		DNSServers:    dnsServers,
	}
	response, err := nativeHelperRequest(request)
	request.Password = ""
	request.Configuration = ""
	if err != nil {
		return err
	}
	if response.Error != "" {
		return fmt.Errorf("%s", response.Error)
	}
	return waitNativeConnected(config.Name, 50*time.Second)
}
