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
	ActionReset          = "reset"
	ProtocolFortiGateSSL = "fortigate-ssl"
	ProtocolOpenVPN      = "openvpn"
	ProtocolOpenConnect  = "openconnect"
	ProtocolIPSec        = "ipsec"
)

var profilePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,79}$`)
var hostnamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)
var digestPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

type Request struct {
	Action          string        `json:"action"`
	Profile         string        `json:"profile"`
	Protocol        string        `json:"protocol,omitempty"`
	Configuration   string        `json:"configuration,omitempty"`
	Host            string        `json:"host,omitempty"`
	Port            int           `json:"port,omitempty"`
	Username        string        `json:"username,omitempty"`
	Password        string        `json:"password,omitempty"`
	OTP             string        `json:"otp,omitempty"`
	TwoFactor       bool          `json:"twoFactor,omitempty"`
	GatewayProtocol string        `json:"gatewayProtocol,omitempty"`
	ExternalBrowser bool          `json:"externalBrowser,omitempty"`
	TrustedCert     string        `json:"trustedCert,omitempty"`
	Routes          []string      `json:"routes,omitempty"`
	Domains         []string      `json:"domains,omitempty"`
	DNSServers      []string      `json:"dnsServers,omitempty"`
	IPSec           *IPSecRequest `json:"ipsec,omitempty"`
}
type IPSecRequest struct {
	Version       int    `json:"version"`
	AuthMode      string `json:"authMode"`
	PreSharedKey  string `json:"preSharedKey"`
	LocalID       string `json:"localID"`
	RemoteID      string `json:"remoteID"`
	ModeConfig    bool   `json:"modeConfig"`
	Aggressive    bool   `json:"aggressive"`
	MOBIKE        bool   `json:"mobike"`
	ForceEncap    bool   `json:"forceEncap"`
	Fragmentation string `json:"fragmentation"`
	DPDAction     string `json:"dpdAction"`
	DPDDelay      int    `json:"dpdDelay"`
	DPDTimeout    int    `json:"dpdTimeout"`
	IKELifetime   int    `json:"ikeLifetime"`
	ChildLifetime int    `json:"childLifetime"`
	ReplayWindow  int    `json:"replayWindow"`
	IKEProposals  string `json:"ikeProposals"`
	ESPProposals  string `json:"espProposals"`
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
	case ActionReset:
		return nil
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
	if request.Protocol == ProtocolIPSec {
		return request.validateIPSecStart()
	}
	return fmt.Errorf("unsupported VPN protocol")
}
func (request Request) validateIPSecStart() error {
	if request.Host == "" || net.ParseIP(request.Host) == nil && !hostnamePattern.MatchString(request.Host) || strings.Contains(request.Host, "..") {
		return fmt.Errorf("invalid VPN gateway")
	}
	if request.IPSec == nil {
		return fmt.Errorf("IPsec settings are required")
	}
	settings := request.IPSec
	if settings.Version != 1 && settings.Version != 2 {
		return fmt.Errorf("invalid IKE version")
	}
	if settings.AuthMode != "none" && settings.AuthMode != "xauth" && settings.AuthMode != "eap" {
		return fmt.Errorf("invalid IPsec authentication mode")
	}
	if settings.Version == 1 && settings.AuthMode == "eap" {
		return fmt.Errorf("IKEv1 does not support EAP")
	}
	if settings.PreSharedKey == "" || len(settings.PreSharedKey) > 4096 || strings.ContainsAny(settings.PreSharedKey, "\r\n\x00") {
		return fmt.Errorf("invalid IPsec pre-shared key")
	}
	if settings.AuthMode != "none" && (request.Username == "" || request.Password == "") {
		return fmt.Errorf("IPsec extended authentication credentials are required")
	}
	for _, value := range []string{request.Username, request.Password, settings.LocalID, settings.RemoteID} {
		if len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("invalid IPsec identity or credential")
		}
	}
	proposalPattern := regexp.MustCompile(`^[a-z0-9_-]+(?:-[a-z0-9_-]+)*(?:,[a-z0-9_-]+(?:-[a-z0-9_-]+)*)*$`)
	if !proposalPattern.MatchString(settings.IKEProposals) || !proposalPattern.MatchString(settings.ESPProposals) {
		return fmt.Errorf("invalid IPsec proposal")
	}
	if settings.Fragmentation != "yes" && settings.Fragmentation != "no" && settings.Fragmentation != "accept" {
		return fmt.Errorf("invalid fragmentation setting")
	}
	if settings.DPDAction != "none" && settings.DPDAction != "clear" && settings.DPDAction != "hold" && settings.DPDAction != "restart" {
		return fmt.Errorf("invalid DPD action")
	}
	if settings.DPDDelay < 0 || settings.DPDDelay > 3600 || settings.DPDTimeout < 0 || settings.DPDTimeout > 86400 || settings.IKELifetime < 60 || settings.IKELifetime > 604800 || settings.ChildLifetime < 60 || settings.ChildLifetime > 604800 || settings.ReplayWindow < 1 || settings.ReplayWindow > 4096 {
		return fmt.Errorf("invalid IPsec timing setting")
	}
	return request.validateRoutes()
}
func (request Request) IPSecConfiguration() string {
	settings := request.IPSec
	localID := settings.LocalID
	if localID == "" {
		localID = request.Username
	}
	remoteID := settings.RemoteID
	if remoteID == "" {
		remoteID = "%any"
	}
	vips := ""
	if settings.ModeConfig {
		vips = "    vips = 0.0.0.0\n"
	}
	aggressive := ""
	if settings.Version == 1 && settings.Aggressive {
		aggressive = "    aggressive = yes\n"
	}
	auth := fmt.Sprintf("    local {\n      auth = psk\n      id = %q\n    }\n    remote {\n      auth = psk\n      id = %q\n    }\n", localID, remoteID)
	secrets := fmt.Sprintf("  ike-%s {\n    id = %q\n    secret = %q\n  }\n", request.Profile, localID, settings.PreSharedKey)
	if settings.AuthMode == "xauth" {
		auth += fmt.Sprintf("    local-xauth {\n      auth = xauth\n      xauth_id = %q\n    }\n", request.Username)
		secrets += fmt.Sprintf("  xauth-%s {\n    id = %q\n    secret = %q\n  }\n", request.Profile, request.Username, request.Password)
	} else if settings.AuthMode == "eap" {
		auth += fmt.Sprintf("    local-eap {\n      auth = eap\n      eap_id = %q\n    }\n", request.Username)
		secrets += fmt.Sprintf("  eap-%s {\n    id = %q\n    secret = %q\n  }\n", request.Profile, request.Username, request.Password)
	}
	return fmt.Sprintf("connections {\n  %s {\n    version = %d\n    remote_addrs = %s\n    proposals = %s\n    rekey_time = %ds\n    dpd_delay = %ds\n    dpd_timeout = %ds\n    fragmentation = %s\n    mobike = %t\n    encap = %t\n%s%s%s    children {\n      net-%s {\n        local_ts = dynamic\n        remote_ts = %s\n        esp_proposals = %s\n        life_time = %ds\n        replay_window = %d\n        dpd_action = %s\n      }\n    }\n  }\n}\nsecrets {\n%s}\n", request.Profile, settings.Version, request.Host, settings.IKEProposals, settings.IKELifetime, settings.DPDDelay, settings.DPDTimeout, settings.Fragmentation, settings.MOBIKE, settings.ForceEncap, aggressive, vips, auth, request.Profile, strings.Join(request.Routes, ","), settings.ESPProposals, settings.ChildLifetime, settings.ReplayWindow, settings.DPDAction, secrets)
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
	if (!request.ExternalBrowser && request.Username == "") || len(request.Username) > 256 || strings.ContainsAny(request.Username, "\r\n\x00") {
		return fmt.Errorf("invalid VPN username")
	}
	if (!request.ExternalBrowser && request.Password == "") || len(request.Password) > 4096 || strings.ContainsAny(request.Password, "\r\n\x00") {
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
	return request.validateSplitDNS()
}
func (request Request) validateSplitDNS() error {
	if len(request.Domains) == 0 && len(request.DNSServers) == 0 {
		return nil
	}
	if len(request.Domains) == 0 || len(request.DNSServers) == 0 {
		return fmt.Errorf("split DNS requires both domains and DNS servers")
	}
	if len(request.Domains) > 32 || len(request.DNSServers) > 8 {
		return fmt.Errorf("too many split DNS domains or servers")
	}
	for _, domain := range request.Domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "" || len(domain) > 253 || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "/") {
			return fmt.Errorf("invalid split DNS domain: %s", domain)
		}
	}
	for _, server := range request.DNSServers {
		ip := net.ParseIP(strings.TrimSpace(server))
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("invalid IPv4 DNS server: %s", server)
		}
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
