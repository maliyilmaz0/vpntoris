package main

import (
	"fmt"
	"sort"
	"strings"
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
	// lastStatus avoids rewriting the disabled status item every tick.
	lastStatus string
}

type profileMenu struct {
	root       *systray.MenuItem
	connect    *systray.MenuItem
	disconnect *systray.MenuItem
	otp        *systray.MenuItem
	edit       *systray.MenuItem
	delete     *systray.MenuItem
	logs       *systray.MenuItem
	// lastTitle/state avoid SetTitle/Enable on every poll — menu churn closes
	// open AppIndicator menus on GNOME when the mouse is still over them.
	lastTitle      string
	lastConnected  bool
	lastNeedsOTP   bool
	stateKnown     bool
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
	addProfile := systray.AddMenuItem("Add Profile…", "Create a new VPN profile")
	refresh := systray.AddMenuItem("Refresh", "Reload profiles from the controller")
	reset := systray.AddMenuItem("Reset All Connections", "Stop every VPNToris session")
	systray.AddSeparator()
	openDir := systray.AddMenuItem("Open Config Folder", "Open the local VPNToris config directory")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "Quit VPNToris tray")

	go app.loop(addProfile, refresh, reset, openDir, quit)
}

func (app *trayApp) onExit() {}

func (app *trayApp) loop(addProfile, refresh, reset, openDir, quit *systray.MenuItem) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	app.refreshProfiles()
	for {
		select {
		case <-addProfile.ClickedCh:
			app.handleAddProfile()
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
		case <-openDir.ClickedCh:
			openConfigDir()
		case <-quit.ClickedCh:
			systray.Quit()
			return
		case <-ticker.C:
			// Never rebuild the menu while a dialog is open or the user is
			// interacting — GNOME AppIndicator closes the popup on property updates.
			if dialogBusy.Load() {
				continue
			}
			app.refreshProfiles()
		}
	}
}

func (app *trayApp) setStatus(message string) {
	if app.status == nil {
		return
	}
	// Truncate long errors so the menu stays readable; never include secrets here.
	if len(message) > 80 {
		message = message[:77] + "..."
	}
	title := "Status: " + message
	if title == app.lastStatus {
		return
	}
	app.lastStatus = title
	app.status.SetTitle(title)
}

