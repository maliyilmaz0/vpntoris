//go:build darwin

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"vpntoris-tray/internal/fortihelper"
	"vpntoris-tray/internal/nativeengine"
)

const socketPath = "/var/run/vpntoris-native/helper.sock"
const logDirectory = "/var/log/vpntoris"

type session struct {
	request       fortihelper.Request
	command       *exec.Cmd
	input         *os.File
	log           *os.File
	interfaceName string
	state         string
	errorText     string
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
	service := &server{engineRoot: engineRoot, userID: uid, sessions: make(map[string]*session)}
	if err := service.serve(uid); err != nil {
		fatal(err.Error())
	}
}

func (service *server) serve(uid int) error {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0711); err != nil {
		return err
	}
	if err := os.MkdirAll(logDirectory, 0750); err != nil {
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
	manifestPath := filepath.Join(service.engineRoot, "openfortivpn", "manifest.json")
	_, executable, err := nativeengine.LoadEngineManifest(service.engineRoot, manifestPath)
	if err != nil {
		return fortihelper.Response{State: "failed", Error: err.Error()}
	}
	before, err := pppInterfaces()
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
	command := exec.Command(executable, request.Arguments()...)
	command.Stdin = readInput
	command.Stdout = logFile
	command.Stderr = logFile
	command.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LANG=C", "LC_ALL=C"}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		readInput.Close()
		writeInput.Close()
		logFile.Close()
		return fortihelper.Response{State: "failed", Error: err.Error()}
	}
	readInput.Close()
	current := &session{request: request, command: command, input: writeInput, log: logFile, state: "connecting"}
	current.request.Password = ""
	current.request.OTP = ""
	service.sessions[request.Profile] = current
	if _, err := current.input.Write([]byte(request.Password + "\n")); err != nil {
		service.stopLocked(current)
		return fortihelper.Response{State: "failed", Error: "could not supply VPN credential"}
	}
	if request.OTP != "" {
		if _, err := current.input.Write([]byte(request.OTP + "\n")); err != nil {
			service.stopLocked(current)
			return fortihelper.Response{State: "failed", Error: "could not supply one-time password"}
		}
	}
	request.Password = ""
	request.OTP = ""
	go service.monitor(current, before)
	return fortihelper.Response{State: "connecting"}
}

func (service *server) monitor(current *session, before map[string]bool) {
	wait := make(chan error, 1)
	go func() { wait <- current.command.Wait() }()
	deadline := time.NewTimer(45 * time.Second)
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
			interfaceName := newPPPInterface(before)
			if interfaceName == "" {
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
	if current == nil || current.input == nil || current.state != "connecting" {
		return fortihelper.Response{State: "failed", Error: "profile is not waiting for authentication"}
	}
	if _, err := current.input.Write([]byte(request.OTP + "\n")); err != nil {
		return fortihelper.Response{State: "failed", Error: "could not supply one-time password"}
	}
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
	return fortihelper.Response{State: current.state, Interface: current.interfaceName, Error: current.errorText}
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

func pppInterfaces() (map[string]bool, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	result := make(map[string]bool)
	for _, networkInterface := range interfaces {
		if strings.HasPrefix(networkInterface.Name, "ppp") {
			result[networkInterface.Name] = true
		}
	}
	return result, nil
}

func newPPPInterface(before map[string]bool) string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, networkInterface := range interfaces {
		if strings.HasPrefix(networkInterface.Name, "ppp") && !before[networkInterface.Name] && networkInterface.Flags&net.FlagUp != 0 {
			return networkInterface.Name
		}
	}
	return ""
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
