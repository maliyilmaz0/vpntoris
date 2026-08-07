package netbackend

import (
	"fmt"
	"net/netip"
	"strings"
)

// DNSConfigurator applies scoped split-DNS for a VPN session.
type DNSConfigurator interface {
	AddScoped(interfaceName string, domain string, servers []string) error
	RemoveScoped(interfaceName string, domain string) error
}

// CommandDNS implements DNSConfigurator by invoking platform commands.
type CommandDNS struct {
	Run Runner
	// Add builds the command used to install a scoped domain resolver.
	Add func(interfaceName, domain string, servers []string) (name string, args []string)
	// Remove builds the command used to clear a scoped domain resolver.
	Remove func(interfaceName, domain string) (name string, args []string)
}

func (dns CommandDNS) AddScoped(interfaceName, domain string, servers []string) error {
	if interfaceName == "" {
		return fmt.Errorf("interface is required")
	}
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" || strings.Contains(domain, "..") || strings.HasPrefix(domain, ".") {
		return fmt.Errorf("invalid DNS domain")
	}
	if len(servers) == 0 {
		return fmt.Errorf("at least one DNS server is required")
	}
	normalized := make([]string, 0, len(servers))
	for _, server := range servers {
		address, err := netip.ParseAddr(strings.TrimSpace(server))
		if err != nil || !address.Is4() {
			return fmt.Errorf("invalid IPv4 DNS server: %s", server)
		}
		normalized = append(normalized, address.String())
	}
	if dns.Run == nil || dns.Add == nil {
		return fmt.Errorf("dns runner is not configured")
	}
	name, args := dns.Add(interfaceName, domain, normalized)
	output, err := dns.Run(name, args...)
	if err != nil {
		return fmt.Errorf("add scoped dns %s: %s", domain, strings.TrimSpace(string(output)))
	}
	return nil
}

func (dns CommandDNS) RemoveScoped(interfaceName, domain string) error {
	if dns.Run == nil || dns.Remove == nil || interfaceName == "" || domain == "" {
		return nil
	}
	name, args := dns.Remove(interfaceName, strings.ToLower(strings.TrimSpace(domain)))
	_, _ = dns.Run(name, args...)
	return nil
}
