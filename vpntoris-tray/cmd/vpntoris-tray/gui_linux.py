#!/usr/bin/env python3

import json
import os
import sys
import time
import urllib.request
import urllib.parse
import urllib.error
import subprocess
import threading
import socket

import gi
gi.require_version("Gtk", "3.0")
gi.require_version("Gdk", "3.0")
from gi.repository import Gtk, Gdk, GLib, Pango

API_BASE = "http://127.0.0.1:17984"
VERSION = "2.1.4"

TYPE_LABELS = {
    "openfortivpn": "FortiGate SSL VPN",
    "ipsec": "FortiGate IPsec",
    "openconnect": "GlobalProtect / OpenConnect",
    "openvpn": "OpenVPN",
}

OC_LABELS = {
    "anyconnect": "Cisco AnyConnect",
    "gp": "Palo Alto GlobalProtect",
    "pulse": "Pulse / Ivanti",
    "nc": "Juniper Network Connect",
    "f5": "F5 BIG-IP",
    "fortinet": "Fortinet SSL VPN",
    "array": "Array Networks",
}

CSS = b"""
window, dialog {
    background-color: #1a1a1e;
    color: #e4e4e7;
}

.header-box {
    background-color: #222227;
    border-bottom: 1px solid #2e2e36;
    padding: 10px 14px;
}

.header-title {
    font-size: 16px;
    font-weight: bold;
    color: #ffffff;
}

.search-entry {
    background-color: #27272e;
    color: #ffffff;
    border: 1px solid #3f3f46;
    border-radius: 8px;
    padding: 4px 8px;
}

.profile-card {
    background-color: #24242b;
    border: 1px solid #33333d;
    border-radius: 12px;
    margin: 6px 10px;
    padding: 12px 14px;
}

.profile-card:hover {
    border-color: #4a4a58;
}

.profile-title {
    font-size: 15px;
    font-weight: 700;
    color: #ffffff;
}

.profile-subtitle {
    font-size: 11px;
    color: #a1a1aa;
}

.routes-label {
    font-size: 11px;
    color: #38bdf8;
    font-family: monospace;
}

.traffic-text {
    font-size: 11px;
    color: #71717a;
    font-family: monospace;
}

.traffic-up {
    color: #60a5fa;
    font-weight: bold;
    font-family: monospace;
}

.traffic-down {
    color: #4ade80;
    font-weight: bold;
    font-family: monospace;
}

.btn-connect {
    background-color: #15803d;
    color: #ffffff;
    font-weight: bold;
    border-radius: 6px;
    padding: 4px 14px;
    border: none;
}
.btn-connect:hover {
    background-color: #16a34a;
}

.btn-disconnect {
    background-color: #b91c1c;
    color: #ffffff;
    font-weight: bold;
    border-radius: 6px;
    padding: 4px 14px;
    border: none;
}
.btn-disconnect:hover {
    background-color: #dc2626;
}

.btn-action {
    background: transparent;
    color: #a1a1aa;
    border: none;
    padding: 2px 8px;
    font-size: 12px;
}
.btn-action:hover {
    color: #ffffff;
    background-color: rgba(255, 255, 255, 0.08);
    border-radius: 4px;
}

.btn-primary {
    background-color: #2563eb;
    color: #ffffff;
    font-weight: bold;
    border-radius: 6px;
    padding: 6px 16px;
    border: none;
}
.btn-primary:hover {
    background-color: #3b82f6;
}

.otp-box {
    background-color: rgba(234, 88, 12, 0.12);
    border: 1px solid rgba(234, 88, 12, 0.35);
    border-radius: 8px;
    padding: 8px 10px;
    margin-top: 6px;
}

.status-bar {
    background-color: #18181c;
    border-top: 1px solid #27272a;
    padding: 8px 14px;
    font-size: 11px;
    color: #71717a;
}

.status-dot {
    color: #22c55e;
    font-size: 10px;
}

.dot-connected {
    color: #22c55e;
}

.dot-disconnected {
    color: #71717a;
}

.routes-active {
    color: #22c55e;
}

.routes-waiting {
    color: #f59e0b;
}

.routes-failed {
    color: #ef4444;
}
"""

def format_bytes(count):
    if count < 1024:
        return f"{count} B"
    elif count < 1024 * 1024:
        return f"{count / 1024:.1f} KB"
    elif count < 1024 * 1024 * 1024:
        return f"{count / (1024 * 1024):.1f} MB"
    else:
        return f"{count / (1024 * 1024 * 1024):.2f} GB"

def format_rate(bps):
    return format_bytes(int(bps)) + "/s"

def format_duration(seconds):
    h = seconds // 3600
    m = (seconds % 3600) // 60
    s = seconds % 60
    if h > 0:
        return f"{h:02d}:{m:02d}:{s:02d}"
    return f"{m:02d}:{s:02d}"


