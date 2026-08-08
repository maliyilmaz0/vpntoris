package netbackend

import (
	"fmt"
	"net/netip"
	"strings"
)

func NetworkAndMask(cidr string) (network string, mask string, err error) {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(cidr))
	if err != nil || !prefix.Addr().Is4() {
		return "", "", fmt.Errorf("invalid IPv4 route: %s", cidr)
	}
	prefix = prefix.Masked()
	if prefix.Bits() == 0 {
		return "", "", fmt.Errorf("default routes are not allowed")
	}
	ones := prefix.Bits()
	var maskValue uint32
	if ones == 0 {
		maskValue = 0
	} else {
		maskValue = ^uint32(0) << (32 - ones)
	}
	mask = fmt.Sprintf("%d.%d.%d.%d", byte(maskValue>>24), byte(maskValue>>16), byte(maskValue>>8), byte(maskValue))
	return prefix.Addr().String(), mask, nil
}
