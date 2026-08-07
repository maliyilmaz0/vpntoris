//go:build darwin

package netbackend

import "os/exec"

// New returns the macOS route backend (/sbin/route).
func New() Router {
	return NewDarwinRouter(func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	})
}

// NewDarwinRouter builds a macOS router with an injectable command runner (tests).
func NewDarwinRouter(run Runner) Router {
	return commandRouter{
		run: run,
		add: func(interfaceName, route string) (string, []string) {
			return "/sbin/route", []string{"-n", "add", "-net", route, "-interface", interfaceName}
		},
		remove: func(interfaceName, route string) (string, []string) {
			return "/sbin/route", []string{"-n", "delete", "-net", route, "-interface", interfaceName}
		},
	}
}