class CredentialsStore:
    @staticmethod
    def _config_file():
        cfg_dir = os.environ.get("XDG_CONFIG_HOME") or os.path.expanduser("~/.config")
        path = os.path.join(cfg_dir, "VPNToris")
        os.makedirs(path, exist_ok=True)
        return os.path.join(path, "credentials.json")

    @classmethod
    def read(cls, profile, field):
        try:
            res = subprocess.run(["secret-tool", "lookup", "service", "vpntoris", "profile", profile, "field", field],
                                 stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=2)
            if res.returncode == 0 and res.stdout.strip():
                return res.stdout.decode("utf-8").strip()
        except Exception:
            pass

        try:
            with open(cls._config_file(), "r", encoding="utf-8") as f:
                data = json.load(f)
            return data.get(f"{profile}/{field}", "")
        except Exception:
            return ""

    @classmethod
    def write(cls, profile, field, value):
        try:
            if value:
                p = subprocess.Popen(["secret-tool", "store", f"--label=VPNToris {profile} {field}",
                                     "service", "vpntoris", "profile", profile, "field", field],
                                     stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                p.communicate(input=value.encode("utf-8"), timeout=2)
            else:
                subprocess.run(["secret-tool", "clear", "service", "vpntoris", "profile", profile, "field", field],
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=2)
        except Exception:
            pass

        try:
            path = cls._config_file()
            data = {}
            if os.path.exists(path):
                with open(path, "r", encoding="utf-8") as f:
                    data = json.load(f)
            key = f"{profile}/{field}"
            if value:
                data[key] = value
            else:
                data.pop(key, None)
            with open(path + ".tmp", "w", encoding="utf-8") as f:
                json.dump(data, f, indent=2)
            os.replace(path + ".tmp", path)
            os.chmod(path, 0o600)
        except Exception:
            pass

    @classmethod
    def delete(cls, profile):
        try:
            subprocess.run(["secret-tool", "clear", "service", "vpntoris", "profile", profile],
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=2)
        except Exception:
            pass
        try:
            path = cls._config_file()
            if os.path.exists(path):
                with open(path, "r", encoding="utf-8") as f:
                    data = json.load(f)
                keys_to_del = [k for k in data if k.startswith(profile + "/") or k == profile]
                for k in keys_to_del:
                    data.pop(k, None)
                with open(path + ".tmp", "w", encoding="utf-8") as f:
                    json.dump(data, f, indent=2)
                os.replace(path + ".tmp", path)
                os.chmod(path, 0o600)
        except Exception:
            pass


class APIClient:
    @staticmethod
    def get_profiles():
        req = urllib.request.Request(f"{API_BASE}/api/profiles")
        with urllib.request.urlopen(req, timeout=5) as resp:
            return json.loads(resp.read().decode("utf-8"))

    @staticmethod
    def get_profile_config(name):
        q = urllib.parse.urlencode({"name": name})
        req = urllib.request.Request(f"{API_BASE}/api/profiles?{q}")
        with urllib.request.urlopen(req, timeout=5) as resp:
            return json.loads(resp.read().decode("utf-8"))

    @staticmethod
    def save_profile(config, replace=""):
        url = f"{API_BASE}/api/profiles"
        if replace and replace != config.get("name"):
            url += "?" + urllib.parse.urlencode({"replace": replace})
        body = json.dumps(config).encode("utf-8")
        req = urllib.request.Request(url, data=body, headers={"Content-Type": "application/json"}, method="POST")
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status in (200, 201, 204)

    @staticmethod
    def delete_profile(name):
        q = urllib.parse.urlencode({"name": name})
        req = urllib.request.Request(f"{API_BASE}/api/profiles?{q}", method="DELETE")
        with urllib.request.urlopen(req, timeout=5) as resp:
            return resp.status in (200, 204)

    @staticmethod
    def connect(name, password="", psk=""):
        q = urllib.parse.urlencode({"action": "connect", "name": name})
        headers = {}
        if password:
            headers["X-VPNToris-Password"] = password
        if psk:
            headers["X-VPNToris-PSK"] = psk
        req = urllib.request.Request(f"{API_BASE}/api/action?{q}", headers=headers, method="POST")
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status in (200, 204)

    @staticmethod
    def disconnect(name):
        q = urllib.parse.urlencode({"action": "disconnect", "name": name})
        req = urllib.request.Request(f"{API_BASE}/api/action?{q}", method="POST")
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status in (200, 204)

    @staticmethod
    def submit_otp(name, otp):
        q = urllib.parse.urlencode({"action": "otp", "name": name})
        req = urllib.request.Request(f"{API_BASE}/api/action?{q}", headers={"X-VPNToris-OTP": otp}, method="POST")
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status in (200, 204)

    @staticmethod
    def reapply_routes(name):
        q = urllib.parse.urlencode({"action": "route", "name": name})
        req = urllib.request.Request(f"{API_BASE}/api/action?{q}", method="POST")
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status in (200, 204)

    @staticmethod
    def reset_all():
        req = urllib.request.Request(f"{API_BASE}/api/reset", method="POST")
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status in (200, 204)

    @staticmethod
    def get_traffic():
        try:
            req = urllib.request.Request(f"{API_BASE}/api/traffic")
            with urllib.request.urlopen(req, timeout=2) as resp:
                items = json.loads(resp.read().decode("utf-8"))
                return {item["name"]: item for item in items}
        except Exception:
            return {}

    @staticmethod
    def get_logs(name):
        q = urllib.parse.urlencode({"name": name})
        req = urllib.request.Request(f"{API_BASE}/api/logs?{q}")
        with urllib.request.urlopen(req, timeout=5) as resp:
            return resp.read().decode("utf-8", errors="replace")


class LogDialog(Gtk.Dialog):
    def __init__(self, parent, profile_name):
        super().__init__(title=f"VPNToris — {profile_name} Logs", parent=parent, flags=0)
        self.profile_name = profile_name
        self.set_default_size(720, 480)
        self.add_button(Gtk.STOCK_COPY, Gtk.ResponseType.APPLY)
        self.add_button(Gtk.STOCK_CLOSE, Gtk.ResponseType.CLOSE)

        box = self.get_content_area()
        box.set_spacing(8)
        box.set_border_width(10)

        self.search_entry = Gtk.SearchEntry()
        self.search_entry.set_placeholder_text("Filter log lines…")
        self.search_entry.connect("search-changed", self._on_search)
        box.pack_start(self.search_entry, False, False, 0)

        scroll = Gtk.ScrolledWindow()
        scroll.set_policy(Gtk.PolicyType.AUTOMATIC, Gtk.PolicyType.AUTOMATIC)
        box.pack_start(scroll, True, True, 0)

        self.text_view = Gtk.TextView()
        self.text_view.set_editable(False)
        self.text_view.set_cursor_visible(False)
        self.text_view.set_wrap_mode(Gtk.WrapMode.CHAR)
        try:
            self.text_view.override_font(Pango.FontDescription.from_string("Monospace 10"))
        except Exception:
            pass
        scroll.add(self.text_view)

        self.raw_logs = ""
        self._load_logs()
        self.show_all()

    def _load_logs(self):
        try:
            self.raw_logs = APIClient.get_logs(self.profile_name)
        except Exception as e:
            self.raw_logs = f"Could not load logs: {e}"
        self._apply_filter()

    def _on_search(self, entry):
        self._apply_filter()

    def _apply_filter(self):
        query = self.search_entry.get_text().strip().lower()
        if not query:
            display = self.raw_logs or "(no log output yet)"
        else:
            lines = [l for l in self.raw_logs.splitlines() if query in l.lower()]
            display = "\n".join(lines) if lines else "(no matching log lines)"
        buf = self.text_view.get_buffer()
        buf.set_text(display)


class ProfileDialog(Gtk.Dialog):
    def __init__(self, parent, existing=None):
        title = "Edit VPN Profile" if existing else "Add VPN Profile"
        super().__init__(title=title, parent=parent, flags=0)
        self.existing = existing or {}
        self.add_button(Gtk.STOCK_CANCEL, Gtk.ResponseType.CANCEL)
        self.add_button(Gtk.STOCK_SAVE, Gtk.ResponseType.OK)
        self.set_default_size(540, 620)
        self.set_border_width(10)

        outer = self.get_content_area()
        outer.set_spacing(10)

        scroll = Gtk.ScrolledWindow()
        scroll.set_policy(Gtk.PolicyType.NEVER, Gtk.PolicyType.AUTOMATIC)
        scroll.set_min_content_height(480)
        outer.pack_start(scroll, True, True, 0)

        self.grid = Gtk.Grid(column_spacing=12, row_spacing=8, margin=8)
        scroll.add(self.grid)

        self.fields = {}
        self.labels = {}
        self.rows = {}
        self.type_groups = {
            None: ["name", "type", "host", "port", "backupGateways", "failoverThreshold",
                   "user", "password", "routes", "domains", "dnsServers", "description",
                   "two_factor", "auto_reconnect", "connect_on_launch"],
            "openconnect": ["openConnectProtocol", "external_browser"],
            "openvpn": ["config"],
            "ipsec": ["psk", "ikeVersion", "authMode", "localID", "remoteID"],
        }
        self.row = 0

        self._build_form()
        self.show_all()
        self._apply_type_visibility(self._current_type())

    def _build_form(self):
        base = self.existing
        self._entry("name", "Profile name", base.get("name", ""))

        self._label("type", "VPN type")
        type_combo = Gtk.ComboBoxText()
        for tid in ["openfortivpn", "ipsec", "openconnect", "openvpn"]:
            type_combo.append(tid, TYPE_LABELS[tid])
        init_type = base.get("type") or "openfortivpn"
        type_combo.set_active(["openfortivpn", "ipsec", "openconnect", "openvpn"].index(init_type) if init_type in TYPE_LABELS else 0)
        type_combo.connect("changed", lambda _: self._apply_type_visibility(self._current_type()))
        self.fields["type"] = type_combo
        self._attach("type", type_combo)

        self._label("openConnectProtocol", "Gateway protocol")
        oc = Gtk.ComboBoxText()
        for oid in ["anyconnect", "gp", "pulse", "nc", "f5", "fortinet", "array"]:
            oc.append(oid, OC_LABELS[oid])
        oc_val = base.get("openConnectProtocol") or "anyconnect"
        try:
            oc.set_active(["anyconnect", "gp", "pulse", "nc", "f5", "fortinet", "array"].index(oc_val))
        except ValueError:
            oc.set_active(0)
        self.fields["openConnectProtocol"] = oc
        self._attach("openConnectProtocol", oc)

        self.external_browser = Gtk.CheckButton(label="Use browser-based SAML / SSO authentication")
        self.external_browser.set_active(bool(base.get("externalBrowser")))
        self.fields["external_browser"] = self.external_browser
        self._attach_span("external_browser", self.external_browser)

        self._entry("host", "Host / Gateway", base.get("host", ""))
        self._entry("port", "Port", base.get("port") or "443")
        self._entry("backupGateways", "Backup gateways", base.get("backupGateways", ""))
        self._entry("failoverThreshold", "Failover threshold", str(base.get("failoverThreshold") or 2))
        self._entry("user", "Username", base.get("user", ""))
        self._secret("password", "Password", "Leave empty to keep existing password")

        self.two_factor = Gtk.CheckButton(label="Ask for 2FA / OTP when connecting")
        self.two_factor.set_active(bool(base.get("twoFactor")))
        self.fields["two_factor"] = self.two_factor
        self._attach_span("two_factor", self.two_factor)

        self.auto_reconnect = Gtk.CheckButton(label="Reconnect automatically if tunnel drops")
        self.auto_reconnect.set_active(bool(base.get("autoReconnect", True)))
        self.fields["auto_reconnect"] = self.auto_reconnect
        self._attach_span("auto_reconnect", self.auto_reconnect)

        self.connect_on_launch = Gtk.CheckButton(label="Connect when VPNToris opens")
        self.connect_on_launch.set_active(bool(base.get("connectOnLaunch")))
        self.fields["connect_on_launch"] = self.connect_on_launch
        self._attach_span("connect_on_launch", self.connect_on_launch)

        self._entry("routes", "VPN routes (CIDRs)", base.get("routes", ""))
        self._entry("domains", "Split DNS domains", base.get("domains", ""))
        self._entry("dnsServers", "VPN DNS servers", base.get("dnsServers", ""))
        self._entry("description", "Description", base.get("description", ""))

        self._label("config", "OpenVPN config")
        cfg_box = Gtk.Box(spacing=6)
        cfg_entry = Gtk.Entry()
        cfg_entry.set_hexpand(True)
        cfg_entry.set_placeholder_text("Choose .ovpn / .conf file…")
        cfg_btn = Gtk.Button(label="Browse…")
        def browse(_):
            dlg = Gtk.FileChooserDialog(title="Select OpenVPN Config", parent=self, action=Gtk.FileChooserAction.OPEN)
            dlg.add_buttons(Gtk.STOCK_CANCEL, Gtk.ResponseType.CANCEL, Gtk.STOCK_OPEN, Gtk.ResponseType.OK)
            if dlg.run() == Gtk.ResponseType.OK:
                path = dlg.get_filename() or ""
                cfg_entry.set_text(path)
                try:
                    with open(path, "r", encoding="utf-8", errors="replace") as f:
                        self._loaded_ovpn_data = f.read()
                except Exception:
                    pass
            dlg.destroy()
        cfg_btn.connect("clicked", browse)
        cfg_box.pack_start(cfg_entry, True, True, 0)
        cfg_box.pack_start(cfg_btn, False, False, 0)
        self.fields["config"] = cfg_entry
        self._attach("config", cfg_box)
        self._loaded_ovpn_data = base.get("config", "")

        self._secret("psk", "Pre-shared key", "Leave empty to keep existing PSK")
        self._label("ikeVersion", "IKE version")
        ike = Gtk.ComboBoxText()
        ike.append("1", "Version 1")
        ike.append("2", "Version 2")
        ipsec = base.get("ipsec") or {}
        ike.set_active(0 if str(ipsec.get("ikeVersion")) == "1" else 1)
        self.fields["ikeVersion"] = ike
        self._attach("ikeVersion", ike)

        self._label("authMode", "Auth mode")
        auth = Gtk.ComboBoxText()
        for aid, alab in [("eap", "EAP"), ("xauth", "XAuth"), ("none", "None")]:
            auth.append(aid, alab)
        auth_val = ipsec.get("authMode") or "eap"
        try:
            auth.set_active(["eap", "xauth", "none"].index(auth_val))
        except ValueError:
            auth.set_active(0)
        self.fields["authMode"] = auth
        self._attach("authMode", auth)

        self._entry("localID", "Local ID", ipsec.get("localID", ""))
        self._entry("remoteID", "Remote ID", ipsec.get("remoteID", ""))

    def _label(self, key, text):
        self.labels[key] = Gtk.Label(label=text, xalign=0)

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
        return w.get_active_id() if w else "openfortivpn"

    def _apply_type_visibility(self, vpn_type):
        specific = set()
        for g, keys in self.type_groups.items():
            if g is not None:
                specific.update(keys)
        for k in specific:
            self._set_row_visible(k, False)
        for k in self.type_groups.get(None, []):
            self._set_row_visible(k, True)
        for k in self.type_groups.get(vpn_type, []):
            self._set_row_visible(k, True)

    def _text(self, key):
        w = self.fields.get(key)
        if w is None:
            return ""
        if isinstance(w, Gtk.ComboBoxText):
            return w.get_active_id() or w.get_active_text() or ""
        if isinstance(w, Gtk.Entry):
            return w.get_text().strip()
        return ""

    def get_config(self):
        vpn_type = self._current_type()
        try:
            failover = int(self._text("failoverThreshold") or "2")
        except ValueError:
            failover = 2

        cfg = {
            "name": self._text("name"),
            "type": vpn_type,
            "host": self._text("host"),
            "port": self._text("port") or "443",
            "user": self._text("user"),
            "routes": self._text("routes"),
            "domains": self._text("domains"),
            "dnsServers": self._text("dnsServers"),
            "backupGateways": self._text("backupGateways"),
            "description": self._text("description"),
            "twoFactor": self.two_factor.get_active(),
            "autoReconnect": self.auto_reconnect.get_active(),
            "connectOnLaunch": self.connect_on_launch.get_active(),
            "failoverThreshold": max(1, failover),
            "config": "",
        }

        password = self._text("password")
        if password:
            cfg["password"] = password

        if vpn_type == "openconnect":
            cfg["openConnectProtocol"] = self._text("openConnectProtocol") or "anyconnect"
            cfg["externalBrowser"] = self.external_browser.get_active()
        elif vpn_type == "openvpn":
            cfg["config"] = getattr(self, "_loaded_ovpn_data", "")
        elif vpn_type == "ipsec":
            ipsec = self.existing.get("ipsec") or {}
            ipsec["ikeVersion"] = int(self._text("ikeVersion") or "2")
            ipsec["authMode"] = self._text("authMode") or "eap"
            ipsec["localID"] = self._text("localID")
            ipsec["remoteID"] = self._text("remoteID")
            psk = self._text("psk")
            if psk:
                ipsec["preSharedKey"] = psk
            cfg["ipsec"] = ipsec

        return cfg


class PasswordPromptDialog(Gtk.Dialog):
    def __init__(self, parent, profile_name):
        super().__init__(title="VPNToris Authentication", parent=parent, flags=0)
        self.set_default_size(360, 160)
        self.add_button(Gtk.STOCK_CANCEL, Gtk.ResponseType.CANCEL)
        self.add_button(Gtk.STOCK_OK, Gtk.ResponseType.OK)

        box = self.get_content_area()
        box.set_spacing(10)
        box.set_border_width(12)

        lbl = Gtk.Label(label=f"Enter password for <b>{profile_name}</b>:", xalign=0, use_markup=True)
        box.pack_start(lbl, False, False, 0)

        self.entry = Gtk.Entry()
        self.entry.set_visibility(False)
        self.entry.set_activates_default(True)
        self.set_default_response(Gtk.ResponseType.OK)
        box.pack_start(self.entry, False, False, 0)

        self.show_all()


class VPNTorisMainWindow(Gtk.Window):
    def __init__(self):
        super().__init__(title="VPNToris")
        self.set_default_size(430, 640)
        self.set_position(Gtk.WindowPosition.CENTER)
        self.set_icon_name("vpntoris")

        css_provider = Gtk.CssProvider()
        css_provider.load_from_data(CSS)
        Gtk.StyleContext.add_provider_for_screen(
            Gdk.Screen.get_default(), css_provider, Gtk.STYLE_PROVIDER_PRIORITY_APPLICATION
        )

        self.connecting_profiles = set()
        self.otp_inputs = {}
        self.profiles = []
        self.traffic_data = {}

        main_box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=0)
        self.add(main_box)

        header = Gtk.Box(orientation=Gtk.Orientation.HORIZONTAL, spacing=8)
        header.get_style_context().add_class("header-box")

        title_lbl = Gtk.Label(label="VPNToris")
        title_lbl.get_style_context().add_class("header-title")
        header.pack_start(title_lbl, False, False, 4)

        self.search_entry = Gtk.SearchEntry()
        self.search_entry.get_style_context().add_class("search-entry")
        self.search_entry.set_placeholder_text("Search profiles…")
        self.search_entry.set_hexpand(True)
        self.search_entry.connect("search-changed", self._on_search_changed)
        header.pack_start(self.search_entry, True, True, 4)

        add_btn = Gtk.Button(label="+ Add")
        add_btn.get_style_context().add_class("btn-primary")
        add_btn.connect("clicked", self._on_add_profile)
        header.pack_start(add_btn, False, False, 2)

        menu_btn = Gtk.MenuButton()
        menu_btn.set_direction(Gtk.ArrowType.DOWN)
        menu = Gtk.Menu()

        reset_item = Gtk.MenuItem(label="Reset All Connections")
        reset_item.connect("activate", self._on_reset_all)
        menu.append(reset_item)

        cfg_item = Gtk.MenuItem(label="Open Config Folder")
        cfg_item.connect("activate", self._on_open_config)
        menu.append(cfg_item)

        menu.append(Gtk.SeparatorMenuItem())
        quit_item = Gtk.MenuItem(label="Quit VPNToris")
        quit_item.connect("activate", self._on_quit)
        menu.append(quit_item)

        menu.show_all()
        menu_btn.set_popup(menu)
        header.pack_start(menu_btn, False, False, 2)

        main_box.pack_start(header, False, False, 0)

        self.scroll = Gtk.ScrolledWindow()
        self.scroll.set_policy(Gtk.PolicyType.NEVER, Gtk.PolicyType.AUTOMATIC)
        self.cards_box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=4)
        self.cards_box.set_margin_top(6)
        self.cards_box.set_margin_bottom(6)
        self.scroll.add(self.cards_box)
        main_box.pack_start(self.scroll, True, True, 0)

        status_bar = Gtk.Box(orientation=Gtk.Orientation.HORIZONTAL, spacing=8)
        status_bar.get_style_context().add_class("status-bar")

        dot = Gtk.Label(label="●")
        dot.get_style_context().add_class("status-dot")
        status_bar.pack_start(dot, False, False, 0)

        self.engine_status_lbl = Gtk.Label(label="Native engine ready")
        status_bar.pack_start(self.engine_status_lbl, False, False, 0)

        ver_lbl = Gtk.Label(label=f"v{VERSION}")
        ver_lbl.set_sensitive(False)
        status_bar.pack_start(ver_lbl, False, False, 4)

        status_bar.pack_start(Gtk.Box(), True, True, 0)

        quit_btn = Gtk.Button(label="Quit")
        quit_btn.get_style_context().add_class("btn-action")
        quit_btn.connect("clicked", self._on_quit)
        status_bar.pack_end(quit_btn, False, False, 0)

        main_box.pack_start(status_bar, False, False, 0)

        self.connect("delete-event", self._on_delete_event)
        self.show_all()

        GLib.timeout_add(1000, self._poll_update)
        self._refresh_profiles()

    def _on_delete_event(self, _widget, _event):
        self.hide()
        return True

    def _on_quit(self, _=None):
        _cleanup_socket()
        Gtk.main_quit()

    def _poll_update(self):
        threading.Thread(target=self._fetch_data_async, daemon=True).start()
        return True

    def _fetch_data_async(self):
        try:
            profiles = APIClient.get_profiles()
            traffic = APIClient.get_traffic()
            GLib.idle_add(self._apply_data, profiles, traffic)
        except Exception:
            pass

    def _apply_data(self, profiles, traffic):
        self.profiles = profiles
        self.traffic_data = traffic
        self._render_cards()

    def _refresh_profiles(self):
        self._fetch_data_async()

    def _on_search_changed(self, entry):
        self._render_cards()

    def _render_cards(self):
        query = self.search_entry.get_text().strip().lower()

        for child in self.cards_box.get_children():
            self.cards_box.remove(child)

        filtered = []
        for p in self.profiles:
            if not query:
                filtered.append(p)
            else:
                combined = f"{p.get('name','')} {p.get('host','')} {p.get('type','')} {p.get('routes','')} {p.get('description','')}".lower()
                if query in combined:
                    filtered.append(p)

        if not filtered:
            empty_box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=8)
            empty_box.set_valign(Gtk.Align.CENTER)
            empty_box.set_halign(Gtk.Align.CENTER)
            empty_box.set_margin_top(80)

            lbl1 = Gtk.Label()
            lbl1.set_markup("<span size='large' weight='bold' color='#71717a'>No VPN profiles</span>")
            empty_box.pack_start(lbl1, False, False, 0)

            lbl2 = Gtk.Label(label="Click '+ Add' above to create a profile.")
            lbl2.get_style_context().add_class("profile-subtitle")
            empty_box.pack_start(lbl2, False, False, 0)
            self.cards_box.pack_start(empty_box, True, True, 0)
            self.cards_box.show_all()
            return

        for p in filtered:
            card = self._create_profile_card(p)
            self.cards_box.pack_start(card, False, False, 0)

        self.cards_box.show_all()

    def _create_profile_card(self, p):
        name = p.get("name", "")
        connected = p.get("connected", False)
        is_connecting = name in self.connecting_profiles
        needs_otp = p.get("needsOtp", False)
        ptype = p.get("type", "openfortivpn")
        type_str = TYPE_LABELS.get(ptype, ptype.upper())
        if ptype == "openconnect" and p.get("protocol"):
            type_str = OC_LABELS.get(p["protocol"], type_str)

        card = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=6)
        card.get_style_context().add_class("profile-card")

        top_row = Gtk.Box(orientation=Gtk.Orientation.HORIZONTAL, spacing=8)

        icon_box = Gtk.Box(valign=Gtk.Align.CENTER)
        icon_dot = Gtk.Label(label="●")
        icon_dot.get_style_context().add_class("dot-connected" if connected else "dot-disconnected")
        icon_box.pack_start(icon_dot, False, False, 0)
        top_row.pack_start(icon_box, False, False, 2)

        info_box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=2)
        title_lbl = Gtk.Label(label=name, xalign=0)
        title_lbl.get_style_context().add_class("profile-title")
        info_box.pack_start(title_lbl, False, False, 0)

        sub_lbl = Gtk.Label(label=f"{type_str} · {p.get('host','')}", xalign=0)
        sub_lbl.get_style_context().add_class("profile-subtitle")
        info_box.pack_start(sub_lbl, False, False, 0)
        top_row.pack_start(info_box, True, True, 0)

        if is_connecting and not connected:
            conn_box = Gtk.Box(spacing=6)
            spinner = Gtk.Spinner()
            spinner.start()
            conn_box.pack_start(spinner, False, False, 0)

            c_lbl = Gtk.Label(label="Connecting…")
            c_lbl.get_style_context().add_class("profile-subtitle")
            conn_box.pack_start(c_lbl, False, False, 0)

            cancel_btn = Gtk.Button(label="Cancel")
            cancel_btn.get_style_context().add_class("btn-action")
            cancel_btn.connect("clicked", lambda _, n=name: self._on_disconnect(n))
            conn_box.pack_start(cancel_btn, False, False, 0)
            top_row.pack_end(conn_box, False, False, 0)
        elif connected:
            dis_btn = Gtk.Button(label="Disconnect")
            dis_btn.get_style_context().add_class("btn-disconnect")
            dis_btn.connect("clicked", lambda _, n=name: self._on_disconnect(n))
            top_row.pack_end(dis_btn, False, False, 0)
        else:
            con_btn = Gtk.Button(label="Connect")
            con_btn.get_style_context().add_class("btn-connect")
            con_btn.connect("clicked", lambda _, n=name, pdata=p: self._on_connect(n, pdata))
            top_row.pack_end(con_btn, False, False, 0)

        card.pack_start(top_row, False, False, 0)

        routes = p.get("routes", "").strip()
        route_box = Gtk.Box(orientation=Gtk.Orientation.HORIZONTAL, spacing=6)
        r_lbl = Gtk.Label(label=f"🛣  {routes if routes else 'No routes configured'}", xalign=0)
        r_lbl.get_style_context().add_class("routes-label")
        route_box.pack_start(r_lbl, True, True, 0)

        route_status = p.get("routeStatus", "")
        if connected:
            if route_status == "ready":
                s_lbl = Gtk.Label(label="✓ Routes active")
                s_lbl.get_style_context().add_class("routes-active")
                route_box.pack_end(s_lbl, False, False, 0)
            elif route_status == "waiting" or route_status == "adding":
                s_lbl = Gtk.Label(label="Adding routes…")
                s_lbl.get_style_context().add_class("routes-waiting")
                route_box.pack_end(s_lbl, False, False, 0)
            elif route_status == "failed":
                s_lbl = Gtk.Label(label="Routes failed")
                s_lbl.get_style_context().add_class("routes-failed")
                route_box.pack_end(s_lbl, False, False, 0)

        card.pack_start(route_box, False, False, 0)

        if connected and name in self.traffic_data:
            t = self.traffic_data[name]
            traffic_box = Gtk.Box(orientation=Gtk.Orientation.HORIZONTAL, spacing=10)

            down_lbl = Gtk.Label(label=f"↓ {format_rate(t.get('receiveBps', 0))}")
            down_lbl.get_style_context().add_class("traffic-down")
            traffic_box.pack_start(down_lbl, False, False, 0)

            up_lbl = Gtk.Label(label=f"↑ {format_rate(t.get('sendBps', 0))}")
            up_lbl.get_style_context().add_class("traffic-up")
            traffic_box.pack_start(up_lbl, False, False, 0)

            tot_lbl = Gtk.Label(label=f"({format_bytes(t.get('received', 0))} / {format_bytes(t.get('sent', 0))})")
            tot_lbl.get_style_context().add_class("traffic-text")
            traffic_box.pack_start(tot_lbl, False, False, 0)

            dur_lbl = Gtk.Label(label=f"⏱ {format_duration(t.get('duration', 0))}")
            dur_lbl.get_style_context().add_class("traffic-text")
            traffic_box.pack_end(dur_lbl, False, False, 0)

            card.pack_start(traffic_box, False, False, 0)

        if needs_otp and not connected:
            otp_box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=6)
            otp_box.get_style_context().add_class("otp-box")

            otp_prompt = Gtk.Label(label="🔐 Two-factor verification code required:", xalign=0)
            otp_box.pack_start(otp_prompt, False, False, 0)

            otp_input_row = Gtk.Box(spacing=6)
            otp_entry = Gtk.Entry()
            otp_entry.set_placeholder_text("Enter 2FA / OTP code")
            otp_entry.set_hexpand(True)
            otp_entry.set_activates_default(True)

            sub_otp_btn = Gtk.Button(label="Submit OTP")
            sub_otp_btn.get_style_context().add_class("btn-primary")
            sub_otp_btn.connect("clicked", lambda _, n=name, e=otp_entry: self._on_submit_otp(n, e.get_text().strip()))

            cancel_otp_btn = Gtk.Button(label="Cancel")
            cancel_otp_btn.get_style_context().add_class("btn-action")
            cancel_otp_btn.connect("clicked", lambda _, n=name: self._on_disconnect(n))

            otp_input_row.pack_start(otp_entry, True, True, 0)
            otp_input_row.pack_start(sub_otp_btn, False, False, 0)
            otp_input_row.pack_start(cancel_otp_btn, False, False, 0)
            otp_box.pack_start(otp_input_row, False, False, 0)

            card.pack_start(otp_box, False, False, 0)

        btn_row = Gtk.Box(orientation=Gtk.Orientation.HORIZONTAL, spacing=4)

        if connected:
            reapply_btn = Gtk.Button(label="Reapply Routes")
            reapply_btn.get_style_context().add_class("btn-action")
            reapply_btn.connect("clicked", lambda _, n=name: self._on_reapply_routes(n))
            btn_row.pack_start(reapply_btn, False, False, 0)

        logs_btn = Gtk.Button(label="Logs")
        logs_btn.get_style_context().add_class("btn-action")
        logs_btn.connect("clicked", lambda _, n=name: self._on_view_logs(n))
        btn_row.pack_start(logs_btn, False, False, 0)

        edit_btn = Gtk.Button(label="Edit")
        edit_btn.get_style_context().add_class("btn-action")
        edit_btn.connect("clicked", lambda _, n=name: self._on_edit_profile(n))
        btn_row.pack_start(edit_btn, False, False, 0)

        btn_row.pack_start(Gtk.Box(), True, True, 0)

        del_btn = Gtk.Button(label="Delete")
        del_btn.get_style_context().add_class("btn-action")
        del_btn.connect("clicked", lambda _, n=name: self._on_delete_profile(n))
        btn_row.pack_end(del_btn, False, False, 0)

        card.pack_start(btn_row, False, False, 0)
        return card

    def _on_connect(self, name, pdata):
        password = CredentialsStore.read(name, "password")
        psk = CredentialsStore.read(name, "psk")

        if not password:
            dlg = PasswordPromptDialog(self, name)
            if dlg.run() == Gtk.ResponseType.OK:
                password = dlg.entry.get_text().strip()
                CredentialsStore.write(name, "password", password)
                dlg.destroy()
            else:
                dlg.destroy()
                return

        self.connecting_profiles.add(name)
        self._render_cards()

        def do_connect():
            try:
                APIClient.connect(name, password, psk)
            except Exception as e:
                print(f"Connect failed for {name}: {e}", file=sys.stderr)
            finally:
                GLib.idle_add(lambda: self.connecting_profiles.discard(name))
                GLib.idle_add(self._refresh_profiles)

        threading.Thread(target=do_connect, daemon=True).start()

    def _on_disconnect(self, name):
        self.connecting_profiles.discard(name)
        self._render_cards()

        def do_disconnect():
            try:
                APIClient.disconnect(name)
            except Exception as e:
                print(f"Disconnect failed for {name}: {e}", file=sys.stderr)
            finally:
                GLib.idle_add(self._refresh_profiles)

        threading.Thread(target=do_disconnect, daemon=True).start()

    def _on_submit_otp(self, name, otp):
        if not otp:
            return
        def do_otp():
            try:
                APIClient.submit_otp(name, otp)
            except Exception as e:
                print(f"OTP failed for {name}: {e}", file=sys.stderr)
            finally:
                GLib.idle_add(self._refresh_profiles)

        threading.Thread(target=do_otp, daemon=True).start()

    def _on_reapply_routes(self, name):
        def do_routes():
            try:
                APIClient.reapply_routes(name)
            except Exception as e:
                print(f"Reapply routes failed: {e}", file=sys.stderr)
            finally:
                GLib.idle_add(self._refresh_profiles)

        threading.Thread(target=do_routes, daemon=True).start()

    def _on_view_logs(self, name):
        dlg = LogDialog(self, name)
        res = dlg.run()
        if res == Gtk.ResponseType.APPLY:
            clipboard = Gtk.Clipboard.get(Gdk.SELECTION_CLIPBOARD)
            clipboard.set_text(dlg.raw_logs, -1)
        dlg.destroy()

    def _on_add_profile(self, _btn):
        dlg = ProfileDialog(self, None)
        if dlg.run() == Gtk.ResponseType.OK:
            cfg = dlg.get_config()
            if cfg.get("name") and cfg.get("host"):
                pwd = cfg.pop("password", "")
                psk = (cfg.get("ipsec") or {}).pop("preSharedKey", "") if cfg.get("ipsec") else ""
                try:
                    APIClient.save_profile(cfg, "")
                    if pwd:
                        CredentialsStore.write(cfg["name"], "password", pwd)
                    if psk:
                        CredentialsStore.write(cfg["name"], "psk", psk)
                    self._refresh_profiles()
                except Exception as e:
                    self._show_error("Save Profile Failed", str(e))
        dlg.destroy()

    def _on_edit_profile(self, name):
        try:
            existing = APIClient.get_profile_config(name)
        except Exception as e:
            self._show_error("Error Loading Profile", str(e))
            return

        dlg = ProfileDialog(self, existing)
        if dlg.run() == Gtk.ResponseType.OK:
            cfg = dlg.get_config()
            if cfg.get("name") and cfg.get("host"):
                pwd = cfg.pop("password", "")
                psk = (cfg.get("ipsec") or {}).pop("preSharedKey", "") if cfg.get("ipsec") else ""
                try:
                    APIClient.save_profile(cfg, replace=name)
                    if pwd:
                        CredentialsStore.write(cfg["name"], "password", pwd)
                    if psk:
                        CredentialsStore.write(cfg["name"], "psk", psk)
                    self._refresh_profiles()
                except Exception as e:
                    self._show_error("Save Profile Failed", str(e))
        dlg.destroy()

    def _on_delete_profile(self, name):
        dlg = Gtk.MessageDialog(
            parent=self, flags=0, type=Gtk.MessageType.QUESTION,
            buttons=Gtk.ButtonsType.YES_NO, text=f"Delete profile “{name}”?"
        )
        dlg.format_secondary_text("This action cannot be undone.")
        if dlg.run() == Gtk.ResponseType.YES:
            try:
                APIClient.delete_profile(name)
                CredentialsStore.delete(name)
                self._refresh_profiles()
            except Exception as e:
                self._show_error("Delete Failed", str(e))
        dlg.destroy()

    def _on_reset_all(self, _):
        dlg = Gtk.MessageDialog(
            parent=self, flags=0, type=Gtk.MessageType.WARNING,
            buttons=Gtk.ButtonsType.YES_NO, text="Reset all VPNToris connections?"
        )
        dlg.format_secondary_text("All active VPN tunnels will be stopped. Profiles are kept.")
        if dlg.run() == Gtk.ResponseType.YES:
            try:
                APIClient.reset_all()
                self._refresh_profiles()
            except Exception as e:
                self._show_error("Reset Failed", str(e))
        dlg.destroy()

    def _on_open_config(self, _):
        cfg_dir = os.path.expanduser("~/.config/VPNToris")
        os.makedirs(cfg_dir, exist_ok=True)
        subprocess.Popen(["xdg-open", cfg_dir])

    def _show_error(self, title, message):
        dlg = Gtk.MessageDialog(parent=self, flags=0, type=Gtk.MessageType.ERROR,
                                buttons=Gtk.ButtonsType.OK, text=title)
        dlg.format_secondary_text(message)
        dlg.run()
        dlg.destroy()


