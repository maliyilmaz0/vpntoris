package main

import (
	"archive/zip"
	"bytes"
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

const imageName = "vpntoris-client:next-2"

var safeNamePattern = regexp.MustCompile(`[^a-z0-9_.-]+`)

type VPNConfig struct {
	Name            string       `json:"name"`
	Description     string       `json:"description"`
	Type            string       `json:"type"`
	Host            string       `json:"host"`
	BackupGateways  string       `json:"backupGateways"`
	FailoverLimit   int          `json:"failoverThreshold"`
	Port            string       `json:"port"`
	User            string       `json:"user"`
	Password        string       `json:"password"`
	TwoFactor       bool         `json:"twoFactor"`
	AutoReconnect   bool         `json:"autoReconnect"`
	ConnectOnLaunch bool         `json:"connectOnLaunch"`
	Routes          string       `json:"routes"`
	Domains         string       `json:"domains"`
	DNSServers      string       `json:"dnsServers"`
	Config          string       `json:"config"`
	IPSec           *IPSecConfig `json:"ipsec,omitempty"`
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

var dockerBootstrap = struct {
	sync.RWMutex
	State   string `json:"state"`
	Message string `json:"message"`
}{State: "checking", Message: "Checking Docker…"}

type trafficSnapshot struct {
	Name       string  `json:"name"`
	Received   uint64  `json:"received"`
	Sent       uint64  `json:"sent"`
	ReceiveBPS float64 `json:"receiveBps"`
	SendBPS    float64 `json:"sendBps"`
	Duration   int64   `json:"duration"`
	updatedAt  time.Time
}

type historyEntry struct {
	ID       string `json:"id"`
	Profile  string `json:"profile"`
	Event    string `json:"event"`
	Time     string `json:"time"`
	Received uint64 `json:"received"`
	Sent     uint64 `json:"sent"`
}

type activeFlow struct {
	ID       string `json:"id"`
	Profile  string `json:"profile"`
	Process  string `json:"process"`
	PID      int    `json:"pid"`
	Local    string `json:"local"`
	Remote   string `json:"remote"`
	RemoteIP string `json:"remoteIp"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type analyticsTraffic struct {
	Received uint64 `json:"received"`
	Sent     uint64 `json:"sent"`
}

type analyticsProfile struct {
	Name         string                      `json:"name"`
	Received     uint64                      `json:"received"`
	Sent         uint64                      `json:"sent"`
	Reconnects   int                         `json:"reconnects"`
	Hourly       map[string]analyticsTraffic `json:"hourly"`
	Daily        map[string]analyticsTraffic `json:"daily"`
	Destinations map[string]int              `json:"destinations"`
	Processes    map[string]int              `json:"processes"`
}

var analyticsState = struct {
	sync.Mutex
	loaded   bool
	lastSave time.Time
	profiles map[string]*analyticsProfile
}{profiles: make(map[string]*analyticsProfile)}

type analyticsSettings struct {
	HourlyDays int `json:"hourlyDays"`
	DailyDays  int `json:"dailyDays"`
}

var trafficState = struct {
	sync.RWMutex
	items map[string]trafficSnapshot
}{items: make(map[string]trafficSnapshot)}

var connectionIntent = struct {
	sync.RWMutex
	names    map[string]bool
	busy     map[string]bool
	profiles map[string]VPNConfig
}{names: make(map[string]bool), busy: make(map[string]bool), profiles: make(map[string]VPNConfig)}

type gatewayRecord struct {
	Active   string `json:"active"`
	Failures int    `json:"failures"`
}

var gatewayState = struct {
	sync.Mutex
	loaded bool
	items  map[string]gatewayRecord
}{items: make(map[string]gatewayRecord)}

var otpRequests = struct {
	sync.RWMutex
	names map[string]bool
}{names: make(map[string]bool)}

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
	go bootstrapDockerImage()
	go monitorTraffic()
	go monitorConnections()
	go monitorAnalyticsFlows()
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

func dockerPath() string {
	if path, err := exec.LookPath("docker"); err == nil {
		return path
	}
	for _, path := range []string{"/usr/local/bin/docker", "/opt/homebrew/bin/docker", "/Applications/Docker.app/Contents/Resources/bin/docker"} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func setDockerBootstrap(state, message string) {
	dockerBootstrap.Lock()
	dockerBootstrap.State = state
	dockerBootstrap.Message = message
	dockerBootstrap.Unlock()
}

func bootstrapDockerImage() {
	docker := dockerPath()
	if docker == "" {
		setDockerBootstrap("missing", "Docker Desktop is not installed.")
		return
	}
	if output, err := exec.Command(docker, "info", "--format", "{{.ServerVersion}}").CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = "Docker Desktop is installed but the engine is not running."
		}
		setDockerBootstrap("stopped", message)
		return
	}
	if exec.Command(docker, "image", "inspect", imageName).Run() == nil {
		setDockerBootstrap("ready", "Docker image is ready.")
		return
	}
	executable, err := os.Executable()
	if err != nil {
		setDockerBootstrap("error", err.Error())
		return
	}
	contextPath := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "Resources", "DockerContext"))
	if _, err := os.Stat(filepath.Join(contextPath, "Dockerfile")); err != nil {
		setDockerBootstrap("error", "The embedded Docker build context is missing.")
		return
	}
	setDockerBootstrap("building", "Building the VPN client image for the first time…")
	output, err := exec.Command(docker, "build", "-t", imageName, contextPath).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		setDockerBootstrap("error", message)
		return
	}
	setDockerBootstrap("ready", "Docker image is ready.")
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

func containerPort(container, target string) (int, error) {
	docker := dockerPath()
	if docker == "" {
		return 0, fmt.Errorf("docker is unavailable")
	}
	output, err := exec.Command(docker, "port", container, target).Output()
	if err != nil {
		return 0, err
	}
	address := strings.TrimSpace(strings.Split(string(output), "\n")[0])
	separator := strings.LastIndex(address, ":")
	if separator < 0 {
		return 0, fmt.Errorf("invalid container port")
	}
	return strconv.Atoi(address[separator+1:])
}

func splitValues(value string) []string {
	fields := strings.FieldsFunc(value, func(character rune) bool {
		return character == ',' || character == ';' || character == '\n' || character == ' '
	})
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func resolveDomains(config VPNConfig) []string {
	addresses := map[string]bool{}
	for _, domain := range splitValues(config.Domains) {
		output, err := exec.Command("docker", "exec", containerName(config.Name), "getent", "ahostsv4", domain).Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(output), "\n") {
			fields := strings.Fields(line)
			if len(fields) > 0 && net.ParseIP(fields[0]).To4() != nil {
				addresses[fields[0]] = true
			}
		}
	}
	result := make([]string, 0, len(addresses))
	for address := range addresses {
		result = append(result, address)
	}
	sort.Strings(result)
	return result
}

func activateProfileRoutes(config VPNConfig, port string) error {
	routeList := config.Routes
	for _, server := range splitValues(config.DNSServers) {
		routeList += "," + server + "/32"
	}
	for _, address := range resolveDomains(config) {
		routeList += "," + address + "/32"
	}
	dnsPort := 0
	if config.Domains != "" && config.DNSServers != "" {
		dnsPort, _ = containerPort(containerName(config.Name), "53/udp")
	}
	return setSystemRoutesWithDNS(containerName(config.Name), port, routeList, config.Domains, dnsPort, true)
}

func setSystemRoutes(key, port, routeList string, enabled bool) error {
	return setSystemRoutesWithDNS(key, port, routeList, "", 0, enabled)
}

func setSystemRoutesWithDNS(key, port, routeList, domains string, dnsPort int, enabled bool) error {
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
	if err := runRootRouter(key, port, routerRoutes, splitValues(domains), dnsPort, enabled); err != nil {
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
	Action  string   `json:"action"`
	Key     string   `json:"key"`
	Port    int      `json:"port"`
	Routes  []string `json:"routes"`
	Domains []string `json:"domains"`
	DNSPort int      `json:"dnsPort"`
}

func runRootRouter(key, port string, routes []proxyRoute, domains []string, dnsPort int, enabled bool) error {
	portNumber := 0
	if enabled {
		var err error
		portNumber, err = strconv.Atoi(port)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return fmt.Errorf("invalid VPN SOCKS port: %s", port)
		}
	}
	action := "stop-v2"
	if enabled {
		action = "start-v2"
	}
	request := routerRequest{Action: action, Key: key, Port: portNumber, Domains: domains, DNSPort: dnsPort}
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
	mux.HandleFunc("/api/docker", handleDockerAPI)
	mux.HandleFunc("/api/traffic", handleTrafficAPI)
	mux.HandleFunc("/api/history", handleHistoryAPI)
	mux.HandleFunc("/api/route-check", handleRouteCheckAPI)
	mux.HandleFunc("/api/recover", handleRecoverAPI)
	mux.HandleFunc("/api/flows", handleFlowsAPI)
	mux.HandleFunc("/api/diagnostics", handleDiagnosticsAPI)
	mux.HandleFunc("/api/analytics", handleAnalyticsAPI)
	mux.HandleFunc("/api/analytics-settings", handleAnalyticsSettingsAPI)
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

func handleDiagnosticsAPI(response http.ResponseWriter, _ *http.Request) {
	buffer := &bytes.Buffer{}
	archive := zip.NewWriter(buffer)
	configs, _ := loadConfigs()
	for index := range configs {
		configs[index].Password = ""
		configs[index].Config = ""
		if configs[index].IPSec != nil {
			configs[index].IPSec.PreSharedKey = ""
		}
	}
	dockerBootstrap.RLock()
	dockerStatus := struct {
		State   string `json:"state"`
		Message string `json:"message"`
	}{dockerBootstrap.State, sanitizeDiagnosticText(dockerBootstrap.Message)}
	dockerBootstrap.RUnlock()
	trafficState.RLock()
	traffic := make([]trafficSnapshot, 0, len(trafficState.items))
	for _, item := range trafficState.items {
		traffic = append(traffic, item)
	}
	trafficState.RUnlock()
	summary := struct {
		Created  string            `json:"created"`
		Version  string            `json:"version"`
		OS       string            `json:"os"`
		Docker   any               `json:"docker"`
		Profiles []VPNConfig       `json:"profiles"`
		Traffic  []trafficSnapshot `json:"traffic"`
	}{time.Now().Format(time.RFC3339), "next", "macOS", dockerStatus, configs, traffic}
	data, _ := json.MarshalIndent(summary, "", "  ")
	writeDiagnosticFile(archive, "summary.json", data)
	writeDiagnosticCommand(archive, "system.txt", "/usr/bin/uname", "-a")
	writeDiagnosticCommand(archive, "routes.txt", "/usr/sbin/netstat", "-rn", "-f", "inet")
	writeDiagnosticCommand(archive, "dns.txt", "/usr/sbin/scutil", "--dns")
	if docker := dockerPath(); docker != "" {
		writeDiagnosticCommand(archive, "docker-containers.txt", docker, "ps", "-a", "--filter", "label=vpntoris=true", "--format", "{{.Names}}\t{{.Status}}\t{{.Image}}")
		for _, config := range configs {
			output, _ := exec.Command(docker, "logs", "--tail", "500", containerName(config.Name)).CombinedOutput()
			writeDiagnosticFile(archive, "logs/"+safeFileName(config.Name)+".log", []byte(sanitizeDiagnosticText(string(output))))
		}
	}
	_ = archive.Close()
	response.Header().Set("Content-Type", "application/zip")
	response.Header().Set("Content-Disposition", `attachment; filename="VPNToris-Diagnostics.zip"`)
	response.Header().Set("Cache-Control", "no-store")
	_, _ = response.Write(buffer.Bytes())
}

func writeDiagnosticCommand(archive *zip.Writer, name, command string, arguments ...string) {
	output, err := exec.Command(command, arguments...).CombinedOutput()
	if err != nil {
		output = append(output, []byte("\n"+err.Error())...)
	}
	writeDiagnosticFile(archive, name, []byte(sanitizeDiagnosticText(string(output))))
}

func writeDiagnosticFile(archive *zip.Writer, name string, data []byte) {
	file, err := archive.Create(name)
	if err == nil {
		_, _ = file.Write(data)
	}
}

func safeFileName(value string) string {
	name := safeNamePattern.ReplaceAllString(strings.ToLower(value), "-")
	if name == "" {
		return "profile"
	}
	return name
}

func sanitizeDiagnosticText(value string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(password|passwd|secret|psk|token|otp|authorization)(\s*[=:]\s*|\s+)[^\s,;]+`),
		regexp.MustCompile(`(?i)(--password|--passwd|--secret|--psk|--token|--otp)\s+[^\s]+`),
	}
	for _, pattern := range patterns {
		value = pattern.ReplaceAllString(value, "$1$2[REDACTED]")
	}
	return value
}

