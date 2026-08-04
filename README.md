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
- Ordered primary/backup gateways with negotiation-based failover and persisted active endpoint
- Fortinet SSL VPN (`openfortivpn`), FortiClient-compatible IPsec, OpenConnect and OpenVPN profiles
- IKEv1/IKEv2, Main/Aggressive mode, XAuth/EAP, Mode Config, NAT-T, DPD, Phase 1/2 proposals, PFS and common encryption/DH groups
- Explicit IPsec DH Group 20 (`ECP-384`) support for FortiClient-compatible profiles
- Inline OTP flow after negotiation starts, including cancel/retry controls
- Live per-profile traffic counters, transfer rates and connection history
- Persistent hourly/daily traffic analytics, reconnect totals, destinations and process rankings
- Event-specific notification preferences, sounds and quiet hours
- Active route, per-process connection and container log views
- Application-aware VPN flow visibility showing process name, PID, protocol, destination and selected profile
- Drag-and-drop OpenVPN/FortiClient profile import with review before saving
- Automatic discovery of installed FortiClient, OpenVPN Connect, Tunnelblick and Viscosity profiles
- Sanitized diagnostics ZIP export for support and troubleshooting
- Verified update downloads using the SHA-256 file attached to each GitHub release
- Secret-free JSON export and optional PBKDF2/AES-256-GCM encrypted profile backups
- English and Turkish application languages
- Bundled `vpntorisctl` command-line client with automatic symlink installation when the target directory is writable
- Per-profile edit and delete actions
- Embedded route helper and `tun2socks`; no separate system extension package
- Signed and notarized macOS release workflow

## Requirements

