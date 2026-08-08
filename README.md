<p align="center">
  <img src="assets/vpntoris-logo.png" width="180" alt="VPNToris logo">
</p>

<h1 align="center">VPNToris</h1>

<p align="center">
  Multi-profile split-route corporate VPN client for macOS, Linux and Windows.
</p>

**VPNToris 2.1.0** runs fully native: VPN engines, the privileged helper and the local controller ship inside the platform installer — no Docker runtime. Split routes stay profile-scoped; the default internet path is not replaced.

## Status

This repository currently publishes **2.1.0** builds.

| Platform | Installer | Notes |
|---|---|---|
| macOS | Complete signed/notarized PKG | App + native engines |
| Linux | DEB / RPM | Controller, helper, tray + engines; bundled AppIndicator extension for GNOME |
| Windows | MSI | Controller, helper, tray + OpenVPN/Wintun engines |

## Test status

| Platform | Status | Details |
|---|---|---|
| macOS | Tested | Tray UI, FortiGate SSL, IPsec (strongSwan, XAuth OTP), OpenConnect and OpenVPN verified end-to-end |
| Linux | Partially tested | AlmaLinux 9.7: tray and FortiGate SSL VPN verified; IPsec, OpenConnect and OpenVPN not yet tested |
| Windows | Not tested | Builds and packages, but no end-to-end testing has been done |

## Features

- Multiple concurrent VPN profiles with independent CIDR split routes
- FortiGate SSL (`openfortivpn`), OpenVPN, OpenConnect and IPsec (platform support varies)
- Local authenticated controller on `127.0.0.1:17984`
- Privileged native helper (macOS LaunchDaemon / Linux systemd / Windows service)
- Engines resolved from the install tree with manifest digests (not from `PATH`)
- OTP/2FA flow when the gateway challenges after connect
- macOS SwiftUI menu bar app; Linux/Windows tray + CLI
- Credentials via platform stores (Keychain / Secret Service via libsecret with a 0600 file fallback / Windows Credential Manager)

## Requirements

### macOS

- Apple Silicon or Intel Mac
- macOS 13 Ventura or later
- Administrator rights for the privileged helper install

### Linux

- amd64 or arm64
- systemd, iproute2, ppp (resolved automatically from base repos)
- GNOME: a private AppIndicator shell extension is bundled and enabled by default on next login
- Optional: `libsecret` (`secret-tool`) for keyring-backed credential storage; dialogs use `zenity` / `kdialog` / `whiptail`

### Windows

- Windows 10/11 x64
- Administrator rights for the native helper service

## Install

Download the platform package from [Releases](https://github.com/maliyilmaz0/vpntoris/releases) (**2.1.0**).

- **macOS:** open the complete PKG (`*-universal-complete.pkg`)
- **Linux:** install the DEB or RPM for your architecture
- **Windows:** install the MSI

## Development (source)

Design and operational notes live in [`docs/`](docs/): architecture, security
model, packaging and development conventions.

```bash
cd vpntoris-tray
go test ./...
```

Maintainer packaging (local only; signing secrets stay out of the tree):

```bash
./build.sh 2.1.0 darwin
./build.sh 2.1.0 linux
./build.sh 2.1.0 windows
```

Output layout:

```text
versions/<ver>/
  macos/
  linux/
  windows/
```

## Safety contract

- No default route unless an explicit full-tunnel mode is enabled later
- Only profile CIDRs are installed as destinations
- Mutations are journaled before apply; failures roll back
- Recovery removes only resources whose ownership can be verified
- Secrets are never written to logs, process arguments or profile JSON

## License

See repository license terms for application code. Bundled VPN engines carry their own open-source licenses (GPL/LGPL/OpenSSL exception as applicable) and are redistributed with the product package.
