//go:build linux

package netbackend

import "os/exec"

// New returns the Linux route backend (ip route via rtnetlink userspace tool).
func New() Router {
	return NewLinuxRouter(func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	})
}

// NewLinuxRouter builds a Linux router with an injectable command runner (tests).
func NewLinuxRouter(run Runner) Router {
	return commandRouter{
		run: run,
		add: func(interfaceName, route string) (string, []string) {
			// replace keeps reconnects idempotent without leaving duplicates.
			return "ip", []string{"-4", "route", "replace", route, "dev", interfaceName}
		},
		remove: func(interfaceName, route string) (string, []string) {
			return "ip", []string{"-4", "route", "del", route, "dev", interfaceName}
		},
	}
}
