package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const imageName = "vpntoris-client"

var safeNamePattern = regexp.MustCompile(`[^a-z0-9_.-]+`)

type VPNConfig struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Type        string       `json:"type"`
	Host        string       `json:"host"`
	Port        string       `json:"port"`
	User        string       `json:"user"`
	Password    string       `json:"password"`
	TwoFactor   bool         `json:"twoFactor"`
	Routes      string       `json:"routes"`
	Config      string       `json:"config"`
	IPSec       *IPSecConfig `json:"ipsec,omitempty"`
}

type IPSecConfig struct {
	IKEVersion      int             `json:"ikeVersion"`
	IKEMode         string          `json:"ikeMode"`
	AuthMode        string          `json:"authMode"`
	PreSharedKey    string          `json:"preSharedKey"`
	LocalID         string          `json:"localID"`
	RemoteID        string          `json:"remoteID"`
	ModeConfig      bool            `json:"modeConfig"`
	NATTraversal    bool            `json:"natTraversal"`
	ForceEncap      bool            `json:"forceEncap"`
	MOBIKE          bool            `json:"mobike"`
	Fragmentation   string          `json:"fragmentation"`
	DPDAction       string          `json:"dpdAction"`
	DPDDelay        int             `json:"dpdDelay"`
	DPDTimeout      int             `json:"dpdTimeout"`
	IKELifetime     int             `json:"ikeLifetime"`
	IKEEncryption   string          `json:"ikeEncryption"`
	IKEIntegrity    string          `json:"ikeIntegrity"`
	IKEPRF          string          `json:"ikePRF"`
	DHGroups        string          `json:"dhGroups"`
	ChildLifetime   int             `json:"childLifetime"`
	ChildLifetimeKB int             `json:"childLifetimeKB"`
	ESPEncryption   string          `json:"espEncryption"`
	ESPIntegrity    string          `json:"espIntegrity"`
	PFS             bool            `json:"pfs"`
	PFSGroups       string          `json:"pfsGroups"`
	ReplayWindow    int             `json:"replayWindow"`
	LocalSelectors  string          `json:"localSelectors"`
	RemoteSelectors string          `json:"remoteSelectors"`
	Phase2Proposals []IPSecProposal `json:"phase2Proposals,omitempty"`
}

type IPSecProposal struct {
	Encryption     string `json:"encryption"`
	Authentication string `json:"authentication"`
}

var configPath string

const pacAddress = "127.0.0.1:17984"

var proxyState = struct {
	sync.RWMutex
	mappings map[string]proxyMapping
	revision uint64
}{mappings: make(map[string]proxyMapping)}

type proxyMapping struct {
	port   string
	routes []proxyRoute
}

type proxyRoute struct {
	network string
	mask    string
	prefix  int
}

func main() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir, err = os.UserHomeDir()
		if err != nil {
			configDir = os.TempDir()
		}
	}
	configDir = filepath.Join(configDir, "VPNToris")
	_ = os.MkdirAll(configDir, 0700)
	configPath = filepath.Join(configDir, "configs.json")
	if err := startPACServer(); err != nil {
		os.Exit(1)
	}
	go restoreHealthyRoutes()
	if len(os.Args) > 2 && os.Args[1] == "--daemon" {
		if parentPID, err := strconv.Atoi(os.Args[2]); err == nil {
			go func() {
				for {
					time.Sleep(time.Second)
					if syscall.Kill(parentPID, 0) != nil {
						os.Exit(0)
					}
				}
			}()
		}
	}
	select {}
}

func proxyPort(container string) (string, error) {
	output, err := exec.Command("docker", "port", container, "1080/tcp").Output()
	if err != nil {
		return "", fmt.Errorf("could not find the VPN proxy port")
	}
	address := strings.TrimSpace(strings.Split(string(output), "\n")[0])
	separator := strings.LastIndex(address, ":")
	if separator < 0 || separator == len(address)-1 {
		return "", fmt.Errorf("invalid VPN proxy address: %s", address)
	}
	return address[separator+1:], nil
}

