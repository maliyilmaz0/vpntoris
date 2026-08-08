//go:build windows

package netbackend

import (
	"fmt"
	"os/exec"
	"strings"
)

func NewDNS() DNSConfigurator {
	return NewWindowsDNS(func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	})
}
func NewWindowsDNS(run Runner) DNSConfigurator {
	return CommandDNS{
		Run: run,
		Add: func(interfaceName, domain string, servers []string) (string, []string) {
			_ = interfaceName
			script := fmt.Sprintf(
				`Add-DnsClientNrptRule -Namespace ".%s" -NameServers %s -Comment "vpntoris"`,
				domain,
				powershellStringList(servers),
			)
			return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", script}
		},
		Remove: func(interfaceName, domain string) (string, []string) {
			_ = interfaceName
			script := fmt.Sprintf(
				`Get-DnsClientNrptRule | Where-Object { $_.Namespace -eq ".%s" -and $_.Comment -eq "vpntoris" } | Remove-DnsClientNrptRule -Force`,
				domain,
			)
			return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", script}
		},
	}
}
func powershellStringList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, "'"+strings.ReplaceAll(value, "'", "''")+"'")
	}
	return "@(" + strings.Join(quoted, ",") + ")"
}
