//go:build linux

package netbackend

import (
	"os/exec"
	"strings"
)

// NewDNS returns the Linux split-DNS backend (systemd-resolved via resolvectl).
func NewDNS() DNSConfigurator {
	return NewLinuxDNS(func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	})
}

// NewLinuxDNS builds a resolvectl-backed configurator with an injectable runner.
func NewLinuxDNS(run Runner) DNSConfigurator {
	return CommandDNS{
		Run: run,
		Add: func(interfaceName, domain string, servers []string) (string, []string) {
			// First set interface DNS servers, then scope the domain with a routing-only suffix.
			// Callers that need both steps use ApplyLinuxSplitDNS.
			args := append([]string{"dns", interfaceName}, servers...)
			return "resolvectl", args
		},
		Remove: func(interfaceName, domain string) (string, []string) {
			return "resolvectl", []string{"revert", interfaceName}
		},
	}
}

// ApplyLinuxSplitDNS installs DNS servers and a ~domain routing domain on an interface.
func ApplyLinuxSplitDNS(run Runner, interfaceName, domain string, servers []string) error {
	dns := NewLinuxDNS(run).(CommandDNS)
	if err := dns.AddScoped(interfaceName, domain, servers); err != nil {
		return err
	}
	name, args := "resolvectl", []string{"domain", interfaceName, "~" + strings.ToLower(strings.TrimSpace(domain))}
	output, err := run(name, args...)
	if err != nil {
		_ = dns.RemoveScoped(interfaceName, domain)
		return err
	}
	_ = output
	return nil
}
