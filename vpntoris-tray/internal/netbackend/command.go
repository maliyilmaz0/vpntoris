package netbackend

import (
	"fmt"
	"net/netip"
	"strings"
)

// Runner executes an external command and returns combined output.
type Runner func(name string, args ...string) ([]byte, error)

// commandRouter implements Router by invoking platform route commands through Runner.
type commandRouter struct {
	run    Runner
	add    func(interfaceName, route string) (name string, args []string)
	remove func(interfaceName, route string) (name string, args []string)
}

func (router commandRouter) AddRoutes(interfaceName string, routes []string) error {
	if interfaceName == "" {
		return fmt.Errorf("interface is required")
	}
	if router.run == nil || router.add == nil {
		return fmt.Errorf("route runner is not configured")
	}
	applied := make([]string, 0, len(routes))
	for _, route := range routes {
		if err := validateRouteCIDR(route); err != nil {
			router.DeleteRoutes(interfaceName, applied)
			return err
		}
		name, args := router.add(interfaceName, route)
		output, err := router.run(name, args...)
		if err != nil {
			router.DeleteRoutes(interfaceName, applied)
			return fmt.Errorf("add route %s: %s", route, strings.TrimSpace(string(output)))
		}
		applied = append(applied, route)
	}
	return nil
}

func (router commandRouter) DeleteRoutes(interfaceName string, routes []string) {
	if interfaceName == "" || router.run == nil || router.remove == nil {
		return
	}
	for index := len(routes) - 1; index >= 0; index-- {
		name, args := router.remove(interfaceName, routes[index])
		_, _ = router.run(name, args...)
	}
}

func validateRouteCIDR(route string) error {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(route))
	if err != nil || !prefix.Addr().Is4() {
		return fmt.Errorf("invalid IPv4 route: %s", route)
	}
	if prefix.Bits() == 0 {
		return fmt.Errorf("default routes are not allowed")
	}
	return nil
}
