package main

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"fyne.io/systray"

	"vpntoris-tray/internal/credentials"
	"vpntoris-tray/internal/trayclient"
)

type trayApp struct {
	client *trayclient.Client
	store  credentials.Store
	mu     sync.Mutex
	items  map[string]*profileMenu
	status *systray.MenuItem
}

type profileMenu struct {
	root       *systray.MenuItem
	connect    *systray.MenuItem
	disconnect *systray.MenuItem
	otp        *systray.MenuItem
}

func newTrayApp() *trayApp {
	return &trayApp{
		client: trayclient.New(),
		store:  credentials.New(),
		items:  map[string]*profileMenu{},
	}
}

func (app *trayApp) onReady() {
	systray.SetIcon(minimalPNG)
	systray.SetTitle("VPNToris")
	systray.SetTooltip("VPNToris")
	app.status = systray.AddMenuItem("Status: starting…", "Controller status")
	app.status.Disable()
	systray.AddSeparator()
	refresh := systray.AddMenuItem("Refresh", "Reload profiles from the controller")
	reset := systray.AddMenuItem("Reset All Connections", "Stop every VPNToris session")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "Quit VPNToris tray")

	go app.loop(refresh, reset, quit)
}

func (app *trayApp) onExit() {}

func (app *trayApp) loop(refresh, reset, quit *systray.MenuItem) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	app.refreshProfiles()
	for {
		select {
		case <-refresh.ClickedCh:
			app.refreshProfiles()
		case <-reset.ClickedCh:
			if promptConfirm("VPNToris", "Reset all VPNToris connections? Profiles are kept.") {
				if err := app.client.ResetAll(); err != nil {
					app.setStatus("reset failed: " + err.Error())
				} else {
					app.setStatus("all connections reset")
					app.refreshProfiles()
				}
			}
		case <-quit.ClickedCh:
			systray.Quit()
			return
		case <-ticker.C:
			app.refreshProfiles()
		}
	}
}

func (app *trayApp) setStatus(message string) {
	if app.status != nil {
		// Truncate long errors so the menu stays readable; never include secrets here.
		if len(message) > 80 {
			message = message[:77] + "..."
		}
		app.status.SetTitle("Status: " + message)
	}
}

func (app *trayApp) refreshProfiles() {
	profiles, err := app.client.Profiles()
	if err != nil {
		app.setStatus("controller offline")
		return
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	connected := 0
	for _, profile := range profiles {
		if profile.Connected {
			connected++
		}
		app.ensureProfileMenu(profile)
		app.updateProfileMenu(profile)
	}
	app.setStatus(fmt.Sprintf("%d profile(s), %d connected", len(profiles), connected))
}

func (app *trayApp) ensureProfileMenu(profile trayclient.Profile) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if _, ok := app.items[profile.Name]; ok {
		return
	}
	root := systray.AddMenuItem(profile.Name, profile.Host)
	item := &profileMenu{
		root:       root,
		connect:    root.AddSubMenuItem("Connect", "Connect "+profile.Name),
		disconnect: root.AddSubMenuItem("Disconnect", "Disconnect "+profile.Name),
		otp:        root.AddSubMenuItem("Submit OTP…", "Submit one-time password for "+profile.Name),
	}
	app.items[profile.Name] = item
	name := profile.Name
	go func() {
		for range item.connect.ClickedCh {
			app.handleConnect(name)
		}
	}()
	go func() {
		for range item.disconnect.ClickedCh {
			if err := app.client.Disconnect(name); err != nil {
				app.setStatus(name + ": " + err.Error())
			} else {
				app.setStatus(name + ": disconnected")
			}
			app.refreshProfiles()
		}
	}()
	go func() {
		for range item.otp.ClickedCh {
			app.handleOTP(name)
		}
	}()
}

func (app *trayApp) updateProfileMenu(profile trayclient.Profile) {
	app.mu.Lock()
	item := app.items[profile.Name]
	app.mu.Unlock()
	if item == nil {
		return
	}
	state := "disconnected"
	if profile.Connected {
		state = "connected"
	}
	if profile.NeedsOTP {
		state = "needs OTP"
	}
	title := fmt.Sprintf("%s [%s]", profile.Name, state)
	if profile.RouteStatus != "" {
		title += " (" + profile.RouteStatus + ")"
	}
	item.root.SetTitle(title)
	if profile.Connected {
		item.connect.Disable()
		item.disconnect.Enable()
	} else {
		item.connect.Enable()
		item.disconnect.Disable()
	}
	if profile.NeedsOTP {
		item.otp.Enable()
	} else {
		item.otp.Disable()
	}
}

func (app *trayApp) handleConnect(name string) {
	password, _ := app.store.Read(name, "password")
	psk, _ := app.store.Read(name, "psk")
	if password == "" {
		value, err := promptSecret("VPNToris", "Password for "+name)
		if err != nil {
			app.setStatus(name + ": password required")
			return
		}
		password = value
		_ = app.store.Write(name, "password", password)
	}
	if err := app.client.Connect(name, password, psk); err != nil {
		app.setStatus(name + ": " + err.Error())
		return
	}
	app.setStatus(name + ": connecting")
	app.refreshProfiles()
}

func (app *trayApp) handleOTP(name string) {
	otp, err := promptSecret("VPNToris OTP", "One-time password for "+name)
	if err != nil || otp == "" {
		app.setStatus(name + ": OTP cancelled")
		return
	}
	if err := app.client.SubmitOTP(name, otp); err != nil {
		app.setStatus(name + ": " + err.Error())
		return
	}
	app.setStatus(name + ": OTP submitted")
	app.refreshProfiles()
}