func (app *trayApp) refreshProfiles() {
	if dialogBusy.Load() {
		return
	}
	profiles, err := app.client.Profiles()
	if err != nil {
		app.setStatus("controller offline")
		return
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	seen := map[string]bool{}
	connected := 0
	for _, profile := range profiles {
		seen[profile.Name] = true
		if profile.Connected {
			connected++
		}
		app.ensureProfileMenu(profile)
		app.updateProfileMenu(profile)
	}
	app.mu.Lock()
	for name, item := range app.items {
		if !seen[name] {
			item.root.Hide()
		} else {
			item.root.Show()
		}
	}
	app.mu.Unlock()
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
		edit:       root.AddSubMenuItem("Edit Profile…", "Edit "+profile.Name),
		delete:     root.AddSubMenuItem("Delete Profile…", "Delete "+profile.Name),
		logs:       root.AddSubMenuItem("View Logs…", "Show recent logs for "+profile.Name),
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
	go func() {
		for range item.edit.ClickedCh {
			app.handleEditProfile(name)
		}
	}()
	go func() {
		for range item.delete.ClickedCh {
			app.handleDeleteProfile(name)
		}
	}()
	go func() {
		for range item.logs.ClickedCh {
			app.handleLogs(name)
		}
	}()
}

func (app *trayApp) updateProfileMenu(profile trayclient.Profile) {
	// refreshProfiles runs on several goroutines (ticker loop + click
	// handlers); serialize menu state mutations against each other and
	// against handleDeleteProfile.
	app.mu.Lock()
	defer app.mu.Unlock()
	item := app.items[profile.Name]
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
	// Only push D-Bus property updates when something actually changed.
	if !item.stateKnown || item.lastTitle != title {
		item.lastTitle = title
		item.root.SetTitle(title)
	}
	if !item.stateKnown || item.lastConnected != profile.Connected {
		item.lastConnected = profile.Connected
		if profile.Connected {
			item.connect.Disable()
			item.disconnect.Enable()
		} else {
			item.connect.Enable()
			item.disconnect.Disable()
		}
	}
	if !item.stateKnown || item.lastNeedsOTP != profile.NeedsOTP {
		item.lastNeedsOTP = profile.NeedsOTP
		if profile.NeedsOTP {
			item.otp.Enable()
		} else {
			item.otp.Disable()
		}
	}
	item.stateKnown = true
}

func (app *trayApp) handleAddProfile() {
	config, _, err := editProfileForm(nil)
	if err != nil {
		if err.Error() != "cancelled" && err.Error() != "profile form cancelled" {
			app.setStatus("add failed: " + err.Error())
		}
		return
	}
	app.persistAndSave("", config)
}

func (app *trayApp) handleEditProfile(name string) {
	existing, err := app.client.ProfileConfig(name)
	if err != nil {
		app.setStatus(name + ": " + err.Error())
		return
	}
	config, replace, err := editProfileForm(&existing)
	if err != nil {
		if err.Error() != "cancelled" && err.Error() != "profile form cancelled" {
			app.setStatus("edit failed: " + err.Error())
		}
		return
	}
	if replace == "" {
		replace = name
	}
	app.persistAndSave(replace, config)
}

func (app *trayApp) persistAndSave(replace string, config trayclient.ProfileConfig) {
	password := config.Password
	psk := ""
	if config.IPSec != nil {
		psk = config.IPSec.PreSharedKey
	}
	// Keep existing secrets when the form left them blank.
	if password == "" {
		if stored, err := app.store.Read(firstNonEmpty(replace, config.Name), "password"); err == nil {
			password = stored
		}
	}
	if psk == "" && config.Type == "ipsec" {
		if stored, err := app.store.Read(firstNonEmpty(replace, config.Name), "psk"); err == nil {
			psk = stored
		}
	}
	// Controller strips secrets from disk; we keep them only in the credential store.
	toSave := config
	toSave.Password = password // sent once; controller drops before write
	if toSave.Type == "ipsec" {
		if toSave.IPSec == nil {
			toSave.IPSec = trayclient.DefaultIPSec()
		}
		toSave.IPSec.PreSharedKey = psk
	}
	if err := app.client.SaveProfile(toSave, replace); err != nil {
		app.setStatus("save failed: " + err.Error())
		return
	}
	if replace != "" && replace != config.Name {
		_ = app.store.Delete(replace)
	}
	_ = app.store.Write(config.Name, "password", password)
	if config.Type == "ipsec" {
		_ = app.store.Write(config.Name, "psk", psk)
	}
	app.setStatus(config.Name + ": saved")
	app.refreshProfiles()
}

func (app *trayApp) handleDeleteProfile(name string) {
	if !promptConfirm("VPNToris", "Delete profile “"+name+"”? This cannot be undone.") {
		app.setStatus(name + ": delete cancelled")
		return
	}
	if err := app.client.DeleteProfile(name); err != nil {
		app.setStatus(name + ": delete failed: " + err.Error())
		showTextDialog("VPNToris", "Could not delete profile “"+name+"”:\n\n"+err.Error())
		return
	}
	_ = app.store.Delete(name)
	app.mu.Lock()
	if item, ok := app.items[name]; ok {
		// Some AppIndicator hosts ignore Hide(); disable + retitle as fallback.
		item.root.SetTitle("(deleted) " + name)
		item.root.Disable()
		item.connect.Disable()
		item.disconnect.Disable()
		item.otp.Disable()
		item.edit.Disable()
		item.delete.Disable()
		item.logs.Disable()
		item.root.Hide()
		delete(app.items, name)
	}
	app.mu.Unlock()
	app.setStatus(name + ": deleted")
	// Force a full refresh so the list matches disk (and re-hides ghosts).
	app.refreshProfiles()
}

func (app *trayApp) handleLogs(name string) {
	// Fetch logs first (no dialog), then open a durable window after the menu closes.
	logs, err := app.client.Logs(name)
	if err != nil {
		app.setStatus(name + ": " + err.Error())
		showTextDialog("VPNToris", "Could not load logs for “"+name+"”:\n\n"+err.Error())
		return
	}
	if strings.TrimSpace(logs) == "" {
		logs = "(no log output yet)"
	}
	showTextDialog("VPNToris — "+name+" logs", logs)
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
