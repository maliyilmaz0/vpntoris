# VPNToris Native Engine

VPNToris 2 removes Docker as a runtime dependency. VPN engines, the controller and platform networking components ship inside the installer. Operating-system networking APIs remain necessary, but every change is scoped to a profile, journaled before application and reversible after a failure or crash.

## Shared architecture

The Go controller owns profiles, engine supervision, OTP, reconnect, failover, analytics and the transaction journal. Platform backends implement interface, route, split-DNS, process ownership and recovery operations. Native user interfaces communicate with the same local authenticated controller and never run privileged shell commands.

## Safety contract

- Default routes are rejected unless an explicit future full-tunnel mode is enabled.
- Existing interface addresses, global DNS, firewall rules and routes are not overwritten.
- Every mutation is persisted before it is applied.
- Partial activation rolls back in reverse order.
- Startup recovery removes only resources whose ownership can be verified.
- Recovery does not trust a journal entry without checking the live resource.
- An independent repair command can recover state without starting the UI.

## Platform backends

### macOS

The signed privileged helper creates utun interfaces, applies destination routes through SystemConfiguration/routing sockets and installs scoped resolvers. The PKG installs and upgrades the helper. SwiftUI remains the macOS tray interface.

### Linux

The privileged service uses `/dev/net/tun`, rtnetlink and systemd-resolved or resolvconf integration. Packages include systemd and polkit definitions. Distribution targets are DEB, RPM and AppImage. Repository metadata is signed in CI with a maintainer-owned GPG key supplied as an encrypted secret.

### Windows

The privileged Windows service uses bundled Wintun and IP Helper APIs. Credentials use Windows Credential Manager. MSI/MSIX artifacts are signed in CI with a maintainer-owned Authenticode certificate supplied by the CI secret store or hardware-backed signing provider.

## Engine packaging

FortiGate SSL, OpenVPN, OpenConnect/GlobalProtect and IPsec engines are invoked through a common supervisor. Initial releases bundle audited platform-specific engine binaries and libraries. They are not host prerequisites and are never resolved through PATH. Each engine manifest contains its protocol, supported architectures, digest, license and capability flags.

Release installers contain the VPN engines and their runtime libraries. Installation and first launch do not download engines, package-manager dependencies or container images. Homebrew and compiler tools are release-build dependencies only. Distributed engine files are signed and checked against the bundled manifest before the privileged service starts them. Corresponding licenses and redistributable source archives are included with the engine payload.

The macOS OpenVPN engine is packaged for both Apple Silicon and Intel. Apple Silicon uses the `darwin-arm64` payload. Intel uses a statically linked `darwin-amd64` payload containing OpenSSL LTS, LZO and LZ4. The privileged helper is a universal Mach-O binary and resolves the engine directory from its runtime architecture.

## Current macOS connection flow

FortiGate SSL and OpenVPN profiles run without Docker when the native helper is installed. The helper validates each OpenVPN profile again at its privilege boundary, rejects command-executing directives, disables configuration-supplied routes and DNS changes, and writes the sanitized configuration to a root-only temporary file. Usernames and passwords are sent through a pipe and never placed in process arguments or the configuration file.

The helper identifies the exact `pppN` or `utunN` interface created by each supervised process. Profile CIDRs are added only after that process reports a completed tunnel, and disconnect or failure removes only the routes owned by that session. Separate sessions therefore retain separate interfaces when multiple VPNs are connected concurrently. OpenVPN profiles that require an interactive challenge remain on the native-engine roadmap; the current native flow supports certificate-only and ordinary username/password authentication.