func setSystemRoutes(key, port, routeList string, enabled bool) error {
	routes, err := parseRoutes(routeList)
	if enabled && err != nil {
		return err
	}
	proxyState.RLock()
	previous := proxyState.mappings[key]
	proxyState.RUnlock()
	routerRoutes := routes
	if !enabled {
		routerRoutes = previous.routes
	}
	if err := runRootRouter(key, port, routerRoutes, enabled); err != nil {
		return err
	}
	proxyState.Lock()
	if enabled {
		proxyState.mappings[key] = proxyMapping{port: port, routes: routes}
	} else {
		delete(proxyState.mappings, key)
	}
	proxyState.revision++
	proxyEnabled := len(proxyState.mappings) > 0
	revision := proxyState.revision
	proxyState.Unlock()

	output, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return fmt.Errorf("could not list macOS network services: %w", err)
	}
	for index, line := range strings.Split(string(output), "\n") {
		service := strings.TrimSpace(line)
		if index == 0 || service == "" || strings.HasPrefix(service, "*") {
			continue
		}
		_ = exec.Command("networksetup", "-setsocksfirewallproxystate", service, "off").Run()
		state := "off"
		if proxyEnabled {
			state = "on"
			pacURL := fmt.Sprintf("http://%s/proxy.pac?v=%d", pacAddress, revision)
			if result, err := exec.Command("networksetup", "-setautoproxyurl", service, pacURL).CombinedOutput(); err != nil {
				return fmt.Errorf("could not configure %s: %s", service, strings.TrimSpace(string(result)))
			}
		}
		if result, err := exec.Command("networksetup", "-setautoproxystate", service, state).CombinedOutput(); err != nil {
			return fmt.Errorf("could not update %s: %s", service, strings.TrimSpace(string(result)))
		}
	}
	return nil
}

type routerRequest struct {
	Action string   `json:"action"`
	Key    string   `json:"key"`
	Port   int      `json:"port"`
	Routes []string `json:"routes"`
}

func runRootRouter(key, port string, routes []proxyRoute, enabled bool) error {
	portNumber := 0
	if enabled {
		var err error
		portNumber, err = strconv.Atoi(port)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return fmt.Errorf("invalid VPN SOCKS port: %s", port)
		}
	}
	action := "stop"
	if enabled {
		action = "start"
	}
	request := routerRequest{Action: action, Key: key, Port: portNumber}
	for _, route := range routes {
		request.Routes = append(request.Routes, fmt.Sprintf("%s/%d", route.network, route.prefix))
	}
	if err := sendRouterRequest(request); err == nil {
		return nil
	}
	if err := installRouterHelper(); err != nil {
		return err
	}
	for attempts := 0; attempts < 30; attempts++ {
		if err := sendRouterRequest(request); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("privileged routing helper did not start")
}

func sendRouterRequest(request routerRequest) error {
	connection, err := net.DialTimeout("unix", "/var/run/vpntoris/router.sock", time.Second)
	if err != nil {
		return err
	}
	defer connection.Close()
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return err
	}
	var response struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(connection).Decode(&response); err != nil {
		return err
	}
	if response.Error != "" {
		return fmt.Errorf("%s", response.Error)
	}
	return nil
}

