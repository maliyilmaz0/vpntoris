//go:build darwin

package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"vpntoris-tray/internal/fortihelper"
	"vpntoris-tray/internal/nativeengine"
	"vpntoris-tray/internal/openvpnconfig"
)

const socketPath = "/var/run/vpntoris-native/helper.sock"
const logDirectory = "/var/log/vpntoris"
const runtimeDirectory = "/var/run/vpntoris-native"

var pppReadyPattern = regexp.MustCompile(`(?m)Interface (ppp[0-9]+) is UP\.`)
var utunReadyPattern = regexp.MustCompile(`(?m)Opened utun device (utun[0-9]+)`)
var openConnectReadyPattern = regexp.MustCompile(`(?m)^VPNTORIS_INTERFACE=((?:utun|tun|ppp)[0-9]+)$`)

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
	managementPath string
	management     net.Conn
	username       string
	password       string
	challenge      string
	challengeState string
	received       uint64
	sent           uint64
	startedAt      time.Time
}

type server struct {
	mu         sync.Mutex
	engineRoot string
	userID     int
	sessions   map[string]*session
}

func main() {
	if os.Geteuid() != 0 {
		fatal("native helper must run as root")
	}
	if len(os.Args) != 4 || os.Args[1] != "daemon" {
		fatal("usage: vpntoris-native-helper daemon uid engine-root")
	}
	uid, err := strconv.Atoi(os.Args[2])
	if err != nil || uid < 0 {
		fatal("invalid user id")
	}
	engineRoot, err := filepath.Abs(os.Args[3])
	if err != nil {
		fatal(err.Error())
	}
	engineRoot = filepath.Join(engineRoot, "darwin-"+runtime.GOARCH)
	service := &server{engineRoot: engineRoot, userID: uid, sessions: make(map[string]*session)}
	if err := service.serve(uid); err != nil {
		fatal(err.Error())
	}
}

func (service *server) serve(uid int) error {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0711); err != nil {
		return err
	}
	if err := os.MkdirAll(logDirectory, 0751); err != nil {
		return err
	}
	if err := os.MkdirAll(runtimeDirectory, 0700); err != nil {
		return err
	}
	if err := os.Chmod(logDirectory, 0751); err != nil {
		return err
	}
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	if err := os.Chown(socketPath, uid, -1); err != nil {
		listener.Close()
		return err
	}
	if err := os.Chmod(socketPath, 0600); err != nil {
		listener.Close()
		return err
	}
	for {
		connection, err := listener.Accept()
		if err != nil {
			continue
		}
		go service.handle(connection)
	}
}

func (service *server) handle(connection net.Conn) {
	defer connection.Close()
	decoder := json.NewDecoder(connection)
	decoder.DisallowUnknownFields()
	var request fortihelper.Request
	if err := decoder.Decode(&request); err != nil {
		writeResponse(connection, fortihelper.Response{State: "failed", Error: "invalid request"})
		return
	}
	if err := request.Validate(); err != nil {
		writeResponse(connection, fortihelper.Response{State: "failed", Error: err.Error()})
		return
	}
	var response fortihelper.Response
	switch request.Action {
	case fortihelper.ActionStart:
		response = service.start(request)
	case fortihelper.ActionOTP:
		response = service.sendOTP(request)
	case fortihelper.ActionStop:
		response = service.stop(request.Profile)
	case fortihelper.ActionStatus:
		response = service.status(request.Profile)
	}
	writeResponse(connection, response)
}

