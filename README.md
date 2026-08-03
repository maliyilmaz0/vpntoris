<p align="center">
  <img src="assets/vpntoris-logo.png" width="180" alt="VPNToris logo">
</p>

<h1 align="center">VPNToris</h1>

<p align="center">
  A native macOS menu bar client for running multiple isolated VPN connections at the same time.
</p>

VPNToris creates one Docker container per VPN profile and routes only the CIDR blocks assigned to that profile. This makes it possible to reach separate private networks simultaneously without replacing the Mac's default internet route.

The user interface is a native SwiftUI tray app. A local Go service manages Docker containers, health checks and profile state; a narrowly scoped privileged helper installs per-destination routes through embedded `tun2socks` instances.

## Features

- Native SwiftUI menu bar interface
- Multiple concurrent VPN connections with independent split routes
- Fortinet SSL VPN (`openfortivpn`), FortiClient-compatible IPsec, OpenConnect and OpenVPN profiles
- IKEv1/IKEv2, Main/Aggressive mode, XAuth/EAP, Mode Config, NAT-T, DPD, Phase 1/2 proposals, PFS and common encryption/DH groups
- Inline OTP flow after negotiation starts, including cancel/retry controls
- Active route and container log views
- Per-profile edit and delete actions
- Embedded route helper and `tun2socks`; no separate system extension package
- Signed and notarized macOS release workflow

## Requirements

- Apple Silicon Mac (`arm64`)
- macOS 13 Ventura or later
- [Docker Desktop for Mac](https://www.docker.com/products/docker-desktop/) running before the first connection
- An administrator account for the one-time route-helper installation
- Network access to the VPN gateway; IPsec commonly requires outbound UDP 500 and 4500
- VPN credentials, remote gateway and the private CIDR routes supplied by your VPN administrator

The release application contains the macOS binaries, but the VPN client image is built locally. After installing Docker Desktop, run once from the source directory:

```bash
docker build -t vpntoris-client docker/
```

## Install

1. Download the notarized DMG from [Releases](https://github.com/maliyilmaz0/vpntoris/releases).
2. Drag `VPNToris.app` to `Applications`.
3. Start Docker Desktop and wait until the engine is ready.
4. Open VPNToris. On the first connection, macOS asks for administrator authorization to install the routing helper.
5. Add a profile, enter one or more destination networks in CIDR form (for example `10.38.0.0/16, 10.68.236.0/24`) and connect.

Only the configured destinations go through a VPN. Normal internet traffic keeps using the Mac's existing default route. When private networks overlap, macOS selects the most specific matching route; avoid assigning the same prefix to two active profiles unless that is intentional.

## OTP / 2FA

Enable **Ask for 2FA / OTP** on the profile. Start the connection first; when the gateway requests the second factor, VPNToris keeps the connection card open and presents the OTP field. Enter the newly received code there. IPsec XAuth OTP is passed to the container through a short-lived FIFO rather than a Docker environment variable.

## Security notes

- Profiles are stored for the current user at `~/Library/Application Support/VPNToris/configs.json` with user-only file permissions. Credentials are currently present in that file in plaintext; protect the macOS account and do not share the file. Keychain storage is planned.
- The local management API listens only on `127.0.0.1:17984`.
- The privileged helper accepts a limited route start/stop protocol over `/var/run/vpntoris/router.sock` and validates requested CIDRs and ports.
- Local profile files, logs, build products and secret files are excluded from Git. Never commit exported VPN configurations.

## Build from source

Install Xcode Command Line Tools, Go, Docker Desktop and Xcode:

```bash
xcode-select --install
brew install go
```

Build and test the source:

```bash
cd vpntoris-tray
go test ./...
cd ..
./scripts/release.sh --unsigned
```

The unsigned DMG is written to `dist/`. It deliberately contains no Apple Developer ID or notarization. Before distributing or opening it, sign each executable and then the application bundle with your own Apple Developer ID certificate:

```bash
APP="build/release/VPNToris.app"
IDENTITY="Developer ID Application: YOUR NAME (TEAMID)"
for BIN in tun2socks vpntoris-route-helper vpntorisd VPNToris; do
  codesign --force --options runtime --timestamp --sign "$IDENTITY" "$APP/Contents/MacOS/$BIN"
done
codesign --force --deep --options runtime --timestamp --sign "$IDENTITY" "$APP"
```

For an official signed and notarized build, the script can instead use a Developer ID Application certificate and a `notarytool` keychain profile:

```bash
SIGN_IDENTITY="Developer ID Application: …" \
NOTARY_PROFILE="FASTNAC_NOTARIZE" \
./scripts/release.sh
```

The release script compiles optimized Go and Swift binaries, builds the app icon, bundles `tun2socks`, applies hardened-runtime signatures, creates a DMG, submits it to Apple notarization and staples the ticket.

## Architecture

```text
SwiftUI menu bar app
        │  localhost API
        ▼
Go controller ───── Docker Engine ───── one VPN + SOCKS container per profile
        │
        ▼
privileged route helper ───── tun2socks/utun ───── destination CIDR routes
```

## Upcoming

- Intel-based macOS support (`x86_64` and Universal Binary)
- Linux desktop application
- Windows desktop application

## Troubleshooting

- **Docker unavailable:** open Docker Desktop and wait for `docker info` to succeed.
- **Connected but host is unreachable:** confirm that the host belongs to a CIDR configured on that profile, then inspect **Active Routes** and **Logs**.
- **IPsec stalls before OTP:** verify IKE version/mode, PSK, XAuth settings, Phase 1/2 proposals and UDP 500/4500 reachability.
- **Route helper asks repeatedly:** remove a stale helper only if necessary, then reconnect so VPNToris can reinstall the signed version.
- **Overlapping local network:** enter a more specific remote CIDR instead of routing a broad block that also contains the local LAN.

## Author

Mehmet Ali YILMAZ

## Third-party software

The app bundles [`tun2socks`](https://github.com/xjasonlyu/tun2socks) under its upstream license. The container image uses Ubuntu packages and a patched strongSwan `xauth-generic` plugin; their respective upstream licenses apply.
