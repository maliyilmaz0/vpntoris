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
	Action string   `json:"action"`
	Key    string   `json:"key"`
	Port   int      `json:"port"`
	Routes []string `json:"routes"`
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
	if err := os.WriteFile(launchDaemonPlist, []byte(plist), 0644); err != nil {
		return err
	}
	_ = exec.Command("/bin/launchctl", "bootout", "system/com.vpntoris.router").Run()
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
	return os.WriteFile(destination, data, 0755)
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
				if req.Action == "start" {
					err = start(req)
				} else if req.Action != "stop" {
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
	device := deviceName(req.Key)
	cmd := exec.Command(tun2socks, "-device", device, "-proxy", fmt.Sprintf("socks5://127.0.0.1:%d", req.Port))
	logFile, err := os.OpenFile(filepath.Join(stateDir, req.Key+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stateDir, req.Key+".pid"), []byte(strconv.Itoa(cmd.Process.Pid)), 0600); err != nil {
		return err
	}
	for tries := 0; tries < 30; tries++ {
		if exec.Command("/sbin/ifconfig", device).Run() == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if output, err := exec.Command("/sbin/ifconfig", device, "198.18.0.1", "198.18.0.1", "up").CombinedOutput(); err != nil {
		return fmt.Errorf("configure %s: %s", device, strings.TrimSpace(string(output)))
	}
	for _, route := range req.Routes {
		if output, err := exec.Command("/sbin/route", "-n", "add", "-net", route, "-interface", device).CombinedOutput(); err != nil {
			return fmt.Errorf("add route %s: %s", route, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func stop(req request) {
	device := deviceName(req.Key)
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
}

func deviceName(key string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return fmt.Sprintf("utun%d", 200+h.Sum32()%40)
}

func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