func installRouterHelper() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	helper := filepath.Join(filepath.Dir(executable), "vpntoris-route-helper")
	command := strings.Join([]string{shellQuote(helper), "install", strconv.Itoa(os.Getuid())}, " ")
	script := "do shell script " + strconv.Quote(command) + " with administrator privileges"
	if output, err := exec.Command("/usr/bin/osascript", "-e", script).CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("privileged helper installation failed: %s", message)
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func parseRoutes(value string) ([]proxyRoute, error) {
	fields := strings.FieldsFunc(value, func(character rune) bool {
		return character == ',' || character == ';' || character == '\n'
	})
	if len(fields) == 0 {
		return nil, fmt.Errorf("add at least one VPN route to the profile (example: 10.68.0.0/16)")
	}
	routes := make([]proxyRoute, 0, len(fields))
	for _, field := range fields {
		_, network, err := net.ParseCIDR(strings.TrimSpace(field))
		if err != nil || network.IP.To4() == nil {
			return nil, fmt.Errorf("invalid IPv4 VPN route: %s", strings.TrimSpace(field))
		}
		mask := net.IP(network.Mask).String()
		ones, _ := network.Mask.Size()
		routes = append(routes, proxyRoute{network: network.IP.String(), mask: mask, prefix: ones})
	}
	return routes, nil
}

func startPACServer() error {
	listener, err := net.Listen("tcp", pacAddress)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/profiles", handleProfilesAPI)
	mux.HandleFunc("/api/action", handleActionAPI)
	mux.HandleFunc("/api/logs", handleLogsAPI)
	mux.HandleFunc("/api/routes", handleRoutesAPI)
	mux.HandleFunc("/proxy.pac", func(response http.ResponseWriter, _ *http.Request) {
		proxyState.RLock()
		type pacRoute struct {
			proxyRoute
			port string
		}
		entries := []pacRoute{}
		for _, mapping := range proxyState.mappings {
			for _, route := range mapping.routes {
				entries = append(entries, pacRoute{proxyRoute: route, port: mapping.port})
			}
		}
		proxyState.RUnlock()
		response.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		response.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		if len(entries) == 0 {
			_, _ = response.Write([]byte(`function FindProxyForURL(url, host) { return "DIRECT"; }`))
			return
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].prefix > entries[j].prefix })
		lines := make([]string, 0, len(entries))
		for _, entry := range entries {
			lines = append(lines, fmt.Sprintf(`  if (isInNet(host, %q, %q)) return "SOCKS5 127.0.0.1:%s; SOCKS 127.0.0.1:%s";`, entry.network, entry.mask, entry.port, entry.port))
		}
		pac := fmt.Sprintf(`function FindProxyForURL(url, host) {
%s
  return "DIRECT";
}`, strings.Join(lines, "\n"))
		_, _ = response.Write([]byte(pac))
	})
	go func() { _ = http.Serve(listener, mux) }()
	go monitorActiveProxy()
	return nil
}

func monitorActiveProxy() {
	failures := map[string]int{}
	for {
		time.Sleep(2 * time.Second)
		proxyState.RLock()
		mappings := make(map[string]proxyMapping, len(proxyState.mappings))
		for key, mapping := range proxyState.mappings {
			mappings[key] = mapping
		}
		proxyState.RUnlock()
		for key, mapping := range mappings {
			connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", mapping.port), time.Second)
			if err == nil {
				_ = connection.Close()
				failures[key] = 0
				continue
			}
			failures[key]++
			if failures[key] >= 3 {
				_ = setSystemRoutes(key, "", "", false)
				delete(failures, key)
			}
		}
	}
}

func restoreHealthyRoutes() {
	time.Sleep(time.Second)
	configs, err := loadConfigs()
	if err != nil {
		return
	}
	for _, config := range configs {
		key := containerName(config.Name)
		if !containerHealthy(key) {
			continue
		}
		port, err := proxyPort(key)
		if err != nil {
			continue
		}
		_ = setSystemRoutes(key, port, config.Routes, true)
	}
}

type profileView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Host        string `json:"host"`
	Routes      string `json:"routes"`
	Connected   bool   `json:"connected"`
	TwoFactor   bool   `json:"twoFactor"`
}