- Apple Silicon or Intel Mac (`arm64` or `x86_64`)
- macOS 13 Ventura or later
- [Docker Desktop for Mac](https://www.docker.com/products/docker-desktop/)
- An administrator account for the one-time route-helper installation through Terminal
- Network access to the VPN gateway; IPsec commonly requires outbound UDP 500 and 4500
- VPN credentials, remote gateway and the private CIDR routes supplied by your VPN administrator

VPNToris checks Docker automatically when it opens. If Docker Desktop is missing or its engine is stopped, the SwiftUI tray displays an installation/startup warning with the appropriate action. When Docker is ready but the `vpntoris-client` image does not exist, VPNToris builds it automatically from the Docker context embedded in the app bundle and displays the build progress. No manual `docker build` command is required.

## Install

1. Download the notarized DMG from [Releases](https://github.com/maliyilmaz0/vpntoris/releases).
2. Drag `VPNToris.app` to `Applications`.
3. Open VPNToris. It prepares the VPN image and attempts to install a `vpntorisctl` symlink under `/opt/homebrew/bin` or `/usr/local/bin`. Start Docker Desktop from the warning if it is not already running.
4. Install the routing helper once from Terminal. This uses the normal `sudo`/Touch ID path and prevents recurring AppleScript password dialogs:

```bash
sudo "/Applications/VPNToris.app/Contents/MacOS/vpntoris-route-helper" install "$(id -u)"
```
5. Add a profile, enter one or more destination networks in CIDR form (for example `10.38.0.0/16, 10.68.236.0/24`) and connect.

Only the configured destinations go through a VPN. Normal internet traffic keeps using the Mac's existing default route. When private networks overlap, macOS selects the most specific matching route; avoid assigning the same prefix to two active profiles unless that is intentional.

After the VPN tunnel becomes healthy, VPNToris waits three seconds and installs the profile's routes automatically. The profile card shows **Routes will be added**, **Adding routes** and **Routes active** states. **Reapply Routes** remains available for manual recovery.

## Protocol compatibility status

- FortiGate SSL VPN and FortiClient-compatible IPsec are the currently exercised connection paths.
- IPsec includes DH Group 20 (`ECP-384`) in both Phase 1 and PFS/Phase 2 selections.
- Palo Alto GlobalProtect support is implemented through OpenConnect but has not yet been tested end to end against a GlobalProtect gateway.
- OpenVPN profile import and connection support are implemented but have not yet been tested end to end against a production OpenVPN gateway.

## OTP / 2FA

Enable **Ask for 2FA / OTP** on the profile. Start the connection first; when the gateway requests the second factor, VPNToris keeps the connection card open and presents the OTP field. Enter the newly received code there. IPsec XAuth OTP is passed to the container through a short-lived FIFO rather than a Docker environment variable.

## Gateway failover

Enter backup gateway hostnames or IP addresses in the profile editor and choose how many failed automatic reconnect attempts are allowed before switching. VPNToris does not use ICMP or assume that ping is enabled. It evaluates the actual VPN negotiation and tunnel health, moves through the configured gateways in order and remembers the selected endpoint across application restarts. The active gateway and endpoint count are shown on the profile card. OpenVPN profiles receive a temporary runtime configuration containing the selected gateway; the saved `.ovpn` content is not modified.

## Security notes

- Profiles are stored for the current user at `~/Library/Application Support/VPNToris/configs.json` with user-only file permissions.
- Passwords and IPsec pre-shared keys are never saved inside the VPN profile JSON. When credentials are remembered, they are stored as separate macOS Keychain items. Saving or exporting an ordinary profile therefore does not embed its password.
- The local management API listens only on `127.0.0.1:17984`.
- The privileged helper accepts a limited route start/stop protocol over `/var/run/vpntoris/router.sock` and validates requested CIDRs and ports.
- Diagnostics export excludes Keychain credentials and container environments, clears legacy credential fields and masks password, secret, PSK, token and OTP patterns in collected output.
- The updater verifies a downloaded DMG against the release's SHA-256 asset before saving it. Installation remains an explicit user action.
- Local profile files, logs, build products and secret files are excluded from Git. Never commit exported VPN configurations.

## Profile import

Drop an OpenVPN `.ovpn`/`.conf` or FortiClient export file directly onto the tray window, or choose **Import VPN Profile…** from the tray menu. The OpenVPN editor also has its own drop area. **Discover Installed VPN Profiles…** scans the standard macOS locations used by FortiClient, OpenVPN Connect, Tunnelblick and Viscosity, including FortiClient's multi-profile `vpn.plist`. VPNToris extracts non-secret connection fields and opens the normal editor for review. Passwords and pre-shared keys are not imported. Add the destination CIDRs and credentials before saving. Imported files are never copied into the repository.

## Command line

The application bundle contains `vpntorisctl`. On launch, VPNToris automatically creates a symlink in `/opt/homebrew/bin` when that directory is available and writable. Intel/Homebrew installations can use `/usr/local/bin`. Open **Help and CLI** from the tray menu to inspect the installation, copy commands or install the link manually.

```bash
sudo ln -sf "/Applications/VPNToris.app/Contents/MacOS/vpntorisctl" "/usr/local/bin/vpntorisctl"
vpntorisctl status
vpntorisctl profiles
vpntorisctl flows
vpntorisctl routes
vpntorisctl check-route 10.38.1.251
VPNTORIS_PASSWORD='…' vpntorisctl connect "Profile Name"
VPNTORIS_PASSWORD='…' VPNTORIS_PSK='…' vpntorisctl connect "IPsec Profile"
vpntorisctl disconnect "Profile Name"
vpntorisctl logs "Profile Name"
```

The tray application must be running because the CLI talks to its localhost controller. `VPNTORIS_PASSWORD` and `VPNTORIS_PSK` are read from the environment and sent only to the localhost API. They are never accepted as command-line arguments or written into the profile file by the CLI.

## Application-aware traffic visibility

Open **Active Connections** to see which local application is currently producing traffic for a configured VPN destination. Each flow includes the process name, PID, TCP destination, protocol and selected VPN profile. SSH sessions, VS Code Remote connections, browsers and database clients can therefore be associated with the tunnel they are using. **Traffic Analytics** provides longer-term per-profile totals, hourly/daily graphs, reconnect counts and sampled process/destination rankings. VPNToris records connection metadata and counters, not packet payloads or process command-line arguments.

## Diagnostics

Choose **Export Diagnostics…** from the tray menu to create a ZIP containing a sanitized summary, route and DNS state, Docker container status and the last 500 lines of each VPNToris container log. Review the archive before sharing it because gateway names, usernames, private CIDRs and hostnames may still be operationally sensitive even though credential values are removed.

## Backup and restore

The default JSON export contains profiles without passwords or IPsec pre-shared keys. Encrypted backups can optionally include Keychain credentials. They use PBKDF2-HMAC-SHA256 with 200,000 iterations for key derivation and AES-256-GCM for authenticated encryption. Existing profiles with matching names are replaced during restore.

## Traffic analytics and notifications

VPNToris retains hourly traffic buckets for seven days and daily buckets for 90 days. It also records reconnect counts and sampled destination/process names without recording payloads, DNS contents or command-line arguments. The Notifications screen controls connect, disconnect, gateway, OTP, Docker, route-conflict and update events independently, including sound and quiet-hour preferences.

## Updates

VPNToris checks the latest GitHub release in the background and also provides **Check for Updates…** in the tray menu. It downloads only a DMG that has a matching `.sha256` release asset, verifies the digest locally and saves the installer under `~/Downloads/VPNToris Updates`. The running application is not silently replaced.

## Build from source

Install Xcode Command Line Tools, Go, Docker Desktop and Xcode:

```bash
xcode-select --install
brew install go
```

The current source targets Go `1.26.5` and tun2socks `v2.7.0`, the latest stable versions resolved when this release branch was prepared.

Build and test the source:

```bash
cd vpntoris-tray
go test ./...
cd ..
./scripts/release.sh --unsigned
```

The unsigned Universal DMG is written to `dist/`. It deliberately contains no Apple Developer ID or notarization. Set `ARCH=arm64` or `ARCH=x86_64` to create a single-architecture build. Before distributing or opening it, sign each executable and then the application bundle with your own Apple Developer ID certificate:

```bash
APP="build/release/VPNToris.app"
IDENTITY="Developer ID Application: YOUR NAME (TEAMID)"
for BIN in tun2socks vpntoris-route-helper vpntorisd vpntorisctl VPNToris; do
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