func handleFlowsAPI(response http.ResponseWriter, _ *http.Request) {
	flows, err := currentFlows()
	if err != nil {
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(flows)
}

func currentFlows() ([]activeFlow, error) {
	output, err := exec.Command("/usr/sbin/lsof", "-nP", "-iTCP", "-sTCP:ESTABLISHED", "-FpcnT").Output()
	if err != nil {
		return nil, err
	}
	return parseFlows(string(output)), nil
}

func analyticsPath() string { return filepath.Join(filepath.Dir(configPath), "analytics.json") }
func analyticsSettingsPath() string {
	return filepath.Join(filepath.Dir(configPath), "analytics-settings.json")
}

func loadAnalyticsSettings() analyticsSettings {
	settings := analyticsSettings{HourlyDays: 7, DailyDays: 90}
	if data, err := os.ReadFile(analyticsSettingsPath()); err == nil {
		_ = json.Unmarshal(data, &settings)
	}
	if settings.HourlyDays < 1 || settings.HourlyDays > 30 {
		settings.HourlyDays = 7
	}
	if settings.DailyDays < 7 || settings.DailyDays > 365 {
		settings.DailyDays = 90
	}
	return settings
}

func loadAnalyticsLocked() {
	if analyticsState.loaded {
		return
	}
	analyticsState.loaded = true
	data, err := os.ReadFile(analyticsPath())
	if err == nil {
		_ = json.Unmarshal(data, &analyticsState.profiles)
	}
	if analyticsState.profiles == nil {
		analyticsState.profiles = make(map[string]*analyticsProfile)
	}
}

func analyticsProfileLocked(name string) *analyticsProfile {
	loadAnalyticsLocked()
	profile := analyticsState.profiles[name]
	if profile == nil {
		profile = &analyticsProfile{Name: name, Hourly: make(map[string]analyticsTraffic), Daily: make(map[string]analyticsTraffic), Destinations: make(map[string]int), Processes: make(map[string]int)}
		analyticsState.profiles[name] = profile
	}
	if profile.Hourly == nil {
		profile.Hourly = make(map[string]analyticsTraffic)
	}
	if profile.Daily == nil {
		profile.Daily = make(map[string]analyticsTraffic)
	}
	if profile.Destinations == nil {
		profile.Destinations = make(map[string]int)
	}
	if profile.Processes == nil {
		profile.Processes = make(map[string]int)
	}
	return profile
}

func saveAnalyticsLocked(force bool) {
	if !force && time.Since(analyticsState.lastSave) < 30*time.Second {
		return
	}
	data, err := json.MarshalIndent(analyticsState.profiles, "", "  ")
	if err == nil {
		_ = os.WriteFile(analyticsPath(), data, 0600)
		analyticsState.lastSave = time.Now()
	}
}

func recordTrafficAnalytics(name string, received, sent uint64) {
	if received == 0 && sent == 0 {
		return
	}
	now := time.Now()
	settings := loadAnalyticsSettings()
	analyticsState.Lock()
	defer analyticsState.Unlock()
	profile := analyticsProfileLocked(name)
	profile.Received += received
	profile.Sent += sent
	hour := now.Format("2006-01-02T15:00:00-07:00")
	day := now.Format("2006-01-02")
	hourly := profile.Hourly[hour]
	hourly.Received += received
	hourly.Sent += sent
	profile.Hourly[hour] = hourly
	daily := profile.Daily[day]
	daily.Received += received
	daily.Sent += sent
	profile.Daily[day] = daily
	for key := range profile.Hourly {
		parsed, err := time.Parse("2006-01-02T15:00:00-07:00", key)
		if err != nil || now.Sub(parsed) > time.Duration(settings.HourlyDays)*24*time.Hour {
			delete(profile.Hourly, key)
		}
	}
	for key := range profile.Daily {
		parsed, err := time.Parse("2006-01-02", key)
		if err != nil || now.Sub(parsed) > time.Duration(settings.DailyDays)*24*time.Hour {
			delete(profile.Daily, key)
		}
	}
	saveAnalyticsLocked(false)
}

func recordReconnectAnalytics(name string) {
	analyticsState.Lock()
	defer analyticsState.Unlock()
	analyticsProfileLocked(name).Reconnects++
	saveAnalyticsLocked(true)
}

func monitorAnalyticsFlows() {
	for {
		time.Sleep(30 * time.Second)
		flows, err := currentFlows()
		if err != nil {
			continue
		}
		analyticsState.Lock()
		for _, flow := range flows {
			profile := analyticsProfileLocked(flow.Profile)
			profile.Destinations[flow.RemoteIP]++
			profile.Processes[flow.Process]++
		}
		saveAnalyticsLocked(false)
		analyticsState.Unlock()
	}
}

func handleAnalyticsAPI(response http.ResponseWriter, request *http.Request) {
	analyticsState.Lock()
	defer analyticsState.Unlock()
	loadAnalyticsLocked()
	if request.Method == http.MethodDelete {
		analyticsState.profiles = make(map[string]*analyticsProfile)
		saveAnalyticsLocked(true)
		response.WriteHeader(http.StatusNoContent)
		return
	}
	profiles := make([]analyticsProfile, 0, len(analyticsState.profiles))
	for _, profile := range analyticsState.profiles {
		profiles = append(profiles, *profile)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(profiles)
}

func handleAnalyticsSettingsAPI(response http.ResponseWriter, request *http.Request) {
	settings := loadAnalyticsSettings()
	if request.Method == http.MethodPost {
		hourly, _ := strconv.Atoi(request.URL.Query().Get("hourlyDays"))
		daily, _ := strconv.Atoi(request.URL.Query().Get("dailyDays"))
		if hourly < 1 || hourly > 30 || daily < 7 || daily > 365 {
			http.Error(response, "retention values are out of range", http.StatusBadRequest)
			return
		}
		settings = analyticsSettings{HourlyDays: hourly, DailyDays: daily}
		if data, err := json.MarshalIndent(settings, "", "  "); err == nil {
			_ = os.WriteFile(analyticsSettingsPath(), data, 0600)
		}
	}
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(settings)
}

func parseFlows(output string) []activeFlow {
	configs, _ := loadConfigs()
	type configuredRoute struct {
		profile string
		block   *net.IPNet
		prefix  int
	}
	routes := []configuredRoute{}
	for _, config := range configs {
		for _, route := range splitValues(config.Routes) {
			_, block, err := net.ParseCIDR(route)
			if err == nil {
				prefix, _ := block.Mask.Size()
				routes = append(routes, configuredRoute{config.Name, block, prefix})
			}
		}
	}
	pid := 0
	process := ""
	flows := []activeFlow{}
	for _, line := range strings.Split(output, "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ = strconv.Atoi(line[1:])
		case 'c':
			process = line[1:]
		case 'n':
			connection := line[1:]
			parts := strings.Split(connection, "->")
			if len(parts) != 2 {
				continue
			}
			remoteHost, remotePort, err := net.SplitHostPort(parts[1])
			if err != nil {
				continue
			}
			remoteIP := net.ParseIP(strings.Trim(remoteHost, "[]"))
			if remoteIP == nil {
				continue
			}
			bestProfile, bestPrefix := "", -1
			for _, route := range routes {
				if route.block.Contains(remoteIP) && route.prefix > bestPrefix {
					bestProfile, bestPrefix = route.profile, route.prefix
				}
			}
			if bestProfile == "" {
				continue
			}
			port, _ := strconv.Atoi(remotePort)
			flows = append(flows, activeFlow{ID: fmt.Sprintf("%d-%s", pid, parts[1]), Profile: bestProfile, Process: process, PID: pid, Local: parts[0], Remote: parts[1], RemoteIP: remoteIP.String(), Port: port, Protocol: "TCP"})
		}
	}
	sort.Slice(flows, func(i, j int) bool {
		if flows[i].Profile == flows[j].Profile {
			return flows[i].Process < flows[j].Process
		}
		return flows[i].Profile < flows[j].Profile
	})
	return flows
}

func handleRecoverAPI(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	go bootstrapDockerImage()
	go restoreHealthyRoutes()
	response.WriteHeader(http.StatusNoContent)
}

func handleTrafficAPI(response http.ResponseWriter, _ *http.Request) {
	trafficState.RLock()
	items := make([]trafficSnapshot, 0, len(trafficState.items))
	for _, item := range trafficState.items {
		items = append(items, item)
	}
	trafficState.RUnlock()
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(items)
}

func monitorTraffic() {
	for {
		configs, _ := loadConfigs()
		seen := make(map[string]bool)
		for _, config := range configs {
			container := containerName(config.Name)
			if !containerRunning(container) {
				continue
			}
			received, sent, err := containerTraffic(container)
			if err != nil {
				continue
			}
			seen[config.Name] = true
			now := time.Now()
			trafficState.Lock()
			previous := trafficState.items[config.Name]
			item := trafficSnapshot{Name: config.Name, Received: received, Sent: sent, updatedAt: now, Duration: containerDuration(container)}
			if !previous.updatedAt.IsZero() {
				seconds := now.Sub(previous.updatedAt).Seconds()
				if seconds > 0 && received >= previous.Received && sent >= previous.Sent {
					receivedDelta := received - previous.Received
					sentDelta := sent - previous.Sent
					item.ReceiveBPS = float64(receivedDelta) / seconds
					item.SendBPS = float64(sentDelta) / seconds
					recordTrafficAnalytics(config.Name, receivedDelta, sentDelta)
				}
			}
			trafficState.items[config.Name] = item
			trafficState.Unlock()
		}
		trafficState.Lock()
		for name := range trafficState.items {
			if !seen[name] {
				delete(trafficState.items, name)
			}
		}
		trafficState.Unlock()
		time.Sleep(time.Second)
	}
}

func containerTraffic(container string) (uint64, uint64, error) {
	docker := dockerPath()
	if docker == "" {
		return 0, 0, fmt.Errorf("docker is unavailable")
	}
	output, err := exec.Command(docker, "stats", "--no-stream", "--format", "{{.NetIO}}", container).Output()
	if err != nil {
		return 0, 0, err
	}
	parts := strings.Split(strings.TrimSpace(string(output)), "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid docker traffic output")
	}
	received, err := parseByteSize(parts[0])
	if err != nil {
		return 0, 0, err
	}
	sent, err := parseByteSize(parts[1])
	return received, sent, err
}

