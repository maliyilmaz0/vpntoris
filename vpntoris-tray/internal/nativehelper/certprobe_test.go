package nativehelper

import (
	"crypto/sha256"
	"encoding/base64"
	"net"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestServerCertificatePin(t *testing.T) {
	server := httptest.NewTLSServer(nil)
	defer server.Close()
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split test server address: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse test server port: %v", err)
	}
	pin, err := serverCertificatePin(host, port)
	if err != nil {
		t.Fatalf("serverCertificatePin: %v", err)
	}
	sum := sha256.Sum256(server.Certificate().Raw)
	expected := "pin-sha256:" + base64.StdEncoding.EncodeToString(sum[:])
	if pin != expected {
		t.Fatalf("pin = %q, want %q", pin, expected)
	}
}

func TestServerCertificatePinUnreachable(t *testing.T) {
	if _, err := serverCertificatePin("127.0.0.1", 1); err == nil {
		t.Fatal("expected an error for an unreachable server")
	}
}
