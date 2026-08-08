package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func nativeProfileID(name string) string {
	digest := sha256.Sum256([]byte(name))
	return "profile-" + hex.EncodeToString(digest[:8])
}
func nativeSplitDNS(config VPNConfig) (domains, servers []string) {
	return splitValues(config.Domains), splitValues(config.DNSServers)
}
func withDNSServerRoutes(routes, servers []string) []string {
	if len(servers) == 0 {
		return routes
	}
	out := append([]string{}, routes...)
	seen := make(map[string]bool, len(out))
	for _, route := range out {
		seen[route] = true
	}
	for _, server := range servers {
		server = strings.TrimSpace(server)
		if server == "" {
			continue
		}
		cidr := server + "/32"
		if !seen[cidr] {
			out = append(out, cidr)
			seen[cidr] = true
		}
	}
	return out
}