func parseByteSize(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	index := 0
	for index < len(value) && ((value[index] >= '0' && value[index] <= '9') || value[index] == '.') {
		index++
	}
	if index == 0 {
		return 0, fmt.Errorf("invalid byte size")
	}
	number, err := strconv.ParseFloat(value[:index], 64)
	if err != nil {
		return 0, err
	}
	unit := strings.ToUpper(strings.TrimSpace(value[index:]))
	multiplier := float64(1)
	switch unit {
	case "KB":
		multiplier = 1e3
	case "MB":
		multiplier = 1e6
	case "GB":
		multiplier = 1e9
	case "TB":
		multiplier = 1e12
	case "KIB":
		multiplier = 1024
	case "MIB":
		multiplier = 1024 * 1024
	case "GIB":
		multiplier = 1024 * 1024 * 1024
	case "B", "":
	default:
		return 0, fmt.Errorf("unknown byte unit: %s", unit)
	}
	return uint64(number * multiplier), nil
}

func containerDuration(container string) int64 {
	docker := dockerPath()
	if docker == "" {
		return 0
	}
	output, err := exec.Command(docker, "inspect", "--format", "{{.State.StartedAt}}", container).Output()
	if err != nil {
		return 0
	}
	started, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(output)))
	if err != nil {
		return 0
	}
	return int64(time.Since(started).Seconds())
}

