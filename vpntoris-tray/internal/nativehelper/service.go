package nativehelper

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"vpntoris-tray/internal/fortihelper"
	"vpntoris-tray/internal/nativeengine"
	"vpntoris-tray/internal/netbackend"
	"vpntoris-tray/internal/openvpnconfig"
	"vpntoris-tray/internal/runtimepaths"
)

const negotiationTimeout = 300 * time.Second
const journalOwner = "com.vpntoris.native-engine"

type Config struct {
	Paths      runtimepaths.Paths
	EngineRoot string
	UserID     int
	Router     netbackend.Router
	Manager    *nativeengine.Manager
}
type Service struct {
	mu           sync.Mutex
	paths        runtimepaths.Paths
	engineRoot   string
	userID       int
	router       netbackend.Router
	manager      *nativeengine.Manager
	sessions     map[string]*session
	ipsecCommand *exec.Cmd
	ipsecLog     *os.File
	ipsecSocket  string
	ipsecConfig  string
	ipsecSwanctl string
}
type session struct {
	request        fortihelper.Request
	command        *exec.Cmd
	input          *os.File
	log            *os.File
	logPath        string
	interfaceName  string
	state          string
	errorText      string
	configPath     string
	otpPath        string
	managementPath string
	management     net.Conn
	username       string
	password       string
	challenge      string
	challengeState string
	received       uint64
	sent           uint64
	certAccepts    int
	startedAt      time.Time
	transaction    *nativeengine.Transaction
}

