package nativeengine

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

type ProfileNetwork struct {
	Profile    string
	Interface  string
	ProcessID  int
	Routes     []string
	Domains    []string
	DNSServers []string
}

func BuildNetworkPlan(network ProfileNetwork) (Plan, error) {
	if network.Profile == "" || network.Interface == "" || network.ProcessID < 1 {
		return Plan{}, fmt.Errorf("profile, interface and process are required")
	}
	plan := Plan{Profile: network.Profile}
	plan.Mutations = append(plan.Mutations, Mutation{Kind: MutationProcess, Resource: fmt.Sprintf("process:%d", network.ProcessID), PID: network.ProcessID})
	plan.Mutations = append(plan.Mutations, Mutation{Kind: MutationInterface, Resource: "interface:" + network.Interface, Interface: network.Interface})
	prefixes := make([]netip.Prefix, 0, len(network.Routes))
	seenPrefixes := map[string]bool{}
	for _, route := range network.Routes {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(route))
		if err != nil || !prefix.Addr().Is4() {
			return Plan{}, fmt.Errorf("invalid IPv4 route: %s", route)
		}
		prefix = prefix.Masked()
		if prefix.Bits() == 0 {
			return Plan{}, fmt.Errorf("default routes are not allowed")
		}
		if !seenPrefixes[prefix.String()] {
			seenPrefixes[prefix.String()] = true
			prefixes = append(prefixes, prefix)
		}
	}
	if len(prefixes) == 0 {
		return Plan{}, fmt.Errorf("at least one destination route is required")
	}
	sort.Slice(prefixes, func(i, k int) bool {
		if prefixes[i].Bits() == prefixes[k].Bits() {
			return prefixes[i].Addr().Less(prefixes[k].Addr())
		}
		return prefixes[i].Bits() > prefixes[k].Bits()
	})
	for _, prefix := range prefixes {
		plan.Mutations = append(plan.Mutations, Mutation{Kind: MutationRoute, Resource: "route:" + network.Interface + ":" + prefix.String(), Interface: network.Interface, CIDR: prefix.String()})
	}
	dnsServers := []string{}
	for _, server := range network.DNSServers {
		address, err := netip.ParseAddr(strings.TrimSpace(server))
		if err != nil || !address.Is4() {
			return Plan{}, fmt.Errorf("invalid IPv4 DNS server: %s", server)
		}
		dnsServers = append(dnsServers, address.String())
	}
	for _, domain := range network.Domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if !validDomainName(domain) || len(dnsServers) == 0 {
			return Plan{}, fmt.Errorf("invalid split DNS configuration")
		}
		plan.Mutations = append(plan.Mutations, Mutation{Kind: MutationDNS, Resource: "dns:" + domain, Interface: network.Interface, Domain: domain, Values: map[string]string{"servers": strings.Join(dnsServers, ",")}})
	}
	return plan, nil
}
func validDomainName(domain string) bool {
	if domain == "" || len(domain) > 253 || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, character := range label {
			if !(character == '-' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9') {
				return false
			}
		}
	}
	return true
}
