package nativehelper

import (
	"fmt"
	"net"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// openVPNManagementArgs returns OpenVPN CLI management arguments and the dial target
// used by manageOpenVPN. On Unix the management channel is a domain socket; on
// Windows it is a localhost TCP listener owned by OpenVPN.
func openVPNManagementArgs(runtimeDir, profile, configPath string) (arguments []string, dialTarget string, err error) {
	if runtime.GOOS == "windows" {
		port, portErr := freeLocalTCPPort()
		if portErr != nil {
			return nil, "", portErr
		}
		// OpenVPN listens on this TCP endpoint for the management interface.
		arguments = []string{"--management", "127.0.0.1", strconv.Itoa(port), "--management-query-passwords", "--management-hold", "--auth-retry", "interact"}
		return arguments, "tcp:127.0.0.1:" + strconv.Itoa(port), nil
	}
	managementPath := strings.TrimSuffix(configPath, ".ovpn") + ".sock"
	if managementPath == configPath {
		managementPath = configPath + ".sock"
	}
	arguments = []string{"--management", managementPath, "unix", "--management-query-passwords", "--management-hold", "--auth-retry", "interact"}
	return arguments, managementPath, nil
}

func dialManagement(target string, timeout time.Duration) (net.Conn, error) {
	if strings.HasPrefix(target, "tcp:") {
		return net.DialTimeout("tcp", strings.TrimPrefix(target, "tcp:"), timeout)
	}
	return net.DialTimeout("unix", target, timeout)
}

func freeLocalTCPPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("could not reserve management port: %w", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
