//go:build windows

package netbackend

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
)

// New returns the Windows route backend (route.exe with interface index).
func New() Router {
	return NewWindowsRouter(func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	}, interfaceIndex)
}

// NewWindowsRouter builds a Windows router with injectable command and index lookup.
func NewWindowsRouter(run Runner, indexOf func(string) (int, error)) Router {
	if indexOf == nil {
		indexOf = interfaceIndex
	}
	return windowsRouter{run: run, indexOf: indexOf}
}

type windowsRouter struct {
	run     Runner
	indexOf func(string) (int, error)
}

func (router windowsRouter) AddRoutes(interfaceName string, routes []string) error {
	if interfaceName == "" {
		return fmt.Errorf("interface is required")
	}
	if router.run == nil {
		return fmt.Errorf("route runner is not configured")
	}
	index, err := router.indexOf(interfaceName)
	if err != nil {
		return err
	}
	applied := make([]string, 0, len(routes))
	for _, route := range routes {
		if err := validateRouteCIDR(route); err != nil {
			router.DeleteRoutes(interfaceName, applied)
			return err
		}
		network, mask, err := NetworkAndMask(route)
		if err != nil {
			router.DeleteRoutes(interfaceName, applied)
			return err
		}
		// 0.0.0.0 gateway with IF binds the destination route to the VPN interface.
		output, err := router.run("route", "ADD", network, "MASK", mask, "0.0.0.0", "IF", strconv.Itoa(index))
		if err != nil {
			router.DeleteRoutes(interfaceName, applied)
			return fmt.Errorf("add route %s: %s", route, string(output))
		}
		applied = append(applied, route)
	}
	return nil
}

func (router windowsRouter) DeleteRoutes(interfaceName string, routes []string) {
	if interfaceName == "" || router.run == nil {
		return
	}
	index, err := router.indexOf(interfaceName)
	if err != nil {
		return
	}
	for i := len(routes) - 1; i >= 0; i-- {
		network, mask, err := NetworkAndMask(routes[i])
		if err != nil {
			continue
		}
		_, _ = router.run("route", "DELETE", network, "MASK", mask, "IF", strconv.Itoa(index))
	}
}

func interfaceIndex(name string) (int, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return 0, fmt.Errorf("interface %s: %w", name, err)
	}
	return iface.Index, nil
}
