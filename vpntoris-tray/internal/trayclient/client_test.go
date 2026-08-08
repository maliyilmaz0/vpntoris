package trayclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProfilesAndActions(t *testing.T) {
	var gotAction, gotName, gotPassword, gotOTP string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/api/profiles":
			_ = json.NewEncoder(response).Encode([]Profile{{
				Name: "office", Host: "vpn.example.invalid", Type: "openvpn", Connected: false, NeedsOTP: true,
			}})
		case request.URL.Path == "/api/action":
			gotAction = request.URL.Query().Get("action")
			gotName = request.URL.Query().Get("name")
			gotPassword = request.Header.Get("X-VPNToris-Password")
			gotOTP = request.Header.Get("X-VPNToris-OTP")
			response.WriteHeader(http.StatusNoContent)
		case request.URL.Path == "/api/reset":
			response.WriteHeader(http.StatusNoContent)
		case request.URL.Path == "/api/logs":
			_, _ = response.Write([]byte("line-one\n"))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, HTTPClient: server.Client()}
	profiles, err := client.Profiles()
	if err != nil || len(profiles) != 1 || profiles[0].Name != "office" || !profiles[0].NeedsOTP {
		t.Fatalf("profiles = %#v err=%v", profiles, err)
	}
	if err := client.Connect("office", "secret", ""); err != nil {
		t.Fatal(err)
	}
	if gotAction != "connect" || gotName != "office" || gotPassword != "secret" {
		t.Fatalf("connect action = %s %s password=%q", gotAction, gotName, gotPassword)
	}
	if err := client.SubmitOTP("office", "123456"); err != nil {
		t.Fatal(err)
	}
	if gotAction != "otp" || gotOTP != "123456" {
		t.Fatalf("otp action = %s otp=%q", gotAction, gotOTP)
	}
	if err := client.Disconnect("office"); err != nil {
		t.Fatal(err)
	}
	if gotAction != "disconnect" {
		t.Fatalf("disconnect action = %s", gotAction)
	}
	if err := client.ResetAll(); err != nil {
		t.Fatal(err)
	}
	logs, err := client.Logs("office")
	if err != nil || !strings.Contains(logs, "line-one") {
		t.Fatalf("logs = %q err=%v", logs, err)
	}
}
func TestConnectErrorSurfaced(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Error(response, "gateway refused", http.StatusBadGateway)
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, HTTPClient: server.Client()}
	err := client.Connect("office", "x", "")
	if err == nil || !strings.Contains(err.Error(), "gateway refused") {
		t.Fatalf("err = %v", err)
	}
}
