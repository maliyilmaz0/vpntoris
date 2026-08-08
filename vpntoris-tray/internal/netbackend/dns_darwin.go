//go:build darwin

package netbackend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func NewDNS() DNSConfigurator {
	return darwinDNS{resolverRoot: "/etc/resolver"}
}

type darwinDNS struct {
	resolverRoot string
}

func (dns darwinDNS) AddScoped(interfaceName, domain string, servers []string) error {
	_ = interfaceName
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" || strings.Contains(domain, "/") || strings.Contains(domain, "..") {
		return fmt.Errorf("invalid DNS domain")
	}
	if err := os.MkdirAll(dns.resolverRoot, 0755); err != nil {
		return err
	}
	var builder strings.Builder
	for _, server := range servers {
		builder.WriteString("nameserver ")
		builder.WriteString(server)
		builder.WriteByte('\n')
	}
	return os.WriteFile(filepath.Join(dns.resolverRoot, domain), []byte(builder.String()), 0644)
}
func (dns darwinDNS) RemoveScoped(interfaceName, domain string) error {
	_ = interfaceName
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" || strings.Contains(domain, "/") || strings.Contains(domain, "..") {
		return nil
	}
	_ = os.Remove(filepath.Join(dns.resolverRoot, domain))
	return nil
}