def _get_socket_path():
    runtime_dir = os.environ.get("XDG_RUNTIME_DIR") or "/tmp"
    return os.path.join(runtime_dir, f"vpntoris-gui-{os.getuid()}.sock")


def _cleanup_socket():
    try:
        sock_path = _get_socket_path()
        if os.path.exists(sock_path):
            os.remove(sock_path)
    except Exception:
        pass


def main():
    sock_path = _get_socket_path()

    try:
        s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        s.connect(sock_path)
        s.sendall(b"show\n")
        s.close()
        sys.exit(0)
    except Exception:
        pass

    GLib.set_prgname("vpntoris-tray")
    win = VPNTorisMainWindow()

    try:
        if os.path.exists(sock_path):
            os.remove(sock_path)
        server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        server.bind(sock_path)
        server.listen(5)

        def listen_loop():
            while True:
                try:
                    conn, _ = server.accept()
                    data = conn.recv(64)
                    conn.close()
                    if b"show" in data:
                        GLib.idle_add(lambda: (win.show_all(), win.present(), False))
                    elif b"quit" in data:
                        GLib.idle_add(Gtk.main_quit)
                except Exception:
                    break

        threading.Thread(target=listen_loop, daemon=True).start()
    except Exception:
        pass

    try:
        Gtk.main()
    finally:
        _cleanup_socket()


if __name__ == "__main__":
    main()