func (service *server) start(request fortihelper.Request) fortihelper.Response {
	service.mu.Lock()
	defer service.mu.Unlock()
	if current := service.sessions[request.Profile]; current != nil && current.state != "stopped" && current.state != "failed" {
		return fortihelper.Response{State: current.state, Interface: current.interfaceName, Error: "profile is already running"}
	}
	protocol := request.Protocol
	if protocol == "" {
		protocol = fortihelper.ProtocolFortiGateSSL
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
	logPath := filepath.Join(logDirectory, request.Profile+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		readInput.Close()
		writeInput.Close()
		return fortihelper.Response{State: "failed", Error: err.Error()}
	}
	if err := logFile.Chown(service.userID, -1); err != nil {
		readInput.Close()
		writeInput.Close()
		logFile.Close()
		return fortihelper.Response{State: "failed", Error: err.Error()}
	}
	arguments := request.Arguments()
	configPath := ""
	if protocol == fortihelper.ProtocolOpenVPN {
		configuration, sanitizeErr := openvpnconfig.Sanitize(request.Configuration)
		if sanitizeErr != nil {
			readInput.Close()
			writeInput.Close()
			logFile.Close()
			return fortihelper.Response{State: "failed", Error: sanitizeErr.Error()}
		}
		configFile, createErr := os.CreateTemp(runtimeDirectory, request.Profile+"-*.ovpn")
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
		managementPath := strings.TrimSuffix(configPath, ".ovpn") + ".sock"
		arguments = []string{"--config", configPath, "--dev", "tun", "--route-nopull", "--script-security", "1", "--auth-nocache", "--verb", "3", "--management", managementPath, "unix", "--management-query-passwords", "--management-hold", "--auth-retry", "interact"}
		if request.Username != "" {
			arguments = append(arguments, "--auth-user-pass")
		}
	}
	if protocol == fortihelper.ProtocolOpenConnect {
		scriptPath := filepath.Join(filepath.Dir(executable), "vpntoris-vpnc-script")
		if info, statErr := os.Stat(scriptPath); statErr != nil || info.Mode()&0111 == 0 {
			readInput.Close()
			writeInput.Close()
			logFile.Close()
			return fortihelper.Response{State: "failed", Error: "OpenConnect interface helper is missing"}
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
			browserPath := filepath.Join(filepath.Dir(executable), "vpntoris-browser-open")
			if info, statErr := os.Stat(browserPath); statErr != nil || info.Mode()&0111 == 0 {
				readInput.Close()
				writeInput.Close()
				logFile.Close()
				return fortihelper.Response{State: "failed", Error: "OpenConnect browser broker is missing"}
			}
			arguments = append(arguments, "--external-browser="+browserPath)
		}
	}
	command := exec.Command(executable, arguments...)
	command.Stdin = readInput
	command.Stdout = logFile
	command.Stderr = logFile
	command.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LANG=C", "LC_ALL=C", "VPNTORIS_USER_UID=" + strconv.Itoa(service.userID)}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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
		current.managementPath = strings.TrimSuffix(configPath, ".ovpn") + ".sock"
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
	if protocol == fortihelper.ProtocolOpenConnect && request.TwoFactor {
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

func (service *server) monitor(current *session) {
	wait := make(chan error, 1)
	go func() { wait <- current.command.Wait() }()
	timeout := 45 * time.Second
	if current.request.TwoFactor {
		timeout = 190 * time.Second
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
			interfaceName := interfaceFromLog(current.logPath, current.request.Protocol)
			if interfaceName == "" {
				continue
			}
			if current.request.Protocol == fortihelper.ProtocolOpenVPN && !logContains(current.logPath, "Initialization Sequence Completed") {
				continue
			}
			if err := addRoutes(interfaceName, current.request.Routes); err != nil {
				service.fail(current, err.Error())
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

func (service *server) sendOTP(request fortihelper.Request) fortihelper.Response {
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
	if current.input == nil {
		return fortihelper.Response{State: "failed", Error: "profile is not waiting for authentication"}
	}
	if _, err := current.input.Write([]byte(request.OTP + "\n")); err != nil {
		return fortihelper.Response{State: "failed", Error: "could not supply one-time password"}
	}
	current.state = "connecting"
	return fortihelper.Response{State: current.state}
}

func (service *server) stop(profile string) fortihelper.Response {
	service.mu.Lock()
	defer service.mu.Unlock()
	current := service.sessions[profile]
	if current == nil {
		return fortihelper.Response{State: "stopped"}
	}
	service.stopLocked(current)
	return fortihelper.Response{State: "stopped"}
}

func (service *server) stopLocked(current *session) {
	if current.interfaceName != "" {
		deleteRoutes(current.interfaceName, current.request.Routes)
		current.interfaceName = ""
	}
	if current.input != nil {
		current.input.Close()
		current.input = nil
	}
	if current.management != nil {
		current.management.Close()
		current.management = nil
	}
	if current.managementPath != "" {
		_ = os.Remove(current.managementPath)
	}
	if current.command != nil && current.command.Process != nil {
		_ = syscall.Kill(-current.command.Process.Pid, syscall.SIGTERM)
	}
	current.state = "stopped"
}

func (service *server) status(profile string) fortihelper.Response {
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

func (service *server) finish(current *session, waitError error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if current.interfaceName != "" {
		deleteRoutes(current.interfaceName, current.request.Routes)
		current.interfaceName = ""
	}
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
	if current.managementPath != "" {
		_ = os.Remove(current.managementPath)
		current.managementPath = ""
	}
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

func (service *server) fail(current *session, message string) {
	service.mu.Lock()
	defer service.mu.Unlock()
	current.errorText = message
	service.stopLocked(current)
	current.state = "failed"
}

func interfaceFromLog(path string, protocol string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	pattern := pppReadyPattern
	if protocol == fortihelper.ProtocolOpenVPN {
		pattern = utunReadyPattern
	} else if protocol == fortihelper.ProtocolOpenConnect {
		pattern = openConnectReadyPattern
	}
	matches := pattern.FindSubmatch(data)
	if len(matches) != 2 {
		return ""
	}
	interfaceName := string(matches[1])
	networkInterface, err := net.InterfaceByName(interfaceName)
	if err != nil || networkInterface.Flags&net.FlagUp == 0 {
		return ""
	}
	return interfaceName
}

func logContains(path string, value string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(data), value)
}

func (service *server) manageOpenVPN(current *session) {
	deadline := time.Now().Add(10 * time.Second)
	var connection net.Conn
	var err error
	for time.Now().Before(deadline) {
		connection, err = net.DialTimeout("unix", current.managementPath, 250*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		service.fail(current, "could not connect to OpenVPN management socket")
		return
	}
	service.mu.Lock()
	current.management = connection
	service.mu.Unlock()
	_, _ = connection.Write([]byte("state on\nbytecount 1\nhold release\n"))
	scanner := bufio.NewScanner(connection)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, ">BYTECOUNT:") {
			parts := strings.Split(strings.TrimPrefix(line, ">BYTECOUNT:"), ",")
			if len(parts) == 2 {
				received, receivedErr := strconv.ParseUint(parts[0], 10, 64)
				sent, sentErr := strconv.ParseUint(parts[1], 10, 64)
				if receivedErr == nil && sentErr == nil {
					service.mu.Lock()
					current.received = received
					current.sent = sent
					service.mu.Unlock()
				}
			}
		}
		if strings.Contains(line, ">PASSWORD:Need 'Auth' username/password") {
			service.mu.Lock()
			if strings.Contains(line, "SC:") {
				current.challenge = "static"
				current.state = "waiting-otp"
			} else if current.challenge == "dynamic" || current.challenge == "append" {
				current.state = "waiting-otp"
			} else {
				err = writeManagementCredentials(connection, current.username, current.password)
				if !current.request.TwoFactor {
					current.password = ""
				}
			}
			service.mu.Unlock()
			if err != nil {
				service.fail(current, "could not supply OpenVPN credentials")
				return
			}
		}
		if strings.Contains(line, ">PASSWORD:Verification Failed:") {
			dynamic := false
			if index := strings.Index(line, "CRV1:"); index >= 0 {
				parts := strings.SplitN(line[index:], ":", 5)
				if len(parts) == 5 {
					decoded, decodeErr := base64.StdEncoding.DecodeString(parts[3])
					if decodeErr == nil {
						service.mu.Lock()
						current.username = string(decoded)
						current.challenge = "dynamic"
						current.challengeState = parts[2]
						service.mu.Unlock()
						dynamic = true
					}
				}
			}
			if !dynamic && current.request.TwoFactor {
				service.mu.Lock()
				current.challenge = "append"
				service.mu.Unlock()
			}
		}
	}
}

func writeManagementCredentials(connection net.Conn, username string, password string) error {
	_, err := fmt.Fprintf(connection, "username \"Auth\" \"%s\"\npassword \"Auth\" \"%s\"\n", managementEscape(username), managementEscape(password))
	return err
}

func openVPNChallengeCredentials(challenge string, state string, username string, password string, otp string) (string, string) {
	switch challenge {
	case "static":
		password = "SCRV1:" + base64.StdEncoding.EncodeToString([]byte(password)) + ":" + base64.StdEncoding.EncodeToString([]byte(otp))
	case "dynamic":
		password = "CRV1::" + state + "::" + otp
	case "append":
		password += otp
	}
	return username, password
}

func managementEscape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func addRoutes(interfaceName string, routes []string) error {
	applied := make([]string, 0, len(routes))
	for _, route := range routes {
		output, err := exec.Command("/sbin/route", "-n", "add", "-net", route, "-interface", interfaceName).CombinedOutput()
		if err != nil {
			deleteRoutes(interfaceName, applied)
			return fmt.Errorf("add route %s: %s", route, strings.TrimSpace(string(output)))
		}
		applied = append(applied, route)
	}
	return nil
}

func deleteRoutes(interfaceName string, routes []string) {
	for index := len(routes) - 1; index >= 0; index-- {
		_ = exec.Command("/sbin/route", "-n", "delete", "-net", routes[index], "-interface", interfaceName).Run()
	}
}

func writeResponse(connection net.Conn, response fortihelper.Response) {
	_ = json.NewEncoder(connection).Encode(response)
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
