package openvpnconfig

import (
	"bufio"
	"fmt"
	"strings"
)

var rejected = map[string]bool{
	"auth-user-pass-verify":   true,
	"cd":                      true,
	"chroot":                  true,
	"client-connect":          true,
	"client-disconnect":       true,
	"config":                  true,
	"daemon":                  true,
	"down":                    true,
	"ipchange":                true,
	"learn-address":           true,
	"log":                     true,
	"log-append":              true,
	"management":              true,
	"management-client":       true,
	"management-external-key": true,
	"plugin":                  true,
	"route-pre-down":          true,
	"route-up":                true,
	"status":                  true,
	"tls-verify":              true,
	"up":                      true,
	"writepid":                true,
}
var removed = map[string]bool{
	"auth-user-pass":    true,
	"block-outside-dns": true,
	"dhcp-option":       true,
	"redirect-gateway":  true,
	"redirect-private":  true,
	"route":             true,
	"route-ipv6":        true,
	"route-nopull":      true,
	"script-security":   true,
}

func Sanitize(configuration string) (string, error) {
	if len(configuration) == 0 || len(configuration) > 1024*1024 || strings.ContainsRune(configuration, '\x00') {
		return "", fmt.Errorf("invalid OpenVPN configuration size or encoding")
	}
	scanner := bufio.NewScanner(strings.NewReader(configuration))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	result := make([]string, 0)
	inline := ""
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if inline != "" {
			if strings.EqualFold(trimmed, "</"+inline+">") {
				result = append(result, line)
				inline = ""
				continue
			}
			result = append(result, line)
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			result = append(result, line)
			continue
		}
		if strings.HasPrefix(trimmed, "<") && strings.HasSuffix(trimmed, ">") {
			name := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(trimmed, "<"), ">"))
			if name == "auth-user-pass" || name == "connection" {
				return "", fmt.Errorf("unsafe inline OpenVPN block: %s", name)
			}
			if name != "ca" && name != "cert" && name != "key" && name != "tls-auth" && name != "tls-crypt" && name != "tls-crypt-v2" && name != "pkcs12" {
				return "", fmt.Errorf("unsupported inline OpenVPN block: %s", name)
			}
			inline = name
			result = append(result, line)
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		directive := strings.ToLower(strings.TrimLeft(fields[0], "-"))
		if rejected[directive] {
			return "", fmt.Errorf("unsafe OpenVPN directive: %s", directive)
		}
		if removed[directive] {
			continue
		}
		if directive == "dev" && len(fields) > 1 && !strings.HasPrefix(strings.ToLower(fields[1]), "tun") {
			return "", fmt.Errorf("only tunnel-mode OpenVPN profiles are supported")
		}
		result = append(result, line)
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if inline != "" {
		return "", fmt.Errorf("unterminated inline OpenVPN block: %s", inline)
	}
	result = append(result, "route-nopull", "script-security 1", "auth-nocache")
	return strings.Join(result, "\n") + "\n", nil
}
