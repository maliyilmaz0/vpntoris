//go:build unix

package main

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"vpntoris-tray/internal/fortihelper"
	"vpntoris-tray/internal/runtimepaths"
)

func nativeFortiSupported(config VPNConfig) bool {
	return config.Type == "openfortivpn" && nativeHelperReady()
}

func nativeHelperReady() bool {
	info, err := os.Stat(runtimepaths.Current().HelperSocket)
	return err == nil && info.Mode()&os.ModeSocket != 0
}

func nativeFortiConnect(config VPNConfig) error {
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
	certificate, err := nativeGatewayCertificate(config.Host, port)
	if err != nil {
		return err
	}
	request := fortihelper.Request{Action: fortihelper.ActionStart, Profile: nativeProfileID(config.Name), Host: config.Host, Port: port, Username: config.User, Password: config.Password, TrustedCert: certificate, Routes: values}
	response, err := nativeFortiRequest(request)
	request.Password = ""
	if err != nil {
		return err
	}
	if response.Error != "" {
		return fmt.Errorf("%s", response.Error)
	}
	if config.TwoFactor {
		otpRequests.Lock()
		otpRequests.names[config.Name] = true
		otpRequests.Unlock()
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
			otpRequests.Lock()
			delete(otpRequests.names, config.Name)
			otpRequests.Unlock()
			return nil
		}
		if status.State == "failed" {
			return fmt.Errorf("native VPN tunnel failed: %s", status.Error)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("native VPN tunnel did not become ready before timeout")
}

func nativeFortiDisconnect(name string) error {
	response, err := nativeFortiRequest(fortihelper.Request{Action: fortihelper.ActionStop, Profile: nativeProfileID(name)})
	if err != nil {
		return err
	}
	if response.Error != "" {
		return fmt.Errorf("%s", response.Error)
	}
	return nil
}

func nativeFortiReset() error {
	response, err := nativeFortiRequest(fortihelper.Request{Action: fortihelper.ActionReset, Profile: "reset"})
	if err != nil {
		return err
	}
	if response.Error != "" {
		return fmt.Errorf("%s", response.Error)
	}
	return nil
}

func nativeFortiOTP(name, otp string) error {
	response, err := nativeFortiRequest(fortihelper.Request{Action: fortihelper.ActionOTP, Profile: nativeProfileID(name), OTP: strings.TrimSpace(otp)})
	if err != nil {
		return err
	}
	if response.Error != "" {
		return fmt.Errorf("%s", response.Error)
	}
	return nil
}

func nativeFortiStatus(name string) (*fortihelper.Response, error) {
	return nativeFortiRequest(fortihelper.Request{Action: fortihelper.ActionStatus, Profile: nativeProfileID(name)})
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

func nativeFortiTraffic(name string) (uint64, uint64, int64, error) {
	status, err := nativeFortiStatus(name)
	if err != nil {
		return 0, 0, 0, err
	}
	if status.State != "connected" || status.Interface == "" {
		return 0, 0, 0, fmt.Errorf("native VPN tunnel is not connected")
	}
	received, sent, err := readInterfaceCounters(status.Interface)
	return received, sent, status.Duration, err
}

func nativeFortiRequest(request fortihelper.Request) (*fortihelper.Response, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	connection, err := net.DialTimeout("unix", runtimepaths.Current().HelperSocket, 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer connection.Close()
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return nil, err
	}
	var response fortihelper.Response
	if err := json.NewDecoder(connection).Decode(&response); err != nil {
		return nil, err
	}
	return &response, nil
}

func nativeGatewayCertificate(host string, port int) (string, error) {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	configuration := &tls.Config{InsecureSkipVerify: true}
	if net.ParseIP(host) == nil {
		configuration.ServerName = host
	}
	connection, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(host, strconv.Itoa(port)), configuration)
	if err != nil {
		return "", fmt.Errorf("could not inspect VPN gateway certificate: %w", err)
	}
	defer connection.Close()
	certificates := connection.ConnectionState().PeerCertificates
	if len(certificates) == 0 {
		return "", fmt.Errorf("VPN gateway did not provide a certificate")
	}
	digest := sha256.Sum256(certificates[0].Raw)
	return hex.EncodeToString(digest[:]), nil
}
