package nativehelper

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"vpntoris-tray/internal/fortihelper"
	"vpntoris-tray/internal/nativeengine"
)

func (service *Service) startIPSecLocked(request fortihelper.Request) fortihelper.Response {
	if err := service.ensureIPSecDaemonLocked(); err != nil {
		return fortihelper.Response{State: "failed", Error: err.Error()}
	}
	configPath := filepath.Join(service.paths.RuntimeDirectory, request.Profile+"-ipsec.conf")
	if err := os.WriteFile(configPath, []byte(request.IPSecConfiguration()), 0600); err != nil {
		return fortihelper.Response{State: "failed", Error: err.Error()}
	}
	logPath := service.paths.ProfileLog(request.Profile)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		_ = os.Remove(configPath)
		return fortihelper.Response{State: "failed", Error: err.Error()}
	}
	_ = chownUser(logFile, service.userID)
	otpPath := ""
	if request.TwoFactor {
		otpPath = filepath.Join(service.paths.RuntimeDirectory, "ipsec-otp")
		_ = os.Remove(otpPath)
		if err := createOTPChannel(otpPath); err != nil {
			logFile.Close()
			_ = os.Remove(configPath)
			service.stopIPSecDaemonIfIdleLocked()
			return fortihelper.Response{State: "failed", Error: "could not create native IPsec OTP channel"}
		}
	}
	load := service.ipsecCommandFor("--load-all", "--file", configPath, "--noprompt")
	load.Stdout = logFile
	load.Stderr = logFile
	if err := load.Run(); err != nil {
		logFile.Close()
		_ = os.Remove(configPath)
		_ = os.Remove(otpPath)
		service.stopIPSecDaemonIfIdleLocked()
		return fortihelper.Response{State: "failed", Error: "could not load native IPsec profile"}
	}
	request.Password = ""
	request.IPSec.PreSharedKey = ""
	state := "connecting"
	if request.TwoFactor {
		state = "waiting-otp"
	}
	current := &session{request: request, logPath: logPath, configPath: configPath, otpPath: otpPath, state: state, startedAt: time.Now()}
	service.sessions[request.Profile] = current
	initiate := service.ipsecCommandFor("--initiate", "--child", "net-"+request.Profile, "--timeout", strconv.FormatInt(int64(negotiationTimeout/time.Second), 10), "--loglevel", "2")
	initiate.Stdout = logFile
	initiate.Stderr = logFile
	go func() {
		err := initiate.Run()
		_ = logFile.Close()
		service.mu.Lock()
		defer service.mu.Unlock()
		if current.state == "stopped" || current.state == "failed" {
			return
		}
		if err != nil {
			current.state = "failed"
			current.errorText = "native IPsec negotiation failed"
			_ = os.Remove(current.otpPath)
			_ = os.Remove(current.configPath)
			service.stopIPSecDaemonIfIdleLocked()
			return
		}
		current.state = "connected"
	}()
	return fortihelper.Response{State: state}
}
func (service *Service) ensureIPSecDaemonLocked() error {
	if service.ipsecCommand != nil && service.ipsecCommand.Process != nil {
		return nil
	}
	manifestPath := filepath.Join(service.engineRoot, "strongswan", "manifest.json")
	_, executable, err := nativeengine.LoadEngineManifest(service.engineRoot, manifestPath)
	if err != nil {
		return err
	}
	enginePath := filepath.Dir(filepath.Dir(executable))
	pluginRuntimePath := filepath.Join(service.paths.RuntimeDirectory, "plugins")
	if err := os.MkdirAll(pluginRuntimePath, 0700); err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(enginePath, "plugins"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		source := filepath.Join(enginePath, "plugins", entry.Name())
		target := filepath.Join(pluginRuntimePath, entry.Name())
		_ = os.Remove(target)
		if err := os.Symlink(source, target); err != nil {
			return err
		}
	}
	service.ipsecSocket = filepath.Join(service.paths.RuntimeDirectory, "charon.vici")
	service.cleanupOrphanedIPSecDaemonLocked()
	_ = os.Remove(service.ipsecSocket)
	_ = os.Remove(filepath.Join(service.paths.RuntimeDirectory, "charon.pid"))
	service.ipsecConfig = filepath.Join(service.paths.RuntimeDirectory, "strongswan.conf")
	service.ipsecSwanctl = filepath.Join(enginePath, "bin", "swanctl")
	configuration := buildCharonConfiguration(filepath.Join(enginePath, "plugins"), service.ipsecSocket, filepath.Join(service.paths.RuntimeDirectory, "charon.pid"))
	if err := os.WriteFile(service.ipsecConfig, []byte(configuration), 0600); err != nil {
		return err
	}
	logPath := filepath.Join(service.paths.LogDirectory, "ipsec-daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return err
	}
	_ = chownUser(logFile, service.userID)
	command := exec.Command(executable)
	command.Stdout = logFile
	command.Stderr = logFile
	command.Env = service.ipsecEnvironment()
	configureProcess(command)
	if err := command.Start(); err != nil {
		logFile.Close()
		return err
	}
	service.ipsecCommand = command
	service.ipsecLog = logFile
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if info, statErr := os.Stat(service.ipsecSocket); statErr == nil && info.Mode()&os.ModeSocket != 0 && service.ipsecVICIReady() {
			go service.waitForIPSecDaemon(command, logFile)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	terminateProcessGroup(command)
	service.ipsecCommand = nil
	logFile.Close()
	return fmt.Errorf("native IPsec daemon did not become ready")
}
func (service *Service) ipsecVICIReady() bool {
	connection, err := net.DialTimeout("unix", service.ipsecSocket, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}
func (service *Service) waitForIPSecDaemon(command *exec.Cmd, logFile *os.File) {
	waitError := command.Wait()
	service.mu.Lock()
	defer service.mu.Unlock()
	_ = logFile.Close()
	if service.ipsecCommand != command {
		return
	}
	service.ipsecCommand = nil
	service.ipsecLog = nil
	_ = os.Remove(service.ipsecSocket)
	for _, current := range service.sessions {
		if current.request.Protocol == fortihelper.ProtocolIPSec && current.state == "connected" {
			current.state = "failed"
			current.errorText = "native IPsec service stopped"
			if waitError != nil {
				current.errorText = "native IPsec service exited unexpectedly"
			}
		}
	}
}
func (service *Service) ipsecEnvironment() []string {
	return []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LANG=C", "LC_ALL=C", "STRONGSWAN_CONF=" + service.ipsecConfig}
}
func (service *Service) cleanupOrphanedIPSecDaemonLocked() {
	pattern := filepath.Join(service.engineRoot, "strongswan", "bin", "charon")
	output, err := exec.Command("pgrep", "-f", pattern).Output()
	if err == nil {
		for _, line := range strings.Fields(string(output)) {
			pid, parseErr := strconv.Atoi(line)
			if parseErr == nil && pid > 1 && pid != os.Getpid() {
				if process, findErr := os.FindProcess(pid); findErr == nil {
					_ = process.Kill()
				}
			}
		}
	}
	pidPath := filepath.Join(service.paths.RuntimeDirectory, "charon.pid")
	if data, readErr := os.ReadFile(pidPath); readErr == nil {
		if pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data))); parseErr == nil {
			if !processAlive(pid) {
				_ = os.Remove(pidPath)
			}
		}
	}
}
func (service *Service) ipsecCommandFor(arguments ...string) *exec.Cmd {
	arguments = append(arguments, "--uri", "unix://"+service.ipsecSocket)
	command := exec.Command(service.ipsecSwanctl, arguments...)
	command.Env = service.ipsecEnvironment()
	return command
}
func (service *Service) stopIPSecLocked(current *session) {
	terminate := service.ipsecCommandFor("--terminate", "--ike", current.request.Profile, "--timeout", "10")
	_ = terminate.Run()
	if current.configPath != "" {
		_ = os.Remove(current.configPath)
		current.configPath = ""
	}
	if current.otpPath != "" {
		_ = os.Remove(current.otpPath)
		current.otpPath = ""
	}
	current.state = "stopped"
	service.stopIPSecDaemonIfIdleLocked()
}
func (service *Service) stopIPSecDaemonIfIdleLocked() {
	for _, candidate := range service.sessions {
		if candidate.request.Protocol == fortihelper.ProtocolIPSec && candidate.state == "connected" {
			return
		}
	}
	if service.ipsecCommand != nil && service.ipsecCommand.Process != nil {
		command := service.ipsecCommand
		service.ipsecCommand = nil
		service.ipsecLog = nil
		terminateProcessGroup(command)
		_ = os.Remove(service.ipsecSocket)
		_ = os.Remove(service.ipsecConfig)
	}
}
