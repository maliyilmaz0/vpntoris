package nativehelper

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"time"
)

func serverCertificatePin(host string, port int) (string, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	connection, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(host, strconv.Itoa(port)), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return "", err
	}
	defer connection.Close()
	peers := connection.ConnectionState().PeerCertificates
	if len(peers) == 0 {
		return "", fmt.Errorf("server presented no certificate")
	}
	sum := sha256.Sum256(peers[0].Raw)
	return "pin-sha256:" + base64.StdEncoding.EncodeToString(sum[:]), nil
}
