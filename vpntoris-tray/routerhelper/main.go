package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type request struct {
	Action  string   `json:"action"`
	Key     string   `json:"key"`
	Port    int      `json:"port"`
	Routes  []string `json:"routes"`
	Domains []string `json:"domains"`
	DNSPort int      `json:"dnsPort"`
}

const stateDir = "/var/run/vpntoris"
const socketPath = stateDir + "/router.sock"
const installedHelper = "/Library/PrivilegedHelperTools/com.vpntoris.router"
const installedTun2Socks = "/Library/PrivilegedHelperTools/com.vpntoris.tun2socks"
const launchDaemonPlist = "/Library/LaunchDaemons/com.vpntoris.router.plist"

func main() {
	if os.Geteuid() != 0 {
		fatal("router helper must run as root")
	}
	if len(os.Args) == 3 && os.Args[1] == "install" {
		uid, err := strconv.Atoi(os.Args[2])
		if err != nil || uid < 0 {
			fatal("invalid user id")
		}
		if err := install(uid); err != nil {
			fatal(err.Error())
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "daemon" {
		uid, err := strconv.Atoi(os.Args[2])
		if err != nil || uid < 0 {
			fatal("invalid user id")
		}
		if err := serve(uid); err != nil {
			fatal(err.Error())
		}
		return
	}
	if len(os.Args) != 3 || (os.Args[1] != "start" && os.Args[1] != "stop") {
		fatal("usage: vpntoris-route-helper prepare | start|stop request.json")
	}
	data, err := os.ReadFile(os.Args[2])
	if err != nil {
		fatal(err.Error())
	}
	var req request
	if err := json.Unmarshal(data, &req); err != nil {
		fatal(err.Error())
	}
	if err := validate(&req); err != nil {
		fatal(err.Error())
	}
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		fatal(err.Error())
	}
	stop(req)
	if os.Args[1] == "start" {
		if err := start(req); err != nil {
			fatal(err.Error())
		}
	}
}

func install(uid int) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	_ = exec.Command("/bin/launchctl", "bootout", "system/com.vpntoris.router").Run()
	_ = os.Remove(socketPath)
	if err := copyExecutable(exe, installedHelper); err != nil {
		return err
	}
	if err := copyExecutable(filepath.Join(filepath.Dir(exe), "tun2socks"), installedTun2Socks); err != nil {
		return err
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.vpntoris.router</string>
<key>ProgramArguments</key><array><string>%s</string><string>daemon</string><string>%d</string></array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
</dict></plist>`, installedHelper, uid)
	if err := writeAtomic(launchDaemonPlist, []byte(plist), 0644); err != nil {
		return err
	}
	if output, err := exec.Command("/bin/launchctl", "bootstrap", "system", launchDaemonPlist).CombinedOutput(); err != nil {
		return fmt.Errorf("install launch daemon: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func copyExecutable(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return writeAtomic(destination, data, 0755)
}

func writeAtomic(destination string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".vpntoris-install-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, destination)
}

func serve(uid int) error {
	if err := os.MkdirAll(stateDir, 0711); err != nil {
		return err
	}
	if err := os.Chmod(stateDir, 0711); err != nil {
		return err
	}
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	if err := os.Chown(socketPath, uid, -1); err != nil {
		return err
	}
	if err := os.Chmod(socketPath, 0600); err != nil {
		return err
	}
	for {
		connection, err := listener.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer connection.Close()
			var req request
			err := json.NewDecoder(connection).Decode(&req)
			if err == nil {
				err = validate(&req)
			}
			if err == nil {
				stop(req)
				if req.Action == "start-v2" {
					err = start(req)
				} else if req.Action != "stop-v2" {
					err = fmt.Errorf("invalid action")
				}
			}
			message := ""
			if err != nil {
				message = err.Error()
			}
			_ = json.NewEncoder(connection).Encode(map[string]string{"error": message})
		}()
	}
}

func validate(req *request) error {
	if req.Key == "" || len(req.Key) > 80 {
		return fmt.Errorf("invalid profile key")
	}
	for _, r := range req.Key {
		if !(r == '-' || r == '_' || r == '.' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return fmt.Errorf("invalid profile key")
		}
	}
	if req.Port < 0 || req.Port > 65535 {
		return fmt.Errorf("invalid SOCKS port")
	}
	if req.DNSPort < 0 || req.DNSPort > 65535 {
		return fmt.Errorf("invalid DNS port")
	}
	if len(req.Domains) > 32 {
		return fmt.Errorf("too many split DNS domains")
	}
	for _, domain := range req.Domains {
		if !validDomain(domain) {
			return fmt.Errorf("invalid split DNS domain: %s", domain)
		}
	}
	if len(req.Routes) > 64 {
		return fmt.Errorf("too many routes")
	}
	for _, value := range req.Routes {
		_, block, err := net.ParseCIDR(value)
		if err != nil || block.IP.To4() == nil {
			return fmt.Errorf("invalid IPv4 route: %s", value)
		}
	}
	return nil
}

func start(req request) error {
	if req.Port == 0 || len(req.Routes) == 0 {
		return fmt.Errorf("SOCKS port and routes are required")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	tun2socks := filepath.Join(filepath.Dir(exe), "tun2socks")
	if exe == installedHelper {
		tun2socks = installedTun2Socks
	}
	devicePath := filepath.Join(stateDir, req.Key+".device")
	_ = os.Remove(devicePath)
	cmd := exec.Command(tun2socks, tun2SocksArguments("utun", req.Port)...)
	cmd.Env = append(os.Environ(), "WG_TUN_NAME_FILE="+devicePath)
	logPath := filepath.Join(stateDir, req.Key+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if err := logFile.Chmod(0644); err != nil {
		logFile.Close()
		return err
	}
	defer logFile.Close()
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		return err
	}
	processDone := make(chan error, 1)
	go func() { processDone <- cmd.Wait() }()
	if err := os.WriteFile(filepath.Join(stateDir, req.Key+".pid"), []byte(strconv.Itoa(cmd.Process.Pid)), 0600); err != nil {
		return err
	}
	device := ""
	for tries := 0; tries < 50; tries++ {
		select {
		case processError := <-processDone:
			logData, _ := os.ReadFile(logPath)
			message := strings.TrimSpace(string(logData))
			if message == "" {
				message = fmt.Sprintf("tun2socks exited: %v", processError)
			}
			return fmt.Errorf("tun2socks could not create a macOS tunnel: %s", message)
		default:
		}
		if data, readError := os.ReadFile(devicePath); readError == nil {
			candidate := strings.TrimSpace(string(data))
			if strings.HasPrefix(candidate, "utun") && exec.Command("/sbin/ifconfig", candidate).Run() == nil {
				device = candidate
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if device == "" {
		_ = cmd.Process.Kill()
		return fmt.Errorf("tun2socks did not report the macOS tunnel interface")
	}
	if output, err := exec.Command("/sbin/ifconfig", device, "198.18.0.1", "198.18.0.1", "up").CombinedOutput(); err != nil {
		return fmt.Errorf("configure %s: %s", device, strings.TrimSpace(string(output)))
	}
	for _, route := range req.Routes {
		if output, err := exec.Command("/sbin/route", "-n", "add", "-net", route, "-interface", device).CombinedOutput(); err != nil {
			return fmt.Errorf("add route %s: %s", route, strings.TrimSpace(string(output)))
		}
	}
	if req.DNSPort > 0 && len(req.Domains) > 0 {
		if err := os.MkdirAll("/etc/resolver", 0755); err != nil {
			return err
		}
		for _, domain := range req.Domains {
			content := fmt.Sprintf("nameserver 127.0.0.1\nport %d\n", req.DNSPort)
			if err := os.WriteFile(filepath.Join("/etc/resolver", domain), []byte(content), 0644); err != nil {
				return err
			}
		}
		_ = os.WriteFile(filepath.Join(stateDir, req.Key+".domains"), []byte(strings.Join(req.Domains, "\n")), 0600)
	}
	return nil
}

func stop(req request) {
	device := deviceName(req.Key)
	devicePath := filepath.Join(stateDir, req.Key+".device")
	if data, err := os.ReadFile(devicePath); err == nil {
		candidate := strings.TrimSpace(string(data))
		if strings.HasPrefix(candidate, "utun") {
			device = candidate
		}
	}
	for _, route := range req.Routes {
		_ = exec.Command("/sbin/route", "-n", "delete", "-net", route, "-interface", device).Run()
	}
	pidPath := filepath.Join(stateDir, req.Key+".pid")
	if data, err := os.ReadFile(pidPath); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 1 {
			_ = syscall.Kill(pid, syscall.SIGTERM)
		}
	}
	_ = os.Remove(pidPath)
	_ = os.Remove(devicePath)
	domainsPath := filepath.Join(stateDir, req.Key+".domains")
	if data, err := os.ReadFile(domainsPath); err == nil {
		for _, domain := range strings.Split(string(data), "\n") {
			if validDomain(domain) {
				_ = os.Remove(filepath.Join("/etc/resolver", domain))
			}
		}
	}
	_ = os.Remove(domainsPath)
}

func validDomain(domain string) bool {
	if domain == "" || len(domain) > 253 || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	for _, character := range domain {
		if !(character == '-' || character == '.' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9') {
			return false
		}
	}
	return true
}

func deviceName(key string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return fmt.Sprintf("utun%d", 200+h.Sum32()%40)
}

func tun2SocksArguments(device string, port int) []string {
	return []string{"--device", device, "--proxy", fmt.Sprintf("socks5://127.0.0.1:%d", port)}
}

func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
