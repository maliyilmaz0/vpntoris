package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"vpntoris-tray/internal/trayclient"
)

func editProfileForm(existing *trayclient.ProfileConfig) (trayclient.ProfileConfig, string, error) {
	var (
		config  trayclient.ProfileConfig
		replace string
		err     error
	)
	withDialog(func() {
		base := trayclient.ProfileConfig{
			Type:                "openfortivpn",
			Port:                "443",
			AutoReconnect:       true,
			FailoverLimit:       2,
			OpenConnectProtocol: "anyconnect",
		}
		replace = ""
		if existing != nil {
			base = *existing
			replace = existing.Name
			base.Password = ""
			if base.IPSec != nil {
				base.IPSec.PreSharedKey = ""
			}
		}
		if runtime.GOOS == "linux" {
			cfg, formErr := gtkProfileForm(base)
			if formErr == nil {
				config, err = cfg, nil
				return
			}
			if formErr.Error() == "profile form cancelled" || formErr.Error() == "cancelled" {
				err = fmt.Errorf("profile form cancelled")
				return
			}
			if formErr.Error() != "gtk not available" && !strings.Contains(formErr.Error(), "executable file not found") {
				err = formErr
				return
			}
		}
		config, replace, err = wizardProfileForm(base, replace)
	})
	return config, replace, err
}
func wizardProfileForm(base trayclient.ProfileConfig, replace string) (trayclient.ProfileConfig, string, error) {
	name, err := promptEntry("VPNToris", "Profile name", base.Name)
	if err != nil || strings.TrimSpace(name) == "" {
		return trayclient.ProfileConfig{}, "", fmt.Errorf("cancelled")
	}
	vpnType, err := promptList("VPNToris", "VPN type", []string{
		"openfortivpn", "ipsec", "openconnect", "openvpn",
	}, firstNonEmpty(base.Type, "openfortivpn"))
	if err != nil {
		return trayclient.ProfileConfig{}, "", fmt.Errorf("cancelled")
	}
	host, err := promptEntry("VPNToris", "Host / gateway", base.Host)
	if err != nil || strings.TrimSpace(host) == "" {
		return trayclient.ProfileConfig{}, "", fmt.Errorf("cancelled")
	}
	port, err := promptEntry("VPNToris", "Port", firstNonEmpty(base.Port, "443"))
	if err != nil {
		return trayclient.ProfileConfig{}, "", fmt.Errorf("cancelled")
	}
	user, err := promptEntry("VPNToris", "Username", base.User)
	if err != nil {
		return trayclient.ProfileConfig{}, "", fmt.Errorf("cancelled")
	}
	password, err := promptSecret("VPNToris", "Password (leave empty to keep existing)")
	if err != nil {
		password = ""
	}
	routes, err := promptEntry("VPNToris", "VPN routes (comma/space separated CIDRs)", base.Routes)
	if err != nil {
		return trayclient.ProfileConfig{}, "", fmt.Errorf("cancelled")
	}
	domains, err := promptEntry("VPNToris", "Split DNS domains (e.g. corp.local)", base.Domains)
	if err != nil {
		domains = base.Domains
	}
	dnsServers, err := promptEntry("VPNToris", "VPN DNS servers (e.g. 10.38.1.10)", base.DNSServers)
	if err != nil {
		dnsServers = base.DNSServers
	}
	twoFactor := promptConfirm("VPNToris", "Ask for 2FA / OTP when connecting?")
	autoReconnect := true
	if base.Name != "" {
		autoReconnect = base.AutoReconnect
	}
	if !promptConfirm("VPNToris", "Reconnect automatically if the tunnel drops?") {
		autoReconnect = false
	} else {
		autoReconnect = true
	}
	config := trayclient.ProfileConfig{
		Name:                strings.TrimSpace(name),
		Description:         base.Description,
		Type:                vpnType,
		Host:                strings.TrimSpace(host),
		Port:                strings.TrimSpace(port),
		User:                strings.TrimSpace(user),
		Password:            password,
		TwoFactor:           twoFactor,
		AutoReconnect:       autoReconnect,
		ConnectOnLaunch:     base.ConnectOnLaunch,
		Routes:              strings.TrimSpace(routes),
		Domains:             strings.TrimSpace(domains),
		DNSServers:          strings.TrimSpace(dnsServers),
		BackupGateways:      base.BackupGateways,
		FailoverLimit:       base.FailoverLimit,
		Config:              base.Config,
		OpenConnectProtocol: firstNonEmpty(base.OpenConnectProtocol, "anyconnect"),
		ExternalBrowser:     base.ExternalBrowser,
		IPSec:               base.IPSec,
	}
	if config.FailoverLimit <= 0 {
		config.FailoverLimit = 2
	}
	switch config.Type {
	case "openconnect":
		proto, err := promptList("VPNToris", "OpenConnect gateway protocol", []string{
			"anyconnect", "gp", "pulse", "nc", "f5", "fortinet", "array",
		}, firstNonEmpty(config.OpenConnectProtocol, "anyconnect"))
		if err == nil {
			config.OpenConnectProtocol = proto
		}
		config.ExternalBrowser = promptConfirm("VPNToris", "Use browser-based SAML / SSO authentication?")
	case "openvpn":
		if path, err := promptFile("VPNToris", "Select OpenVPN .ovpn / .conf file"); err == nil && path != "" {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return trayclient.ProfileConfig{}, "", readErr
			}
			config.Config = string(data)
		} else if strings.TrimSpace(config.Config) == "" {
			text, _ := promptEntry("VPNToris", "Paste OpenVPN config (or leave empty)", "")
			config.Config = text
		}
	case "ipsec":
		if config.IPSec == nil {
			config.IPSec = trayclient.DefaultIPSec()
		}
		psk, err := promptSecret("VPNToris", "IPsec pre-shared key (leave empty to keep existing)")
		if err == nil {
			config.IPSec.PreSharedKey = psk
		}
	}
	return config, replace, nil
}
func gtkProfileForm(base trayclient.ProfileConfig) (trayclient.ProfileConfig, error) {
	payload, err := json.Marshal(base)
	if err != nil {
		return trayclient.ProfileConfig{}, err
	}
	script := `#!/usr/bin/env python3
import json, sys
try:
    import gi
    gi.require_version("Gtk", "3.0")
    from gi.repository import Gtk, GLib
except Exception:
    sys.exit(127)

raw = sys.stdin.read()
try:
    base = json.loads(raw) if raw.strip() else {}
except Exception:
    base = {}

TYPE_IDS = ["openfortivpn", "ipsec", "openconnect", "openvpn"]
TYPE_LABELS = {
    "openfortivpn": "FortiGate SSL VPN",
    "ipsec": "FortiGate IPsec",
    "openconnect": "GlobalProtect / OpenConnect",
    "openvpn": "OpenVPN",
}
OC_IDS = ["anyconnect", "gp", "pulse", "nc", "f5", "fortinet", "array"]
OC_LABELS = {
    "anyconnect": "Cisco AnyConnect",
    "gp": "Palo Alto GlobalProtect",
    "pulse": "Pulse / Ivanti",
    "nc": "Juniper Network Connect",
    "f5": "F5 BIG-IP",
    "fortinet": "Fortinet SSL VPN",
    "array": "Array Networks",
}

class Form(Gtk.Dialog):
    def __init__(self):
        Gtk.Dialog.__init__(self, title="VPNToris Profile", flags=0)
        self.add_buttons(Gtk.STOCK_CANCEL, Gtk.ResponseType.CANCEL, Gtk.STOCK_SAVE, Gtk.ResponseType.OK)
        self.set_default_size(560, 640)
        self.set_border_width(8)
        outer = self.get_content_area()
        outer.set_spacing(8)

        title = Gtk.Label()
        title.set_markup("<b>Profile</b>")
        title.set_xalign(0)
        outer.pack_start(title, False, False, 0)

        scroll = Gtk.ScrolledWindow()
        scroll.set_policy(Gtk.PolicyType.NEVER, Gtk.PolicyType.AUTOMATIC)
        scroll.set_min_content_height(520)
        outer.pack_start(scroll, True, True, 0)

        self.grid = Gtk.Grid(column_spacing=12, row_spacing=8, margin=6)
        scroll.add(self.grid)

        self.fields = {}
        self.labels = {}
        self.rows = {}          # key -> (label_widget, value_widget) for show/hide
        self.type_groups = {   # which keys belong to which VPN type (None = always)
            None: ["name", "type", "host", "port", "backupGateways", "failoverThreshold",
                   "user", "password", "routes", "domains", "dnsServers", "description",
                   "two_factor", "auto_reconnect", "connect_on_launch"],
            "openconnect": ["openConnectProtocol", "external_browser"],
            "openvpn": ["config", "config_status"],
            "ipsec": ["psk", "ikeVersion", "authMode", "localID", "remoteID"],
        }
        self.row = 0

        self._entry("name", "Profile name", base.get("name", ""))

        # VPN type picker (human labels, internal ids)
        self._label("type", "VPN type")
        type_combo = Gtk.ComboBoxText()
        initial_type = base.get("type") or "openfortivpn"
        for tid in TYPE_IDS:
            type_combo.append(tid, TYPE_LABELS[tid])
        try:
            type_combo.set_active(TYPE_IDS.index(initial_type))
        except ValueError:
            type_combo.set_active(0)
        type_combo.connect("changed", self._on_type_changed)
        self.fields["type"] = type_combo
        self._attach("type", type_combo)

        # OpenConnect-only
        self._label("openConnectProtocol", "Gateway protocol")
        oc = Gtk.ComboBoxText()
        for oid in OC_IDS:
            oc.append(oid, OC_LABELS[oid])
        oc_val = base.get("openConnectProtocol") or "anyconnect"
        try:
            oc.set_active(OC_IDS.index(oc_val))
        except ValueError:
            oc.set_active(0)
        self.fields["openConnectProtocol"] = oc
        self._attach("openConnectProtocol", oc)

        self.external_browser = Gtk.CheckButton(label="Use browser-based SAML / SSO authentication")
        self.external_browser.set_active(bool(base.get("externalBrowser")))
        self.fields["external_browser"] = self.external_browser
        self._attach_span("external_browser", self.external_browser)

        # Common connection fields
        self._entry("host", "Host", base.get("host", ""))
        self._entry("port", "Port", base.get("port") or "443")
        self._entry("backupGateways", "Backup gateways (one per line)", base.get("backupGateways", ""))
        self._entry("failoverThreshold", "Switch gateway after N failed reconnects",
                    str(base.get("failoverThreshold") or 2))
        self._entry("user", "Username", base.get("user", ""))
        self._secret("password", "Password", "leave empty to keep existing")

        self.two_factor = Gtk.CheckButton(label="Ask for 2FA / OTP when connecting")
        self.two_factor.set_active(bool(base.get("twoFactor")))
        self.fields["two_factor"] = self.two_factor
        self._attach_span("two_factor", self.two_factor)

        self.auto_reconnect = Gtk.CheckButton(label="Reconnect automatically if the tunnel drops")
        self.auto_reconnect.set_active(bool(base.get("autoReconnect", True)))
        self.fields["auto_reconnect"] = self.auto_reconnect
        self._attach_span("auto_reconnect", self.auto_reconnect)

        self.connect_on_launch = Gtk.CheckButton(label="Connect when VPNToris opens")
        self.connect_on_launch.set_active(bool(base.get("connectOnLaunch")))
        self.fields["connect_on_launch"] = self.connect_on_launch
        self._attach_span("connect_on_launch", self.connect_on_launch)
        self.two_factor.connect("toggled", self._on_two_factor)

        self._entry("routes", "VPN routes (10.68.0.0/16, …)", base.get("routes", ""))
        self._entry("domains", "Split DNS domains (e.g. corp.local)", base.get("domains", ""))
        self._entry("dnsServers", "VPN DNS servers (e.g. 10.38.1.10)", base.get("dnsServers", ""))
        self._entry("description", "Description", base.get("description", ""))

        # OpenVPN-only
        self._label("config", "OpenVPN config file")
        cfg_box = Gtk.Box(spacing=6)
        cfg_entry = Gtk.Entry()
        cfg_entry.set_hexpand(True)
        cfg_entry.set_placeholder_text("Choose an .ovpn / .conf file…")
        cfg_btn = Gtk.Button(label="Choose File…")
        def browse(_btn):
            dlg = Gtk.FileChooserDialog(
                title="OpenVPN configuration", parent=self,
                action=Gtk.FileChooserAction.OPEN)
            dlg.add_buttons(Gtk.STOCK_CANCEL, Gtk.ResponseType.CANCEL,
                            Gtk.STOCK_OPEN, Gtk.ResponseType.OK)
            filt = Gtk.FileFilter()
            filt.set_name("OpenVPN")
            filt.add_pattern("*.ovpn")
            filt.add_pattern("*.conf")
            dlg.add_filter(filt)
            if dlg.run() == Gtk.ResponseType.OK:
                path = dlg.get_filename() or ""
                cfg_entry.set_text(path)
                self._update_config_status(path)
            dlg.destroy()
        cfg_btn.connect("clicked", browse)
        cfg_box.pack_start(cfg_entry, True, True, 0)
        cfg_box.pack_start(cfg_btn, False, False, 0)
        self.fields["config"] = cfg_entry
        self._attach("config", cfg_box)

        existing_cfg = base.get("config") or ""
        status_text = ("OpenVPN configuration loaded · %d bytes" % len(existing_cfg.encode("utf-8"))
                       if existing_cfg else "The remote host, port and complete configuration will be imported.")
        self.config_status = Gtk.Label(label=status_text)
        self.config_status.set_xalign(0)
        self.config_status.set_line_wrap(True)
        self.fields["config_status"] = self.config_status
        self._attach_span("config_status", self.config_status)
        self._loaded_config = existing_cfg

        # IPsec-only (core fields; full phase1/2 lives on macOS advanced panel)
        self._secret("psk", "Pre-shared key", "leave empty to keep existing")

        self._label("ikeVersion", "IKE version")
        ike = Gtk.ComboBoxText()
        ike.append("1", "Version 1")
        ike.append("2", "Version 2")
        ipsec = base.get("ipsec") or {}
        if not isinstance(ipsec, dict):
            ipsec = {}
        ike_ver = str(ipsec.get("ikeVersion") or 2)
        ike.set_active(0 if ike_ver == "1" else 1)
        self.fields["ikeVersion"] = ike
        self._attach("ikeVersion", ike)

        self._label("authMode", "Extended authentication")
        auth = Gtk.ComboBoxText()
        for aid, alab in (("none", "None"), ("xauth", "XAuth"), ("eap", "EAP")):
            auth.append(aid, alab)
        auth_val = ipsec.get("authMode") or "eap"
        try:
            auth.set_active(["none", "xauth", "eap"].index(auth_val))
        except ValueError:
            auth.set_active(2)
        self.fields["authMode"] = auth
        self._attach("authMode", auth)

        self._entry("localID", "Local ID", ipsec.get("localID", ""))
        self._entry("remoteID", "Remote ID", ipsec.get("remoteID", ""))

        self._apply_type_visibility(initial_type)
        self._on_two_factor(self.two_factor)
        self.show_all()
        # Re-apply after show_all (which reveals everything)
        self._apply_type_visibility(initial_type)

    def _label(self, key, text):
        lab = Gtk.Label(label=text, xalign=0)
        self.labels[key] = lab

    def _entry(self, key, label, value):
        self._label(key, label)
        w = Gtk.Entry()
        w.set_text(str(value or ""))
        w.set_hexpand(True)
        self.fields[key] = w
        self._attach(key, w)

    def _secret(self, key, label, placeholder):
        self._label(key, label)
        w = Gtk.Entry()
        w.set_visibility(False)
        w.set_placeholder_text(placeholder)
        w.set_hexpand(True)
        self.fields[key] = w
        self._attach(key, w)

    def _attach(self, key, widget):
        lab = self.labels.get(key)
        r = self.row
        self.row += 1
        if lab is not None:
            self.grid.attach(lab, 0, r, 1, 1)
            self.grid.attach(widget, 1, r, 1, 1)
            self.rows[key] = (lab, widget)
        else:
            self.grid.attach(widget, 0, r, 2, 1)
            self.rows[key] = (None, widget)

    def _attach_span(self, key, widget):
        r = self.row
        self.row += 1
        self.grid.attach(widget, 0, r, 2, 1)
        self.rows[key] = (None, widget)

    def _set_row_visible(self, key, visible):
        pair = self.rows.get(key)
        if not pair:
            return
        lab, widget = pair
        if lab is not None:
            lab.set_visible(visible)
            lab.set_no_show_all(not visible)
        widget.set_visible(visible)
        widget.set_no_show_all(not visible)

    def _current_type(self):
        w = self.fields.get("type")
        if w is None:
            return "openfortivpn"
        tid = w.get_active_id()
        if tid:
            return tid
        # fallback for older gtk
        text = w.get_active_text() or ""
        for k, v in TYPE_LABELS.items():
            if v == text:
                return k
        return "openfortivpn"

    def _on_type_changed(self, _combo):
        self._apply_type_visibility(self._current_type())

    def _apply_type_visibility(self, vpn_type):
        # Hide all type-specific first, then show matching group + always-visible.
        type_specific = set()
        for group, keys in self.type_groups.items():
            if group is not None:
                type_specific.update(keys)
        for key in type_specific:
            self._set_row_visible(key, False)
        for key in self.type_groups.get(None, []):
            self._set_row_visible(key, True)
        for key in self.type_groups.get(vpn_type, []):
            self._set_row_visible(key, True)
        # Connect-on-launch disabled when 2FA is on (macOS parity)
        self._on_two_factor(self.two_factor)

    def _on_two_factor(self, btn):
        disabled = btn.get_active()
        self.connect_on_launch.set_sensitive(not disabled)
        if disabled:
            self.connect_on_launch.set_active(False)

    def _update_config_status(self, path):
        if not path:
            return
        try:
            with open(path, "r", encoding="utf-8", errors="replace") as f:
                data = f.read()
            self._loaded_config = data
            self.config_status.set_text(
                "OpenVPN configuration loaded · %d bytes" % len(data.encode("utf-8")))
        except Exception as e:
            self.config_status.set_text("Could not read file: %s" % e)

    def _text(self, key):
        w = self.fields.get(key)
        if w is None:
            return ""
        if isinstance(w, Gtk.ComboBoxText):
            tid = w.get_active_id()
            if tid is not None:
                return tid
            return w.get_active_text() or ""
        if isinstance(w, Gtk.Entry):
            return w.get_text().strip()
        return ""

    def result(self):
        vpn_type = self._current_type()
        try:
            failover = int(self._text("failoverThreshold") or "2")
        except ValueError:
            failover = 2
        if failover < 1:
            failover = 1
        cfg = {
            "name": self._text("name"),
            "type": vpn_type,
            "host": self._text("host"),
            "port": self._text("port") or "443",
            "user": self._text("user"),
            "password": self._text("password"),
            "routes": self._text("routes"),
            "domains": self._text("domains"),
            "dnsServers": self._text("dnsServers"),
            "backupGateways": self._text("backupGateways"),
            "description": self._text("description"),
            "twoFactor": self.two_factor.get_active(),
            "autoReconnect": self.auto_reconnect.get_active(),
            "connectOnLaunch": self.connect_on_launch.get_active() and not self.two_factor.get_active(),
            "failoverThreshold": failover,
            "config": "",
            "openConnectProtocol": "",
            "externalBrowser": False,
        }
        if vpn_type == "openconnect":
            cfg["openConnectProtocol"] = self._text("openConnectProtocol") or "anyconnect"
            cfg["externalBrowser"] = self.external_browser.get_active()
        elif vpn_type == "openvpn":
            path = self._text("config")
            if path:
                try:
                    with open(path, "r", encoding="utf-8", errors="replace") as f:
                        cfg["config"] = f.read()
                except Exception as e:
                    cfg["configError"] = str(e)
            else:
                cfg["config"] = getattr(self, "_loaded_config", "") or (base.get("config") or "")
        elif vpn_type == "ipsec":
            ipsec = base.get("ipsec") or {}
            if not isinstance(ipsec, dict):
                ipsec = {}
            defaults = {
                "ikeVersion": 2, "ikeMode": "main", "authMode": "eap",
                "modeConfig": True, "natTraversal": True, "fragmentation": "yes",
                "dpdAction": "restart", "dpdDelay": 30, "dpdTimeout": 150,
                "ikeLifetime": 28800,
                "ikeEncryption": "aes256,aes128,aes256gcm16,aes128gcm16,chacha20poly1305",
                "ikeIntegrity": "sha256,sha384,sha512",
                "ikePRF": "prfsha256,prfsha384,prfsha512",
                "dhGroups": "14,19,20,21,31",
                "childLifetime": 3600,
                "espEncryption": "aes256,aes128,aes256gcm16,aes128gcm16,chacha20poly1305",
                "espIntegrity": "sha256,sha384,sha512",
                "pfsGroups": "14,19,20,21,31",
                "replayWindow": 32,
            }
            for k, v in defaults.items():
                ipsec.setdefault(k, v)
            try:
                ipsec["ikeVersion"] = int(self._text("ikeVersion") or "2")
            except ValueError:
                ipsec["ikeVersion"] = 2
            ipsec["authMode"] = self._text("authMode") or "eap"
            if ipsec["ikeVersion"] == 1 and ipsec["authMode"] == "eap":
                ipsec["authMode"] = "xauth"
            ipsec["localID"] = self._text("localID")
            ipsec["remoteID"] = self._text("remoteID")
            psk = self._text("psk")
            if psk:
                ipsec["preSharedKey"] = psk
            cfg["ipsec"] = ipsec
        return cfg

GLib.set_prgname("vpntoris-tray")
try:
    form = Form()
except Exception:
    sys.exit(127)
response = form.run()
if response != Gtk.ResponseType.OK:
    form.destroy()
    sys.exit(1)
out = form.result()
form.destroy()
if not out.get("name") or not out.get("host"):
    sys.exit(2)
if out.get("configError"):
    print(out["configError"], file=sys.stderr)
    sys.exit(3)
print(json.dumps(out))
`
	tmp, err := os.CreateTemp("", "vpntoris-profile-*.py")
	if err != nil {
		return trayclient.ProfileConfig{}, err
	}
	path := tmp.Name()
	defer os.Remove(path)
	if _, err := tmp.WriteString(script); err != nil {
		tmp.Close()
		return trayclient.ProfileConfig{}, err
	}
	tmp.Close()
	_ = os.Chmod(path, 0700)
	cmd := exec.Command("python3", path)
	cmd.Stdin = strings.NewReader(string(payload))
	cmd.Env = os.Environ()
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		cmd.Env = append(cmd.Env, "DISPLAY=:0")
	}
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			switch exitErr.ExitCode() {
			case 1, 2:
				return trayclient.ProfileConfig{}, fmt.Errorf("profile form cancelled")
			case 3:
				errMsg := strings.TrimSpace(string(exitErr.Stderr))
				if errMsg == "" {
					errMsg = "invalid config file"
				}
				return trayclient.ProfileConfig{}, fmt.Errorf("%s", errMsg)
			case 127:
				return trayclient.ProfileConfig{}, fmt.Errorf("gtk not available")
			default:
				return trayclient.ProfileConfig{}, fmt.Errorf("gtk profile form failed: %w", err)
			}
		}
		return trayclient.ProfileConfig{}, fmt.Errorf("gtk not available")
	}
	var config trayclient.ProfileConfig
	if err := json.Unmarshal(out, &config); err != nil {
		return trayclient.ProfileConfig{}, err
	}
	if config.Type == "ipsec" && config.IPSec == nil {
		config.IPSec = trayclient.DefaultIPSec()
	}
	if config.Type == "openvpn" && strings.TrimSpace(config.Config) == "" && strings.TrimSpace(base.Config) != "" {
		config.Config = base.Config
	}
	return config, nil
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
