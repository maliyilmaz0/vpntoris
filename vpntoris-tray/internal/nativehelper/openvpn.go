package nativehelper

import (
	"bufio"
	"encoding/base64"
	"net"
	"strconv"
	"strings"
	"time"
)

func (service *Service) manageOpenVPN(current *session) {
	deadline := time.Now().Add(10 * time.Second)
	var connection net.Conn
	var err error
	for time.Now().Before(deadline) {
		connection, err = dialManagement(current.managementPath, 250*time.Millisecond)
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