func handleProfilesAPI(response http.ResponseWriter, _ *http.Request) {
	response.Header().Set("Access-Control-Allow-Origin", "*")
	configs, err := loadConfigs()
	if err != nil {
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}
	profiles := make([]profileView, 0, len(configs))
	for _, config := range configs {
		profiles = append(profiles, profileView{
			Name: config.Name, Description: config.Description, Type: config.Type,
			Host: config.Host, Routes: config.Routes,
			Connected: containerHealthy(containerName(config.Name)),
			TwoFactor: config.TwoFactor,
		})
	}
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(profiles)
}

func handleActionAPI(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Access-Control-Allow-Origin", "*")
	if request.Method != http.MethodPost {
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := request.URL.Query().Get("name")
	action := request.URL.Query().Get("action")
	configs, err := loadConfigs()
	if err != nil {
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}
	var selected *VPNConfig
	for index := range configs {
		if configs[index].Name == name {
			selected = &configs[index]
			break
		}
	}
	if action != "add" && selected == nil {
		http.Error(response, "profile not found", http.StatusNotFound)
		return
	}
	switch action {
	case "connect":
		err = connectVPN(*selected)
	case "otp":
		err = sendOTP(*selected, request.Header.Get("X-VPNToris-OTP"))
	case "disconnect":
		_ = setSystemRoutes(containerName(selected.Name), "", "", false)
		err = disconnectVPN(containerName(selected.Name))
	case "route":
		var port string
		port, err = proxyPort(containerName(selected.Name))
		if err == nil {
			err = setSystemRoutes(containerName(selected.Name), port, selected.Routes, true)
		}

	default:
		http.Error(response, "unknown action", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func handleLogsAPI(response http.ResponseWriter, request *http.Request) {
	name := request.URL.Query().Get("name")
	configs, err := loadConfigs()
	if err != nil {
		http.Error(response, err.Error(), 500)
		return
	}
	var selected *VPNConfig
	for index := range configs {
		if configs[index].Name == name {
			selected = &configs[index]
			break
		}
	}
	if selected == nil {
		http.Error(response, "profile not found", 404)
		return
	}
	path := "/logs/" + selected.Name + ".log"
	output, err := exec.Command("docker", "exec", containerName(selected.Name), "tail", "-n", "300", path).CombinedOutput()
	if err != nil {
		output, err = exec.Command("docker", "logs", "--tail", "300", containerName(selected.Name)).CombinedOutput()
		if err != nil {
			http.Error(response, strings.TrimSpace(string(output)), 500)
			return
		}
	}
	response.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = response.Write(output)
}

func handleRoutesAPI(response http.ResponseWriter, _ *http.Request) {
	type routeView struct {
		Profile string `json:"profile"`
		CIDR    string `json:"cidr"`
		Port    string `json:"port"`
	}
	proxyState.RLock()
	entries := []routeView{}
	for key, mapping := range proxyState.mappings {
		for _, route := range mapping.routes {
			entries = append(entries, routeView{key, fmt.Sprintf("%s/%d", route.network, route.prefix), mapping.port})
		}
	}
	proxyState.RUnlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].CIDR < entries[j].CIDR })
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(entries)
}

func loadConfigs() ([]VPNConfig, error) {
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return []VPNConfig{}, nil
	}
	if err != nil {
		return nil, err
	}
	var configs []VPNConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, err
	}
	return configs, nil
}

func containerName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = safeNamePattern.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-_.")
	if name == "" {
		name = "vpn"
	}
	return "vpntoris-" + name
}

func containerRunning(name string) bool {
	out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", name).Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func containerHealthy(name string) bool {
	out, err := exec.Command("docker", "inspect", "-f", "{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}", name).Output()
	return err == nil && strings.TrimSpace(string(out)) == "healthy"
}

