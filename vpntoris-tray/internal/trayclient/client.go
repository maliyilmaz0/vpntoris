package trayclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}
type Profile struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Type          string `json:"type"`
	Protocol      string `json:"protocol"`
	Host          string `json:"host"`
	ActiveHost    string `json:"activeGateway"`
	Routes        string `json:"routes"`
	Connected     bool   `json:"connected"`
	TwoFactor     bool   `json:"twoFactor"`
	AutoReconnect bool   `json:"autoReconnect"`
	NeedsOTP      bool   `json:"needsOtp"`
	RouteStatus   string `json:"routeStatus"`
}
type IPSecConfig struct {
	IKEVersion      int    `json:"ikeVersion"`
	IKEMode         string `json:"ikeMode"`
	AuthMode        string `json:"authMode"`
	PreSharedKey    string `json:"preSharedKey"`
	LocalID         string `json:"localID"`
	RemoteID        string `json:"remoteID"`
	ModeConfig      bool   `json:"modeConfig"`
	NATTraversal    bool   `json:"natTraversal"`
	ForceEncap      bool   `json:"forceEncap"`
	MOBIKE          bool   `json:"mobike"`
	Fragmentation   string `json:"fragmentation"`
	DPDAction       string `json:"dpdAction"`
	DPDDelay        int    `json:"dpdDelay"`
	DPDTimeout      int    `json:"dpdTimeout"`
	IKELifetime     int    `json:"ikeLifetime"`
	IKEEncryption   string `json:"ikeEncryption"`
	IKEIntegrity    string `json:"ikeIntegrity"`
	IKEPRF          string `json:"ikePRF"`
	DHGroups        string `json:"dhGroups"`
	ChildLifetime   int    `json:"childLifetime"`
	ChildLifetimeKB int    `json:"childLifetimeKB"`
	ESPEncryption   string `json:"espEncryption"`
	ESPIntegrity    string `json:"espIntegrity"`
	PFS             bool   `json:"pfs"`
	PFSGroups       string `json:"pfsGroups"`
	ReplayWindow    int    `json:"replayWindow"`
	LocalSelectors  string `json:"localSelectors"`
	RemoteSelectors string `json:"remoteSelectors"`
}
type ProfileConfig struct {
	Name                string       `json:"name"`
	Description         string       `json:"description"`
	Type                string       `json:"type"`
	Host                string       `json:"host"`
	BackupGateways      string       `json:"backupGateways"`
	FailoverLimit       int          `json:"failoverThreshold"`
	Port                string       `json:"port"`
	User                string       `json:"user"`
	Password            string       `json:"password"`
	TwoFactor           bool         `json:"twoFactor"`
	AutoReconnect       bool         `json:"autoReconnect"`
	ConnectOnLaunch     bool         `json:"connectOnLaunch"`
	Routes              string       `json:"routes"`
	Domains             string       `json:"domains"`
	DNSServers          string       `json:"dnsServers"`
	Config              string       `json:"config"`
	OpenConnectProtocol string       `json:"openConnectProtocol,omitempty"`
	ExternalBrowser     bool         `json:"externalBrowser,omitempty"`
	IPSec               *IPSecConfig `json:"ipsec,omitempty"`
}

func DefaultIPSec() *IPSecConfig {
	return &IPSecConfig{
		IKEVersion:    2,
		IKEMode:       "main",
		AuthMode:      "eap",
		ModeConfig:    true,
		NATTraversal:  true,
		Fragmentation: "yes",
		DPDAction:     "restart",
		DPDDelay:      30,
		DPDTimeout:    150,
		IKELifetime:   28800,
		IKEEncryption: "aes256,aes128,aes256gcm16,aes128gcm16,chacha20poly1305",
		IKEIntegrity:  "sha256,sha384,sha512",
		IKEPRF:        "prfsha256,prfsha384,prfsha512",
		DHGroups:      "14,19,20,21,31",
		ChildLifetime: 3600,
		ESPEncryption: "aes256,aes128,aes256gcm16,aes128gcm16,chacha20poly1305",
		ESPIntegrity:  "sha256,sha384,sha512",
		PFSGroups:     "14,19,20,21,31",
		ReplayWindow:  32,
	}
}
func New() *Client {
	return &Client{
		BaseURL: "http://127.0.0.1:17984",
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}
func (client *Client) Profiles() ([]Profile, error) {
	var profiles []Profile
	if err := client.getJSON("/api/profiles", &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}
func (client *Client) ProfileConfig(name string) (ProfileConfig, error) {
	var config ProfileConfig
	if err := client.getJSON("/api/profiles?name="+url.QueryEscape(name), &config); err != nil {
		return ProfileConfig{}, err
	}
	return config, nil
}
func (client *Client) SaveProfile(config ProfileConfig, replace string) error {
	path := "/api/profiles"
	if replace != "" && replace != config.Name {
		path += "?replace=" + url.QueryEscape(replace)
	}
	body, err := json.Marshal(config)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, client.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.HTTPClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusCreated {
		message := strings.TrimSpace(string(payload))
		if message == "" {
			message = response.Status
		}
		return fmt.Errorf("%s", message)
	}
	return nil
}
func (client *Client) DeleteProfile(name string) error {
	request, err := http.NewRequest(http.MethodDelete, client.BaseURL+"/api/profiles?name="+url.QueryEscape(name), nil)
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
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = response.Status
		}
		return fmt.Errorf("%s", message)
	}
	return nil
}
func (client *Client) Connect(name, password, psk string) error {
	return client.postAction("connect", name, map[string]string{
		"X-VPNToris-Password": password,
		"X-VPNToris-PSK":      psk,
	})
}
func (client *Client) Disconnect(name string) error {
	return client.postAction("disconnect", name, nil)
}
func (client *Client) SubmitOTP(name, otp string) error {
	return client.postAction("otp", name, map[string]string{
		"X-VPNToris-OTP": otp,
	})
}
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