func handleDockerAPI(response http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodPost {
		dockerBootstrap.RLock()
		state := dockerBootstrap.State
		dockerBootstrap.RUnlock()
		if state != "building" {
			go bootstrapDockerImage()
		}
	}
	dockerBootstrap.RLock()
	status := struct {
		State   string `json:"state"`
		Message string `json:"message"`
	}{dockerBootstrap.State, dockerBootstrap.Message}
	dockerBootstrap.RUnlock()
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(status)
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
		_ = activateProfileRoutes(config, port)
	}
}

type profileView struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Type          string `json:"type"`
	Host          string `json:"host"`
	ActiveHost    string `json:"activeGateway"`
	GatewayCount  int    `json:"gatewayCount"`
	Routes        string `json:"routes"`
	Connected     bool   `json:"connected"`
	TwoFactor     bool   `json:"twoFactor"`
	AutoReconnect bool   `json:"autoReconnect"`
	NeedsOTP      bool   `json:"needsOtp"`
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
		otpRequests.RLock()
		needsOTP := otpRequests.names[config.Name]
		otpRequests.RUnlock()
		gateways := gatewayCandidates(config)
		profiles = append(profiles, profileView{
			Name: config.Name, Description: config.Description, Type: config.Type,
			Host: config.Host, ActiveHost: activeGateway(config), GatewayCount: len(gateways), Routes: config.Routes,
			Connected:     containerHealthy(containerName(config.Name)),
			TwoFactor:     config.TwoFactor,
			AutoReconnect: config.AutoReconnect,
			NeedsOTP:      needsOTP,
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
		selected.Password = request.Header.Get("X-VPNToris-Password")
		if selected.IPSec != nil {
			selected.IPSec.PreSharedKey = request.Header.Get("X-VPNToris-PSK")
		}
		err = connectVPNWithFailover(*selected, true)
		if err == nil && selected.AutoReconnect {
			connectionIntent.Lock()
			connectionIntent.names[selected.Name] = true
			connectionIntent.profiles[selected.Name] = *selected
			connectionIntent.Unlock()
		}
		if err == nil {
			recordHistory(selected.Name, "connected")
		}
	case "arm":
		selected.Password = request.Header.Get("X-VPNToris-Password")
		if selected.IPSec != nil {
			selected.IPSec.PreSharedKey = request.Header.Get("X-VPNToris-PSK")
		}
		connectionIntent.Lock()
		connectionIntent.names[selected.Name] = selected.AutoReconnect
		connectionIntent.profiles[selected.Name] = *selected
		connectionIntent.Unlock()
	case "otp":
		err = sendOTP(*selected, request.Header.Get("X-VPNToris-OTP"))
		if err == nil {
			otpRequests.Lock()
			delete(otpRequests.names, selected.Name)
			otpRequests.Unlock()
		}
	case "disconnect":
		connectionIntent.Lock()
		delete(connectionIntent.names, selected.Name)
		delete(connectionIntent.profiles, selected.Name)
		connectionIntent.Unlock()
		otpRequests.Lock()
		delete(otpRequests.names, selected.Name)
		otpRequests.Unlock()
		_ = setSystemRoutes(containerName(selected.Name), "", "", false)
		err = disconnectVPN(containerName(selected.Name))
		if err == nil {
			recordHistory(selected.Name, "disconnected")
		}
	case "route":
		var port string
		port, err = proxyPort(containerName(selected.Name))
		if err == nil {
			err = activateProfileRoutes(*selected, port)
		}
	case "delete":
		_ = setSystemRoutes(containerName(selected.Name), "", "", false)
		_ = disconnectVPN(containerName(selected.Name))
		deleteGatewayState(selected.Name)
		err = deleteConfig(selected.Name)

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

func historyPath() string { return filepath.Join(filepath.Dir(configPath), "history.json") }

func loadHistory() []historyEntry {
	data, err := os.ReadFile(historyPath())
	if err != nil {
		return []historyEntry{}
	}
	var entries []historyEntry
	if json.Unmarshal(data, &entries) != nil {
		return []historyEntry{}
	}
	return entries
}

func recordHistory(profile, event string) {
	entry := historyEntry{ID: fmt.Sprintf("%d-%s", time.Now().UnixNano(), profile), Profile: profile, Event: event, Time: time.Now().Format(time.RFC3339)}
	trafficState.RLock()
	if item, ok := trafficState.items[profile]; ok {
		entry.Received, entry.Sent = item.Received, item.Sent
	}
	trafficState.RUnlock()
	entries := append(loadHistory(), entry)
	if len(entries) > 1000 {
		entries = entries[len(entries)-1000:]
	}
	if data, err := json.MarshalIndent(entries, "", "  "); err == nil {
		_ = os.WriteFile(historyPath(), data, 0600)
	}
}

func handleHistoryAPI(response http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodDelete {
		_ = os.WriteFile(historyPath(), []byte("[]\n"), 0600)
		response.WriteHeader(http.StatusNoContent)
		return
	}
	entries := loadHistory()
	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(entries)
}

func handleRouteCheckAPI(response http.ResponseWriter, request *http.Request) {
	target := net.ParseIP(strings.TrimSpace(request.URL.Query().Get("target"))).To4()
	if target == nil {
		http.Error(response, "enter a valid IPv4 address", http.StatusBadRequest)
		return
	}
	configs, err := loadConfigs()
	if err != nil {
		http.Error(response, err.Error(), 500)
		return
	}
	type match struct {
		Profile   string `json:"profile"`
		CIDR      string `json:"cidr"`
		Prefix    int    `json:"prefix"`
		Connected bool   `json:"connected"`
	}
	matches := []match{}
	for _, config := range configs {
		routes, _ := parseRoutes(config.Routes)
		for _, route := range routes {
			_, block, _ := net.ParseCIDR(fmt.Sprintf("%s/%d", route.network, route.prefix))
			if block.Contains(target) {
				matches = append(matches, match{config.Name, fmt.Sprintf("%s/%d", route.network, route.prefix), route.prefix, containerHealthy(containerName(config.Name))})
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Prefix > matches[j].Prefix })
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(map[string]any{"target": target.String(), "matches": matches, "conflict": len(matches) > 1 && matches[0].Prefix == matches[1].Prefix})
}

func monitorConnections() {
	time.Sleep(3 * time.Second)
	for {
		configs, err := loadConfigs()
		if err == nil {
			for _, config := range configs {
				container := containerName(config.Name)
				connectionIntent.RLock()
				wanted := connectionIntent.names[config.Name]
				busy := connectionIntent.busy[config.Name]
				profile, cached := connectionIntent.profiles[config.Name]
				connectionIntent.RUnlock()
				if !wanted || !cached || busy || containerHealthy(container) {
					continue
				}
				connectionIntent.Lock()
				connectionIntent.busy[config.Name] = true
				connectionIntent.Unlock()
				go func(profile VPNConfig) {
					recordReconnectAnalytics(profile.Name)
					if profile.TwoFactor {
						otpRequests.Lock()
						otpRequests.names[profile.Name] = true
						otpRequests.Unlock()
					}
					err := connectVPNWithFailover(profile, false)
					if err != nil {
						otpRequests.Lock()
						delete(otpRequests.names, profile.Name)
						otpRequests.Unlock()
					}
					connectionIntent.Lock()
					delete(connectionIntent.busy, profile.Name)
					connectionIntent.Unlock()
				}(profile)
			}
		}
		time.Sleep(5 * time.Second)
	}
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

func deleteConfig(name string) error {
	configs, err := loadConfigs()
	if err != nil {
		return err
	}
	filtered := make([]VPNConfig, 0, len(configs))
	for _, config := range configs {
		if config.Name != name {
			filtered = append(filtered, config)
		}
	}
	data, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0600)
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

func gatewayStatePath() string {
	return filepath.Join(filepath.Dir(configPath), "gateway-state.json")
}

func loadGatewayStateLocked() {
	if gatewayState.loaded {
		return
	}
	gatewayState.loaded = true
	data, err := os.ReadFile(gatewayStatePath())
	if err == nil {
		_ = json.Unmarshal(data, &gatewayState.items)
	}
}

func saveGatewayStateLocked() {
	if data, err := json.MarshalIndent(gatewayState.items, "", "  "); err == nil {
		_ = os.WriteFile(gatewayStatePath(), data, 0600)
	}
}

func deleteGatewayState(name string) {
	gatewayState.Lock()
	defer gatewayState.Unlock()
	loadGatewayStateLocked()
	delete(gatewayState.items, name)
	saveGatewayStateLocked()
}

func gatewayCandidates(config VPNConfig) []string {
	values := append([]string{strings.TrimSpace(config.Host)}, splitValues(config.BackupGateways)...)
	seen := map[string]bool{}
	gateways := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			gateways = append(gateways, value)
		}
	}
	return gateways
}

func validGateway(value string) bool {
	return regexp.MustCompile(`^[A-Za-z0-9._-]+$`).MatchString(value)
}

func activeGateway(config VPNConfig) string {
	gateways := gatewayCandidates(config)
	if len(gateways) == 0 {
		return ""
	}
	gatewayState.Lock()
	defer gatewayState.Unlock()
	loadGatewayStateLocked()
	record := gatewayState.items[config.Name]
	for _, gateway := range gateways {
		if gateway == record.Active {
			return gateway
		}
	}
	return gateways[0]
}

func setGatewayResult(config VPNConfig, gateway string, succeeded bool, rotate bool) string {
	gateways := gatewayCandidates(config)
	if len(gateways) == 0 {
		return ""
	}
	gatewayState.Lock()
	defer gatewayState.Unlock()
	loadGatewayStateLocked()
	record := gatewayState.items[config.Name]
	record.Active = gateway
	if succeeded {
		record.Failures = 0
	} else {
		record.Failures++
	}
	limit := config.FailoverLimit
	if limit < 1 {
		limit = 2
	}
	if rotate || record.Failures >= limit {
		for index, candidate := range gateways {
			if candidate == gateway {
				record.Active = gateways[(index+1)%len(gateways)]
				break
			}
		}
		record.Failures = 0
	}
	gatewayState.items[config.Name] = record
	saveGatewayStateLocked()
	return record.Active
}

func orderedGateways(config VPNConfig) []string {
	gateways := gatewayCandidates(config)
	active := activeGateway(config)
	for index, gateway := range gateways {
		if gateway == active {
			return append(append([]string{}, gateways[index:]...), gateways[:index]...)
		}
	}
	return gateways
}

func connectVPNWithFailover(config VPNConfig, exhaustive bool) error {
	gateways := orderedGateways(config)
	if len(gateways) == 0 {
		return fmt.Errorf("at least one VPN gateway is required")
	}
	if !exhaustive {
		gateways = gateways[:1]
	}
	errors := make([]string, 0, len(gateways))
	for _, gateway := range gateways {
		if !validGateway(gateway) {
			return fmt.Errorf("invalid VPN gateway: %s", gateway)
		}
		attempt := config
		attempt.Host = gateway
		err := connectVPN(attempt)
		if err == nil {
			port, portErr := proxyPort(containerName(config.Name))
			if portErr == nil {
				portErr = activateProfileRoutes(attempt, port)
			}
			if portErr == nil {
				setGatewayResult(config, gateway, true, false)
				return nil
			}
			err = portErr
		}
		errors = append(errors, gateway+": "+err.Error())
		setGatewayResult(config, gateway, false, exhaustive)
	}
	return fmt.Errorf("all VPN gateway attempts failed: %s", strings.Join(errors, " | "))
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
		"-p", "127.0.0.1::53/udp",
		"-e", "VPN_TYPE=" + config.Type,
		"-e", "VPN_NAME=" + config.Name,
		"-e", "VPN_HOST=" + config.Host,
		"-e", "VPN_PORT=" + config.Port,
		"-e", "VPN_USER=" + config.User,
		"-e", "VPN_PASS=" + config.Password,
		"-e", fmt.Sprintf("VPN_2FA=%t", config.TwoFactor),
		"-e", "VPN_DNS_SERVERS=" + config.DNSServers,
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
		openVPNConfig := overrideOpenVPNRemote(config.Config, config.Host, config.Port)
		if err := os.WriteFile(configFile, []byte(openVPNConfig), 0600); err != nil {
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

func overrideOpenVPNRemote(configuration, gateway, port string) string {
	lines := strings.Split(configuration, "\n")
	result := make([]string, 0, len(lines)+1)
	replaced := false
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.EqualFold(fields[0], "remote") {
			if replaced {
				continue
			}
			fields[1] = gateway
			result = append(result, strings.Join(fields, " "))
			replaced = true
			continue
		}
		result = append(result, line)
	}
	if !replaced {
		remote := "remote " + gateway
		if strings.TrimSpace(port) != "" {
			remote += " " + strings.TrimSpace(port)
		}
		result = append([]string{remote}, result...)
	}
	return strings.Join(result, "\n")
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