func waitForContainerHealthy(name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if containerHealthy(name) {
			return nil
		}
		if !containerRunning(name) {
			logs, _ := exec.Command("docker", "logs", "--tail", "40", name).CombinedOutput()
			return fmt.Errorf("VPN tunnel failed: %s", strings.TrimSpace(string(logs)))
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("VPN tunnel did not become ready before timeout")
}

func connectVPN(config VPNConfig, otp ...string) error {
	if strings.TrimSpace(config.Name) == "" || strings.TrimSpace(config.Type) == "" {
		return fmt.Errorf("profile name and VPN type are required")
	}
	name := containerName(config.Name)
	_ = exec.Command("docker", "rm", "-f", name).Run()
	profileDir := filepath.Join(filepath.Dir(configPath), "profiles", strings.TrimPrefix(name, "vpntoris-"))
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		return fmt.Errorf("could not create profile directory: %w", err)
	}
	args := []string{
		"run", "-d", "--name", name,
		"--cap-add=NET_ADMIN", "--device=/dev/net/tun",
		"--label", "vpntoris=true", "--label", "vpntoris.profile=" + config.Name,
		"-p", "127.0.0.1::1080",
		"-e", "VPN_TYPE=" + config.Type,
		"-e", "VPN_NAME=" + config.Name,
		"-e", "VPN_HOST=" + config.Host,
		"-e", "VPN_PORT=" + config.Port,
		"-e", "VPN_USER=" + config.User,
		"-e", "VPN_PASS=" + config.Password,
		"-e", fmt.Sprintf("VPN_2FA=%t", config.TwoFactor),
	}
	if config.Type == "openfortivpn" {
		args = append(args, "--cap-add=MKNOD", "--device-cgroup-rule=c 108:0 rwm")
	}
	if config.Type == "ipsec" {
		if config.IPSec == nil {
			return fmt.Errorf("IPsec advanced settings are required")
		}
		swanConfig, err := renderSwanctlConfig(config)
		if err != nil {
			return err
		}
		configFile := filepath.Join(profileDir, "swanctl.conf")
		if err := os.WriteFile(configFile, []byte(swanConfig), 0600); err != nil {
			return fmt.Errorf("could not save IPsec configuration: %w", err)
		}
		args = append(args, "--cap-add=SYS_ADMIN", "--sysctl", "net.ipv4.ip_forward=1",
			"-v", configFile+":/etc/swanctl/swanctl.conf:ro")
	}
	if config.Type == "openvpn" {
		if strings.TrimSpace(config.Config) == "" {
			return fmt.Errorf("OpenVPN configuration is required")
		}
		configFile := filepath.Join(profileDir, "config.conf")
		if err := os.WriteFile(configFile, []byte(config.Config), 0600); err != nil {
			return fmt.Errorf("could not save OpenVPN configuration: %w", err)
		}
		args = append(args, "-v", configFile+":/vpn/config.conf:ro", "-e", "VPN_CONFIG=/vpn/config.conf")
	}
	args = append(args, imageName)
	output, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not start VPN: %s", strings.TrimSpace(string(output)))
	}
	timeout := 45 * time.Second
	if config.TwoFactor {
		timeout = 190 * time.Second
	}
	return waitForContainerHealthy(name, timeout)
}