func New(config Config) (*Service, error) {
	if config.EngineRoot == "" {
		return nil, fmt.Errorf("engine root is required")
	}
	if config.Router == nil {
		config.Router = netbackend.New()
	}
	defaults := runtimepaths.Current()
	if config.Paths.RuntimeDirectory == "" {
		config.Paths.RuntimeDirectory = defaults.RuntimeDirectory
	}
	if config.Paths.LogDirectory == "" {
		config.Paths.LogDirectory = defaults.LogDirectory
	}
	if config.Paths.Platform == "" {
		config.Paths.Platform = defaults.Platform
	}
	root, err := filepath.Abs(config.EngineRoot)
	if err != nil {
		return nil, err
	}
	manager := config.Manager
	if manager == nil && config.Paths.StateDirectory != "" {
		journal, journalErr := nativeengine.NewJournal(config.Paths.StateDirectory, journalOwner)
		if journalErr != nil {
			return nil, journalErr
		}
		backend := netbackend.MutationBackend{Router: config.Router, DNS: netbackend.NewDNS()}
		manager, journalErr = nativeengine.NewManager(journal, backend)
		if journalErr != nil {
			return nil, journalErr
		}
	}
	return &Service{
		paths:      config.Paths,
		engineRoot: root,
		userID:     config.UserID,
		router:     config.Router,
		manager:    manager,
		sessions:   make(map[string]*session),
	}, nil
}
func (service *Service) PrepareRuntime() error {
	if err := os.MkdirAll(service.paths.LogDirectory, 0751); err != nil {
		return err
	}
	if err := os.MkdirAll(service.paths.RuntimeDirectory, 0755); err != nil {
		return err
	}
	if err := os.Chmod(service.paths.RuntimeDirectory, 0755); err != nil {
		return err
	}
	if service.paths.StateDirectory != "" {
		if err := os.MkdirAll(service.paths.StateDirectory, 0700); err != nil {
			return err
		}
	}
	if err := os.Chmod(service.paths.LogDirectory, 0751); err != nil {
		return err
	}
	return service.Recover(context.Background())
}
func (service *Service) Recover(ctx context.Context) error {
	if service.manager == nil {
		return nil
	}
	return service.manager.Recover(ctx)
}
func (service *Service) Handle(request fortihelper.Request) fortihelper.Response {
	if err := request.Validate(); err != nil {
		return fortihelper.Response{State: "failed", Error: err.Error()}
	}
	switch request.Action {
	case fortihelper.ActionStart:
		return service.Start(request)
	case fortihelper.ActionOTP:
		return service.SendOTP(request)
	case fortihelper.ActionStop:
		return service.Stop(request.Profile)
	case fortihelper.ActionStatus:
		return service.Status(request.Profile)
	case fortihelper.ActionReset:
		return service.Reset()
	default:
		return fortihelper.Response{State: "failed", Error: "invalid action"}
	}
}
func (service *Service) Start(request fortihelper.Request) fortihelper.Response {
	service.mu.Lock()
	defer service.mu.Unlock()
	if current := service.sessions[request.Profile]; current != nil && current.state != "stopped" && current.state != "failed" {
		return fortihelper.Response{State: current.state, Interface: current.interfaceName, Error: "profile is already running"}
	}
	protocol := request.Protocol
	if protocol == "" {
		protocol = fortihelper.ProtocolFortiGateSSL
	}
	if protocol == fortihelper.ProtocolIPSec {
		return service.startIPSecLocked(request)
	}
	engineName := "openfortivpn"
	if protocol == fortihelper.ProtocolOpenVPN {
		engineName = "openvpn"
	} else if protocol == fortihelper.ProtocolOpenConnect {
		engineName = "openconnect"
	}
	manifestPath := filepath.Join(service.engineRoot, engineName, "manifest.json")
	_, executable, err := nativeengine.LoadEngineManifest(service.engineRoot, manifestPath)
	if err != nil {
		return fortihelper.Response{State: "failed", Error: err.Error()}
	}
	readInput, writeInput, err := os.Pipe()
	if err != nil {
		return fortihelper.Response{State: "failed", Error: err.Error()}
	}
	logPath := service.paths.ProfileLog(request.Profile)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		readInput.Close()
		writeInput.Close()
		return fortihelper.Response{State: "failed", Error: err.Error()}
	}
	if err := chownUser(logFile, service.userID); err != nil {
		readInput.Close()
		writeInput.Close()
		logFile.Close()
		return fortihelper.Response{State: "failed", Error: err.Error()}
	}
	arguments := request.Arguments()
	configPath := ""
	managementDial := ""
	if protocol == fortihelper.ProtocolOpenVPN {
		configuration, sanitizeErr := openvpnconfig.Sanitize(request.Configuration)
		if sanitizeErr != nil {
			readInput.Close()
			writeInput.Close()
			logFile.Close()
			return fortihelper.Response{State: "failed", Error: sanitizeErr.Error()}
		}
		configFile, createErr := os.CreateTemp(service.paths.RuntimeDirectory, request.Profile+"-*.ovpn")
		if createErr != nil {
			readInput.Close()
			writeInput.Close()
			logFile.Close()
			return fortihelper.Response{State: "failed", Error: createErr.Error()}
		}
		configPath = configFile.Name()
		writeErr := configFile.Chmod(0600)
		if writeErr == nil {
			_, writeErr = configFile.Write([]byte(configuration))
		}
		closeErr := configFile.Close()
		if writeErr == nil {
			writeErr = closeErr
		}
		if writeErr != nil {
			readInput.Close()
			writeInput.Close()
			logFile.Close()
			_ = os.Remove(configPath)
			return fortihelper.Response{State: "failed", Error: writeErr.Error()}
		}
		managementArgs, dialTarget, managementErr := openVPNManagementArgs(service.paths.RuntimeDirectory, request.Profile, configPath)
		if managementErr != nil {
			readInput.Close()
			writeInput.Close()
			logFile.Close()
			_ = os.Remove(configPath)
			return fortihelper.Response{State: "failed", Error: managementErr.Error()}
		}
		managementDial = dialTarget
		arguments = append([]string{"--config", configPath, "--dev", openVPNDeviceType(), "--route-nopull", "--script-security", "1", "--auth-nocache", "--verb", "3"}, managementArgs...)
		if request.Username != "" {
			arguments = append(arguments, "--auth-user-pass")
		}
	}
	if protocol == fortihelper.ProtocolOpenConnect {
		scriptPath, scriptErr := engineSideBinary(filepath.Dir(executable), "vpntoris-vpnc-script")
		if scriptErr != nil {
			readInput.Close()
			writeInput.Close()
			logFile.Close()
			return fortihelper.Response{State: "failed", Error: "OpenConnect interface helper is missing"}
		}
		scriptPath, scriptErr = spaceFreeBinary(service.paths.RuntimeDirectory, scriptPath)
		if scriptErr != nil {
			readInput.Close()
			writeInput.Close()
			logFile.Close()
			return fortihelper.Response{State: "failed", Error: scriptErr.Error()}
		}
		arguments = []string{
			"--protocol=" + request.GatewayProtocol,
			"--script=" + scriptPath,
			"--timestamp",
			"--server=" + "https://" + net.JoinHostPort(request.Host, strconv.Itoa(request.Port)),
		}
		if request.Username != "" {
			arguments = append(arguments, "--user="+request.Username)
		}
		if request.Password != "" {
			arguments = append(arguments, "--passwd-on-stdin")
		}
		if request.ExternalBrowser {
			browserPath, browserErr := engineSideBinary(filepath.Dir(executable), "vpntoris-browser-open")
			if browserErr != nil {
				readInput.Close()
				writeInput.Close()
				logFile.Close()
				return fortihelper.Response{State: "failed", Error: "OpenConnect browser broker is missing"}
			}
			browserPath, browserErr = spaceFreeBinary(service.paths.RuntimeDirectory, browserPath)
			if browserErr != nil {
				readInput.Close()
				writeInput.Close()
				logFile.Close()
				return fortihelper.Response{State: "failed", Error: browserErr.Error()}
			}
			arguments = append(arguments, "--external-browser="+browserPath)
		}
	}
	command := exec.Command(executable, arguments...)
	command.Stdin = readInput
	command.Stdout = logFile
	command.Stderr = logFile
	command.Env = engineEnvironment(service.userID)
	configureProcess(command)
	if err := command.Start(); err != nil {
		readInput.Close()
		writeInput.Close()
		logFile.Close()
		if configPath != "" {
			_ = os.Remove(configPath)
		}
		return fortihelper.Response{State: "failed", Error: err.Error()}
	}
	readInput.Close()
	current := &session{request: request, command: command, input: writeInput, log: logFile, logPath: logPath, configPath: configPath, state: "connecting", username: request.Username, password: request.Password, startedAt: time.Now()}
	if protocol == fortihelper.ProtocolOpenVPN {
		current.managementPath = managementDial
	}
	current.request.Password = ""
	current.request.OTP = ""
	current.request.Configuration = ""
	service.sessions[request.Profile] = current
	if protocol != fortihelper.ProtocolOpenVPN && request.Password != "" {
		credential := request.Password + "\n"
		if _, err := current.input.Write([]byte(credential)); err != nil {
			service.stopLocked(current)
			return fortihelper.Response{State: "failed", Error: "could not supply VPN credential"}
		}
	}
	if protocol == fortihelper.ProtocolOpenVPN {
		current.input.Close()
		current.input = nil
		go service.manageOpenVPN(current)
	}
	if (protocol == fortihelper.ProtocolOpenConnect || protocol == fortihelper.ProtocolFortiGateSSL) && request.TwoFactor {
		current.state = "waiting-otp"
	}
	if request.OTP != "" {
		if _, err := current.input.Write([]byte(request.OTP + "\n")); err != nil {
			service.stopLocked(current)
			return fortihelper.Response{State: "failed", Error: "could not supply one-time password"}
		}
	}
	request.Password = ""
	request.OTP = ""
	go service.monitor(current)
	return fortihelper.Response{State: "connecting"}
}
func (service *Service) SendOTP(request fortihelper.Request) fortihelper.Response {
	service.mu.Lock()
	defer service.mu.Unlock()
	current := service.sessions[request.Profile]
	if current == nil || current.state != "connecting" && current.state != "waiting-otp" {
		return fortihelper.Response{State: "failed", Error: "profile is not waiting for authentication"}
	}
	if current.request.Protocol == fortihelper.ProtocolOpenVPN {
		if current.management == nil || current.challenge == "" {
			return fortihelper.Response{State: "failed", Error: "OpenVPN is not waiting for a challenge response"}
		}
		username, password := openVPNChallengeCredentials(current.challenge, current.challengeState, current.username, current.password, request.OTP)
		if err := writeManagementCredentials(current.management, username, password); err != nil {
			return fortihelper.Response{State: "failed", Error: "could not supply OpenVPN challenge response"}
		}
		current.password = ""
		current.challenge = ""
		current.challengeState = ""
		current.state = "connecting"
		return fortihelper.Response{State: current.state}
	}
	if current.request.Protocol == fortihelper.ProtocolIPSec {
		if current.otpPath == "" {
			return fortihelper.Response{State: "failed", Error: "IPsec is not waiting for an OTP"}
		}
		otpFile, err := os.OpenFile(current.otpPath, os.O_WRONLY, 0600)
		if err != nil {
			return fortihelper.Response{State: "failed", Error: "could not open the native IPsec OTP channel"}
		}
		_, writeErr := otpFile.WriteString(request.OTP + "\n")
		_ = otpFile.Close()
		if writeErr != nil {
			return fortihelper.Response{State: "failed", Error: "could not submit the native IPsec OTP"}
		}
		current.state = "connecting"
		return fortihelper.Response{State: current.state}
	}
	if current.input == nil {
		return fortihelper.Response{State: "failed", Error: "profile is not waiting for authentication"}
	}
	if _, err := current.input.Write([]byte(request.OTP + "\n")); err != nil {
		return fortihelper.Response{State: "failed", Error: "could not supply one-time password"}
	}
	current.state = "connecting"
	return fortihelper.Response{State: current.state}
}
func (service *Service) Stop(profile string) fortihelper.Response {
	service.mu.Lock()
	defer service.mu.Unlock()
	current := service.sessions[profile]
	if current == nil {
		return fortihelper.Response{State: "stopped"}
	}
	service.stopLocked(current)
	return fortihelper.Response{State: "stopped"}
}
func (service *Service) Reset() fortihelper.Response {
	service.mu.Lock()
	defer service.mu.Unlock()
	for _, current := range service.sessions {
		if current.state != "stopped" {
			service.stopLocked(current)
		}
	}
	service.sessions = make(map[string]*session)
	service.stopIPSecDaemonIfIdleLocked()
	return fortihelper.Response{State: "stopped"}
}
func (service *Service) Status(profile string) fortihelper.Response {
	service.mu.Lock()
	defer service.mu.Unlock()
	current := service.sessions[profile]
	if current == nil {
		return fortihelper.Response{State: "stopped"}
	}
	duration := int64(0)
	if !current.startedAt.IsZero() {
		duration = int64(time.Since(current.startedAt).Seconds())
	}
	return fortihelper.Response{State: current.state, Interface: current.interfaceName, Error: current.errorText, Received: current.received, Sent: current.sent, Duration: duration}
}
func (service *Service) stopLocked(current *session) {
	if current.request.Protocol == fortihelper.ProtocolIPSec {
		service.stopIPSecLocked(current)
		return
	}
	service.releaseNetworkLocked(current)
	if current.input != nil {
		current.input.Close()
		current.input = nil
	}
	if current.management != nil {
		current.management.Close()
		current.management = nil
	}
	if current.managementPath != "" && !strings.HasPrefix(current.managementPath, "tcp:") {
		_ = os.Remove(current.managementPath)
	}
	if current.otpPath != "" {
		_ = os.Remove(current.otpPath)
		current.otpPath = ""
	}
	terminateProcessGroup(current.command)
	current.state = "stopped"
}
func (service *Service) releaseNetworkLocked(current *session) {
	if current.transaction != nil && service.manager != nil {
		_ = service.manager.Deactivate(context.Background(), current.transaction)
		current.transaction = nil
		current.interfaceName = ""
		return
	}
	if current.interfaceName != "" {
		service.router.DeleteRoutes(current.interfaceName, current.request.Routes)
		current.interfaceName = ""
	}
}
func (service *Service) applyNetwork(current *session, interfaceName string) error {
	if service.manager == nil {
		return service.router.AddRoutes(interfaceName, current.request.Routes)
	}
	processID := 1
	if current.command != nil && current.command.Process != nil && current.command.Process.Pid > 0 {
		processID = current.command.Process.Pid
	}
	plan, err := nativeengine.BuildNetworkPlan(nativeengine.ProfileNetwork{
		Profile:    current.request.Profile,
		Interface:  interfaceName,
		ProcessID:  processID,
		Routes:     current.request.Routes,
		Domains:    current.request.Domains,
		DNSServers: current.request.DNSServers,
	})
	if err != nil {
		return err
	}
	transaction, err := service.manager.Activate(context.Background(), plan)
	if err != nil {
		return err
	}
	service.mu.Lock()
	current.transaction = transaction
	service.mu.Unlock()
	return nil
}
func (service *Service) finish(current *session, waitError error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.releaseNetworkLocked(current)
	if current.input != nil {
		current.input.Close()
		current.input = nil
	}
	if current.log != nil {
		current.log.Close()
		current.log = nil
	}
	if current.management != nil {
		current.management.Close()
		current.management = nil
	}
	if current.managementPath != "" && !strings.HasPrefix(current.managementPath, "tcp:") {
		_ = os.Remove(current.managementPath)
	}
	current.managementPath = ""
	if current.configPath != "" {
		_ = os.Remove(current.configPath)
		current.configPath = ""
	}
	if current.state == "stopped" {
		return
	}
	current.state = "failed"
	if waitError != nil {
		current.errorText = waitError.Error()
	} else {
		current.errorText = "VPN process exited"
	}
}
func (service *Service) fail(current *session, message string) {
	service.mu.Lock()
	defer service.mu.Unlock()
	current.errorText = message
	service.stopLocked(current)
	current.state = "failed"
}
func (service *Service) acceptCertificates(current *session) {
	data, err := os.ReadFile(current.logPath)
	if err != nil {
		return
	}
	prompts := strings.Count(string(data), "Enter 'yes' to accept")
	service.mu.Lock()
	defer service.mu.Unlock()
	for current.certAccepts < prompts && current.input != nil {
		if _, err := current.input.Write([]byte("yes\n")); err != nil {
			return
		}
		current.certAccepts++
	}
}
func (service *Service) monitor(current *session) {
	wait := make(chan error, 1)
	go func() { wait <- current.command.Wait() }()
	timeout := 45 * time.Second
	if current.request.TwoFactor {
		timeout = negotiationTimeout
	}
	deadline := time.NewTimer(timeout)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		select {
		case err := <-wait:
			service.finish(current, err)
			return
		case <-deadline.C:
			service.fail(current, "VPN interface was not created before timeout")
			go service.finish(current, <-wait)
			return
		case <-ticker.C:
			if current.request.Protocol == fortihelper.ProtocolOpenConnect {
				service.acceptCertificates(current)
			}
			interfaceName := interfaceFromLog(current.logPath, current.request.Protocol)
			if interfaceName == "" {
				continue
			}
			if current.request.Protocol == fortihelper.ProtocolOpenVPN && !logContains(current.logPath, "Initialization Sequence Completed") {
				continue
			}
			if err := service.applyNetwork(current, interfaceName); err != nil {
				service.fail(current, "network setup failed: "+err.Error())
				go service.finish(current, <-wait)
				return
			}
			service.mu.Lock()
			current.interfaceName = interfaceName
			current.state = "connected"
			current.password = ""
			service.mu.Unlock()
			go func() {
				err := <-wait
				service.finish(current, err)
			}()
			return
		}
	}
}
