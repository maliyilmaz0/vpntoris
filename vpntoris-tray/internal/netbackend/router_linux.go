//go:build linux

package netbackend

import "os/exec"

func New() Router {
	return NewLinuxRouter(func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	})
}
func NewLinuxRouter(run Runner) Router {
	return commandRouter{
		run: run,
		add: func(interfaceName, route string) (string, []string) {
			return "ip", []string{"-4", "route", "replace", route, "dev", interfaceName}
		},
		remove: func(interfaceName, route string) (string, []string) {
			return "ip", []string{"-4", "route", "del", route, "dev", interfaceName}
		},
	}
}
