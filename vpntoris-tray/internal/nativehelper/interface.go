package nativehelper

import (
	"net"
	"os"
	"regexp"
	"strings"
	"vpntoris-tray/internal/fortihelper"
)

var pppUsingPattern = regexp.MustCompile(`(?m)Using interface (ppp[0-9]+)`)
var pppReadyPattern = regexp.MustCompile(`(?m)Interface (ppp[0-9]+) is UP\.`)
var openVPNReadyPattern = regexp.MustCompile(`(?m)(?:Opened (?:utun|tun) device |TUN/TAP device )((?:utun|tun|tap)[0-9]+)`)
var openVPNWindowsReadyPattern = regexp.MustCompile(`(?m)(?:TAP-WIN32 device \[([^\]]+)\] opened|Wintun(?: Userspace Tunnel)? \[([^\]]+)\] opened|Opened tun device \[([^\]]+)\])`)
var openConnectReadyPattern = regexp.MustCompile(`(?m)^VPNTORIS_INTERFACE=((?:utun|tun|ppp)[0-9]+|.+)$`)

func InterfaceFromLogData(data []byte, protocol string) string {
	if protocol == fortihelper.ProtocolOpenVPN {
		if matches := openVPNReadyPattern.FindSubmatch(data); len(matches) == 2 {
			return string(matches[1])
		}
		if matches := openVPNWindowsReadyPattern.FindSubmatch(data); len(matches) > 1 {
			for _, match := range matches[1:] {
				if len(match) > 0 {
					return string(match)
				}
			}
		}
		return ""
	}
	if protocol == fortihelper.ProtocolOpenConnect {
		matches := openConnectReadyPattern.FindSubmatch(data)
		if len(matches) != 2 {
			return ""
		}
		return string(matches[1])
	}
	if matches := pppUsingPattern.FindSubmatch(data); len(matches) == 2 {
		return string(matches[1])
	}
	matches := pppReadyPattern.FindSubmatch(data)
	if len(matches) != 2 {
		return ""
	}
	return string(matches[1])
}

var lookupInterface = defaultLookupInterface

func defaultLookupInterface(name string) bool {
	networkInterface, err := net.InterfaceByName(name)
	return err == nil && networkInterface.Flags&net.FlagUp != 0
}
func interfaceFromLog(path string, protocol string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	interfaceName := InterfaceFromLogData(data, protocol)
	if interfaceName == "" {
		return ""
	}
	if !lookupInterface(interfaceName) {
		return ""
	}
	return interfaceName
}
func logContains(path string, value string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(data), value)
}