func sendOTP(config VPNConfig, otp string) error {
	otp = strings.TrimSpace(otp)
	if otp == "" || len(otp) > 32 {
		return fmt.Errorf("invalid OTP code")
	}
	command := exec.Command("docker", "exec", "-i", containerName(config.Name), "/bin/bash", "-c", "cat > /run/vpntoris/otp")
	command.Stdin = strings.NewReader(otp + "\n")
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("could not submit OTP: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

var proposalToken = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

var dhGroupNames = map[string]string{
	"1": "modp768", "2": "modp1024", "5": "modp1536", "14": "modp2048",
	"15": "modp3072", "16": "modp4096", "17": "modp6144", "18": "modp8192",
	"19": "ecp256", "20": "ecp384", "21": "ecp521", "31": "curve25519", "32": "curve448",
}

func tokens(value string) ([]string, error) {
	fields := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' })
	for _, field := range fields {
		if !proposalToken.MatchString(field) {
			return nil, fmt.Errorf("invalid IPsec algorithm token: %s", field)
		}
	}
	return fields, nil
}

func groups(value string) ([]string, error) {
	values, err := tokens(value)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if mapped, ok := dhGroupNames[value]; ok {
			result = append(result, mapped)
		} else if proposalToken.MatchString(value) {
			result = append(result, value)
		}
	}
	return result, nil
}

func buildProposals(encryptionValue, integrityValue, prfValue, groupValue string, ike bool) (string, error) {
	encryptions, err := tokens(encryptionValue)
	if err != nil {
		return "", err
	}
	integrities, err := tokens(integrityValue)
	if err != nil {
		return "", err
	}
	prfs, err := tokens(prfValue)
	if err != nil {
		return "", err
	}
	dhs, err := groups(groupValue)
	if err != nil {
		return "", err
	}
	if len(encryptions) == 0 {
		return "", fmt.Errorf("at least one encryption algorithm is required")
	}
	if len(dhs) == 0 {
		dhs = []string{""}
	}
	if len(prfs) == 0 {
		prfs = []string{""}
	}
	result := []string{}
	for _, enc := range encryptions {
		aead := strings.Contains(enc, "gcm") || strings.Contains(enc, "ccm") || strings.Contains(enc, "chacha20poly1305")
		auths := integrities
		if aead {
			auths = []string{""}
		} else if len(auths) == 0 {
			return "", fmt.Errorf("integrity algorithm is required for %s", enc)
		}
		for _, auth := range auths {
			for _, prf := range prfs {
				for _, dh := range dhs {
					parts := []string{enc}
					if auth != "" {
						parts = append(parts, auth)
					}
					if ike && prf != "" {
						parts = append(parts, prf)
					}
					if dh != "" {
						parts = append(parts, dh)
					}
					result = append(result, strings.Join(parts, "-"))
				}
			}
		}
	}
	return strings.Join(result, ","), nil
}

func renderSwanctlConfig(config VPNConfig) (string, error) {
	ip := config.IPSec
	if ip.IKEVersion != 1 && ip.IKEVersion != 2 {
		return "", fmt.Errorf("IKE version must be 1 or 2")
	}
	if strings.ContainsAny(config.Host, "\r\n{}") {
		return "", fmt.Errorf("invalid IPsec gateway")
	}
	if ip.IKELifetime <= 0 {
		ip.IKELifetime = 28800
	}
	if ip.ChildLifetime <= 0 {
		ip.ChildLifetime = 3600
	}
	if ip.DPDDelay <= 0 {
		ip.DPDDelay = 30
	}
	if ip.DPDTimeout <= 0 {
		ip.DPDTimeout = 150
	}
	if ip.ReplayWindow <= 0 {
		ip.ReplayWindow = 32
	}
	ikePRF := ip.IKEPRF
	if ip.IKEVersion == 1 {
		ikePRF = ""
	}
	ikeProposals, err := buildProposals(ip.IKEEncryption, ip.IKEIntegrity, ikePRF, ip.DHGroups, true)
	if err != nil {
		return "", err
	}
	pfs := ""
	if ip.PFS {
		pfs = ip.PFSGroups
	}
	var espProposals string
	if len(ip.Phase2Proposals) > 0 {
		items := []string{}
		for _, proposal := range ip.Phase2Proposals {
			value, proposalErr := buildProposals(proposal.Encryption, proposal.Authentication, "", pfs, false)
			if proposalErr != nil {
				return "", proposalErr
			}
			items = append(items, value)
		}
		espProposals = strings.Join(items, ",")
	} else {
		espProposals, err = buildProposals(ip.ESPEncryption, ip.ESPIntegrity, "", pfs, false)
		if err != nil {
			return "", err
		}
	}
	remoteTS := ip.RemoteSelectors
	if strings.TrimSpace(remoteTS) == "" {
		if ip.ModeConfig {
			remoteTS = "0.0.0.0/0"
		} else {
			remoteTS = config.Routes
		}
	}
	if _, err := parseRoutes(remoteTS); err != nil {
		return "", fmt.Errorf("invalid IPsec remote selector: %w", err)
	}
	localTS := ip.LocalSelectors
	if ip.ModeConfig && (strings.TrimSpace(localTS) == "" || strings.TrimSpace(localTS) == "0.0.0.0/0") {
		localTS = "dynamic"
	} else if strings.TrimSpace(localTS) == "" {
		localTS = "0.0.0.0/0"
	}
	dpdAction := ip.DPDAction
	if dpdAction == "" {
		dpdAction = "restart"
	}
	fragmentation := ip.Fragmentation
	if fragmentation == "" {
		fragmentation = "yes"
	}
	localID := ip.LocalID
	if localID == "" {
		localID = config.User
	}
	remoteID := ip.RemoteID
	if remoteID == "" {
		remoteID = "%any"
	}
	vips := ""
	if ip.ModeConfig {
		vips = "    vips = 0.0.0.0\n"
	}
	aggressive := ""
	if ip.IKEVersion == 1 && ip.IKEMode == "aggressive" {
		aggressive = "    aggressive = yes\n"
	}
	rekeyBytes := ""
	if ip.ChildLifetimeKB > 0 {
		rekeyBytes = fmt.Sprintf("        rekey_bytes = %dKB\n", ip.ChildLifetimeKB)
	}
	if strings.TrimSpace(ip.PreSharedKey) == "" {
		return "", fmt.Errorf("IPsec pre-shared key is required")
	}
	authSections := fmt.Sprintf("    local {\n      auth = psk\n      id = %q\n    }\n    remote {\n      auth = psk\n      id = %q\n    }\n", localID, remoteID)
	secretSections := fmt.Sprintf("  ike-vpntoris {\n    id = %q\n    secret = %q\n  }\n", localID, ip.PreSharedKey)
	authMode := ip.AuthMode
	if ip.IKEVersion == 1 && authMode == "eap" {
		authMode = "xauth"
	}
	if authMode == "xauth" {
		authSections += fmt.Sprintf("    local-xauth {\n      auth = xauth\n      xauth_id = %q\n    }\n", config.User)
		secretSections += fmt.Sprintf("  xauth-vpntoris {\n    id = %q\n    secret = %q\n  }\n", config.User, config.Password)
	} else if authMode == "eap" {
		authSections += fmt.Sprintf("    local-eap {\n      auth = eap\n      eap_id = %q\n    }\n", config.User)
		secretSections += fmt.Sprintf("  eap-vpntoris {\n    id = %q\n    secret = %q\n  }\n", config.User, config.Password)
	}
	return fmt.Sprintf(`connections {
  vpntoris {
    version = %d
    remote_addrs = %s
    proposals = %s
    rekey_time = %ds
    dpd_delay = %ds
    dpd_timeout = %ds
    fragmentation = %s
    mobike = %t
    encap = %t
%s%s%s    children {
      net {
        local_ts = %s
        remote_ts = %s
        esp_proposals = %s
        life_time = %ds
%s        replay_window = %d
        dpd_action = %s
        start_action = start
      }
    }
  }
}
secrets {
%s}
`, ip.IKEVersion, config.Host, ikeProposals, ip.IKELifetime, ip.DPDDelay, ip.DPDTimeout,
		fragmentation, ip.MOBIKE, ip.ForceEncap, aggressive, vips, authSections,
		localTS, remoteTS, espProposals, ip.ChildLifetime, rekeyBytes, ip.ReplayWindow, dpdAction, secretSections), nil
}

func disconnectVPN(name string) error {
	output, err := exec.Command("docker", "rm", "-f", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not stop VPN: %s", strings.TrimSpace(string(output)))
	}
	return nil
}
