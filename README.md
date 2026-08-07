<p align="center">
  <img src="assets/vpntoris-logo.png" width="180" alt="VPNToris logo">
</p>

<h1 align="center">VPNToris</h1>

<p align="center">
  Multi-profile split-route corporate VPN client for macOS, Linux and Windows.
</p>

**VPNToris 2.0.0-alpha** moves off a Docker runtime. VPN engines, the privileged helper and the local controller ship inside the platform installer. Split routes stay profile-scoped; the default internet path is not replaced.

## Status

This repository currently publishes **2.0.0-alpha** builds.

| Platform | Installer | Notes |
|---|---|---|
| macOS | Complete signed/notarized PKG | App + native engines |
| Linux | DEB / RPM | Controller, helper, tray + engines |
| Windows | MSI | Controller, helper, tray + OpenVPN/Wintun engines |

Alpha quality: expect API and packaging changes before a stable 2.0.0.

## Features

- Multiple concurrent VPN profiles with independent CIDR split routes
- FortiGate SSL (`openfortivpn`), OpenVPN, OpenConnect and IPsec (platform support varies)
- Local authenticated controller on `127.0.0.1:17984`
- Privileged native helper (macOS LaunchDaemon / Linux systemd / Windows service)
- Engines resolved from the install tree with manifest digests (not from `PATH`)
- OTP/2FA flow when the gateway challenges after connect
- macOS SwiftUI menu bar app; Linux/Windows tray + CLI
- Credentials via platform stores (Keychain / libsecret path / Windows Credential Manager)

## Requirements

### macOS

- Apple Silicon or Intel Mac
- macOS 13 Ventura or later
- Administrator rights for the privileged helper install

### Linux

- amd64 or arm64
- systemd, iproute2
- Optional tray dialog helpers (`zenity` / `kdialog` / `whiptail`)

### Windows

- Windows 10/11 x64
- Administrator rights for the native helper service

## Install

Download the platform package from [Releases](https://github.com/maliyilmaz0/vpntoris/releases) (pre-release **2.0.0-alpha**).

- **macOS:** open the complete PKG (`*-universal-complete.pkg`)
- **Linux:** install the DEB or RPM for your architecture
- **Windows:** install the MSI

## Development (source)

```bash
cd vpntoris-tray
go test ./...
```

Maintainer packaging (local only; signing secrets stay out of the tree):

```bash
./build.sh 2.0.0-alpha darwin
./build.sh 2.0.0-alpha linux
./build.sh 2.0.0-alpha windows
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
