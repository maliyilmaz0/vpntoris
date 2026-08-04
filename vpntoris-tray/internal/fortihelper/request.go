package fortihelper

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"vpntoris-tray/internal/openvpnconfig"
)

const (
	ActionStart          = "start"
	ActionOTP            = "otp"
	ActionStop           = "stop"
	ActionStatus         = "status"
	ProtocolFortiGateSSL = "fortigate-ssl"
	ProtocolOpenVPN      = "openvpn"
	ProtocolOpenConnect  = "openconnect"
)

var profilePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,79}$`)
var hostnamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)
var digestPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

type Request struct {
	Action          string   `json:"action"`
	Profile         string   `json:"profile"`
	Protocol        string   `json:"protocol,omitempty"`
	Configuration   string   `json:"configuration,omitempty"`
	Host            string   `json:"host,omitempty"`
	Port            int      `json:"port,omitempty"`
	Username        string   `json:"username,omitempty"`
	Password        string   `json:"password,omitempty"`
	OTP             string   `json:"otp,omitempty"`
	TwoFactor       bool     `json:"twoFactor,omitempty"`
	GatewayProtocol string   `json:"gatewayProtocol,omitempty"`
	TrustedCert     string   `json:"trustedCert,omitempty"`
	Routes          []string `json:"routes,omitempty"`
}

type Response struct {
	State     string `json:"state"`
	Interface string `json:"interface,omitempty"`
	Error     string `json:"error,omitempty"`
	Received  uint64 `json:"received,omitempty"`
	Sent      uint64 `json:"sent,omitempty"`
	Duration  int64  `json:"duration,omitempty"`
}

func (request Request) Validate() error {
	if !profilePattern.MatchString(request.Profile) {
		return fmt.Errorf("invalid profile identifier")
	}
	switch request.Action {
	case ActionStart:
		return request.validateStart()
	case ActionOTP:
		if request.OTP == "" || len(request.OTP) > 128 || strings.ContainsAny(request.OTP, "\r\n\x00") {
			return fmt.Errorf("invalid one-time password")
		}
	case ActionStop, ActionStatus:
	default:
		return fmt.Errorf("invalid action")
	}
	return nil
}

func (request Request) validateStart() error {
	if request.Protocol == "" || request.Protocol == ProtocolFortiGateSSL {
		return request.validateFortiGateStart()
	}
	if request.Protocol == ProtocolOpenVPN {
		return request.validateOpenVPNStart()
	}
	if request.Protocol == ProtocolOpenConnect {
		return request.validateOpenConnectStart()
	}
	return fmt.Errorf("unsupported VPN protocol")
}

func (request Request) validateOpenConnectStart() error {
	if request.Host == "" || net.ParseIP(request.Host) == nil && !hostnamePattern.MatchString(request.Host) || strings.Contains(request.Host, "..") {
		return fmt.Errorf("invalid VPN gateway")
	}
	if request.Port < 1 || request.Port > 65535 {
		return fmt.Errorf("invalid VPN port")
	}
	allowed := map[string]bool{"anyconnect": true, "gp": true, "pulse": true, "nc": true, "f5": true, "fortinet": true, "array": true}
	if !allowed[request.GatewayProtocol] {
		return fmt.Errorf("unsupported OpenConnect gateway protocol")
	}
	if request.Username == "" || len(request.Username) > 256 || strings.ContainsAny(request.Username, "\r\n\x00") {
		return fmt.Errorf("invalid VPN username")
	}
	if request.Password == "" || len(request.Password) > 4096 || strings.ContainsAny(request.Password, "\r\n\x00") {
		return fmt.Errorf("invalid VPN password")
	}
	return request.validateRoutes()
}

func (request Request) validateFortiGateStart() error {
	if request.Host == "" || net.ParseIP(request.Host) == nil && !hostnamePattern.MatchString(request.Host) || strings.Contains(request.Host, "..") {
		return fmt.Errorf("invalid VPN gateway")
	}
	if request.Port < 1 || request.Port > 65535 {
		return fmt.Errorf("invalid VPN port")
	}
	if request.Username == "" || len(request.Username) > 256 || strings.ContainsAny(request.Username, "\r\n\x00") {
		return fmt.Errorf("invalid VPN username")
	}
	if request.Password == "" || len(request.Password) > 4096 || strings.ContainsAny(request.Password, "\r\n\x00") {
		return fmt.Errorf("invalid VPN password")
	}
	if request.OTP != "" && (len(request.OTP) > 128 || strings.ContainsAny(request.OTP, "\r\n\x00")) {
		return fmt.Errorf("invalid one-time password")
	}
	if request.TrustedCert != "" && !digestPattern.MatchString(request.TrustedCert) {
		return fmt.Errorf("invalid trusted certificate digest")
	}
	return request.validateRoutes()
}

func (request Request) validateOpenVPNStart() error {
	if _, err := openvpnconfig.Sanitize(request.Configuration); err != nil {
		return err
	}
	if request.Username != "" && (len(request.Username) > 256 || strings.ContainsAny(request.Username, "\r\n\x00")) {
		return fmt.Errorf("invalid VPN username")
	}
	if request.Password != "" && (len(request.Password) > 4096 || strings.ContainsAny(request.Password, "\r\n\x00")) {
		return fmt.Errorf("invalid VPN password")
	}
	if (request.Username == "") != (request.Password == "") {
		return fmt.Errorf("OpenVPN username and password must be supplied together")
	}
	if request.OTP != "" {
		return fmt.Errorf("OpenVPN start does not accept an inline one-time password")
	}
	return request.validateRoutes()
}

func (request Request) validateRoutes() error {
	if len(request.Routes) == 0 || len(request.Routes) > 64 {
		return fmt.Errorf("one to 64 routes are required")
	}
	seen := make(map[string]bool, len(request.Routes))
	for _, value := range request.Routes {
		ip, network, err := net.ParseCIDR(value)
		if err != nil || ip.To4() == nil || network.String() == "0.0.0.0/0" || network.String() != value || seen[value] {
			return fmt.Errorf("invalid or duplicate IPv4 route: %s", value)
		}
		seen[value] = true
	}
	return nil
}

func (request Request) Arguments() []string {
	arguments := []string{
		net.JoinHostPort(request.Host, strconv.Itoa(request.Port)),
		"--username=" + request.Username,
		"--no-routes",
		"--no-dns",
		"--pppd-no-peerdns",
		"--pppd-ipparam=vpntoris-" + request.Profile,
	}
	if request.TrustedCert != "" {
		arguments = append(arguments, "--trusted-cert="+strings.ToLower(request.TrustedCert))
	}
	return arguments
}
