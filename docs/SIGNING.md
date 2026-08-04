# Release signing

## Windows SafeNet USB token

The Windows private key remains on the SafeNet USB token. Install SafeNet Authentication Client, the Windows SDK and a self-hosted GitHub Actions runner on the signing machine. Import only the public certificate into the Windows certificate store and confirm that it reports a hardware-backed private key while the token is connected.

Set `WINDOWS_SIGNING_CERT_THUMBPRINT` on the self-hosted runner. The signing script selects the certificate by thumbprint, invokes the SafeNet provider through `signtool`, applies an RFC 3161 timestamp and verifies every signature. The token PIN is not stored in the repository or passed as a command-line argument. Token authentication is handled by SafeNet Authentication Client according to the token policy.

```powershell
powershell -ExecutionPolicy Bypass -File scripts/windows/sign-safenet.ps1 -InputPath dist\VPNToris.exe,dist\VPNToris.msi
```

The signing job must use a dedicated self-hosted runner label and must not run for untrusted pull requests. Build artifacts can be produced by GitHub-hosted runners, but signing and release publication happen only in the protected environment attached to the USB token machine.

## Linux GPG

Linux packages and repository metadata use a maintainer-owned GPG signing subkey. The private subkey is supplied to the protected release job through an encrypted CI secret or a hardware token. It is never committed to Git.

```bash
GPG_SIGNING_KEY_ID=KEYID scripts/linux/sign-artifacts.sh dist/*.deb dist/*.rpm dist/*.AppImage
```

Publish the armored public key with the package repository and document its full fingerprint. DEB `Release`/`InRelease` metadata and RPM repository metadata must be signed in addition to detached package signatures.
