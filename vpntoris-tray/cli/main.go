package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const api = "http://127.0.0.1:17984"

type profile struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Host      string `json:"host"`
	Connected bool   `json:"connected"`
}
type flow struct {
	Profile string `json:"profile"`
	Process string `json:"process"`
	PID     int    `json:"pid"`
	Remote  string `json:"remote"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	command := os.Args[1]
	switch command {
	case "status", "profiles":
		var profiles []profile
		getJSON("/api/profiles", &profiles)
		for _, item := range profiles {
			state := "disconnected"
			if item.Connected {
				state = "connected"
			}
			fmt.Printf("%-24s %-14s %-12s %s\n", item.Name, state, item.Type, item.Host)
		}
	case "flows":
		var flows []flow
		getJSON("/api/flows", &flows)
		for _, item := range flows {
			fmt.Printf("%-18s pid=%-7d %-22s %s\n", item.Process, item.PID, item.Remote, item.Profile)
		}
	case "routes":
		var routes any
		getJSON("/api/routes", &routes)
		printJSON(routes)
	case "connect", "disconnect", "route":
		requireName()
		action(command, strings.Join(os.Args[2:], " "))
	case "logs":
		requireName()
		name := strings.Join(os.Args[2:], " ")
		response, err := http.Get(api + "/api/logs?name=" + url.QueryEscape(name))
		check(err)
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		check(err)
		if response.StatusCode != 200 {
			fatal(strings.TrimSpace(string(data)))
		}
		fmt.Print(string(data))
	case "check-route":
		requireName()
		var result any
		getJSON("/api/route-check?target="+url.QueryEscape(os.Args[2]), &result)
		printJSON(result)
	default:
		usage()
	}
}
func action(action, name string) {
	request, err := http.NewRequest(http.MethodPost, api+"/api/action?action="+url.QueryEscape(action)+"&name="+url.QueryEscape(name), nil)
	check(err)
	if action == "connect" {
		request.Header.Set("X-VPNToris-Password", os.Getenv("VPNTORIS_PASSWORD"))
		request.Header.Set("X-VPNToris-PSK", os.Getenv("VPNTORIS_PSK"))
	}
	response, err := http.DefaultClient.Do(request)
	check(err)
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusNoContent {
		fatal(strings.TrimSpace(string(data)))
	}
	fmt.Println("ok")
}
func getJSON(path string, target any) {
	response, err := http.Get(api + path)
	check(err)
	defer response.Body.Close()
	if response.StatusCode != 200 {
		data, _ := io.ReadAll(response.Body)
		fatal(strings.TrimSpace(string(data)))
	}
	check(json.NewDecoder(response.Body).Decode(target))
}
func printJSON(value any) { data, _ := json.MarshalIndent(value, "", "  "); fmt.Println(string(data)) }
func requireName() {
	if len(os.Args) < 3 {
		fatal("profile name or target is required")
	}
}
func check(err error) {
	if err != nil {
		fatal(err.Error())
	}
}
func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
func usage() {
	fmt.Fprintln(os.Stderr, "usage: vpntorisctl status|profiles|flows|routes|connect <profile>|disconnect <profile>|route <profile>|logs <profile>|check-route <ip>")
	os.Exit(2)
}
