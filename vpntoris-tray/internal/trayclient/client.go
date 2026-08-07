package trayclient

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to the local VPNToris controller HTTP API.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// Profile is the controller profile list projection used by the tray.
type Profile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Protocol    string `json:"protocol"`
	Host        string `json:"host"`
	ActiveHost  string `json:"activeGateway"`
	Routes      string `json:"routes"`
	Connected   bool   `json:"connected"`
	TwoFactor   bool   `json:"twoFactor"`
	NeedsOTP    bool   `json:"needsOtp"`
	RouteStatus string `json:"routeStatus"`
}

// New returns a client for the default localhost controller.
func New() *Client {
	return &Client{
		BaseURL: "http://127.0.0.1:17984",
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Profiles lists configured VPN profiles.
func (client *Client) Profiles() ([]Profile, error) {
	var profiles []Profile
	if err := client.getJSON("/api/profiles", &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

// Connect starts a VPN session. Password and PSK are sent as headers only.
func (client *Client) Connect(name, password, psk string) error {
	return client.postAction("connect", name, map[string]string{
		"X-VPNToris-Password": password,
		"X-VPNToris-PSK":      psk,
	})
}

// Disconnect stops a VPN session.
func (client *Client) Disconnect(name string) error {
	return client.postAction("disconnect", name, nil)
}

// SubmitOTP delivers a challenge response for an in-flight session.
func (client *Client) SubmitOTP(name, otp string) error {
	return client.postAction("otp", name, map[string]string{
		"X-VPNToris-OTP": otp,
	})
}

// ResetAll closes every connection and clears helper state without deleting profiles.
func (client *Client) ResetAll() error {
	request, err := http.NewRequest(http.MethodPost, client.BaseURL+"/api/reset", nil)
	if err != nil {
		return err
	}
	response, err := client.HTTPClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s", strings.TrimSpace(string(body)))
	}
	return nil
}

// Logs returns the recent log text for a profile.
func (client *Client) Logs(name string) (string, error) {
	response, err := client.HTTPClient.Get(client.BaseURL + "/api/logs?name=" + url.QueryEscape(name))
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s", strings.TrimSpace(string(body)))
	}
	return string(body), nil
}

func (client *Client) postAction(action, name string, headers map[string]string) error {
	endpoint := client.BaseURL + "/api/action?action=" + url.QueryEscape(action) + "&name=" + url.QueryEscape(name)
	request, err := http.NewRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	for key, value := range headers {
		if value != "" {
			request.Header.Set(key, value)
		}
	}
	response, err := client.HTTPClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = response.Status
		}
		return fmt.Errorf("%s", message)
	}
	return nil
}

func (client *Client) getJSON(path string, target any) error {
	response, err := client.HTTPClient.Get(client.BaseURL + path)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s", strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, target)
}
