//go:build darwin

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"vpntoris-tray/internal/fortihelper"
)

const socketPath = "/var/run/vpntoris-native/helper.sock"

type localProfile struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Host   string `json:"host"`
	Port   string `json:"port"`
	User   string `json:"user"`
	Routes string `json:"routes"`
}

func main() {
	if len(os.Args) < 3 {
		fatal("usage: vpntoris-native-client start profile [route] [certificate-sha256] | otp profile code | stop profile | status profile")
	}
	action := os.Args[1]
	name := os.Args[2]
	profileID := privateProfileID(name)
	var request fortihelper.Request
	switch action {
	case fortihelper.ActionStart:
		profile, err := loadProfile(name)
		if err != nil {
			fatal(err.Error())
		}
		password, err := keychainPassword(name)
		if err != nil {
			fatal("VPN credential is unavailable in Keychain")
		}
		port, err := strconv.Atoi(profile.Port)
		if err != nil {
			fatal("profile has an invalid port")
		}
		routes := splitValues(profile.Routes)
		if len(os.Args) >= 4 && os.Args[3] != "" {
			routes = []string{os.Args[3]}
		}
		certificate := ""
		if len(os.Args) >= 5 {
			certificate = strings.ReplaceAll(os.Args[4], ":", "")
		}
		request = fortihelper.Request{Action: action, Profile: profileID, Host: profile.Host, Port: port, Username: profile.User, Password: password, TrustedCert: certificate, Routes: routes}
	case fortihelper.ActionOTP:
		if len(os.Args) != 4 {
			fatal("OTP code is required")
		}
		request = fortihelper.Request{Action: action, Profile: profileID, OTP: os.Args[3]}
	case fortihelper.ActionStop, fortihelper.ActionStatus:
		request = fortihelper.Request{Action: action, Profile: profileID}
	default:
		fatal("unknown action")
	}
	if err := request.Validate(); err != nil {
		fatal(err.Error())
	}
	response, err := send(request)
	request.Password = ""
	request.OTP = ""
	if err != nil {
		fatal(err.Error())
	}
	data, _ := json.Marshal(response)
	fmt.Println(string(data))
	if response.Error != "" {
		os.Exit(1)
	}
}

func loadProfile(name string) (*localProfile, error) {
	path := filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "VPNToris", "configs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var profiles []localProfile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, err
	}
	for index := range profiles {
		if profiles[index].Name == name {
			if profiles[index].Type != "openfortivpn" {
				return nil, fmt.Errorf("profile is not a FortiGate SSL VPN")
			}
			return &profiles[index], nil
		}
	}
	return nil, fmt.Errorf("profile was not found")
}

func keychainPassword(name string) (string, error) {
	command := exec.Command("/usr/bin/security", "find-generic-password", "-w", "-s", "com.vpntoris.credentials", "-a", name+".password")
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(output), "\n"), nil
}

func privateProfileID(name string) string {
	digest := sha256.Sum256([]byte(name))
	return "profile-" + hex.EncodeToString(digest[:8])
}

func splitValues(value string) []string {
	fields := strings.FieldsFunc(value, func(character rune) bool {
		return character == ',' || character == '\n' || character == ' ' || character == '\t'
	})
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func send(request fortihelper.Request) (*fortihelper.Response, error) {
	connection, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, err
	}
	defer connection.Close()
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return nil, err
	}
	var response fortihelper.Response
	if err := json.NewDecoder(connection).Decode(&response); err != nil {
		return nil, err
	}
	return &response, nil
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
