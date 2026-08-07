param(
    [string]$OutputRoot = "",
    [string]$OpenVPNBin = "",
    [string]$WintunDll = ""
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
if ([string]::IsNullOrWhiteSpace($OutputRoot)) {
    $OutputRoot = Join-Path $repoRoot ".build\native-engines\windows-amd64\openvpn"
}

New-Item -ItemType Directory -Force -Path (Join-Path $OutputRoot "bin") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $OutputRoot "lib") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $OutputRoot "licenses") | Out-Null

if ([string]::IsNullOrWhiteSpace($OpenVPNBin) -or -not (Test-Path $OpenVPNBin)) {
    @"
VPNToris Windows OpenVPN packaging (manual Windows-host fallback).

Preferred path on the maintainer macOS host (complete MSI pipeline):

  ./scripts/windows/build-engines.sh
  VERSION=2.0.0 ./scripts/windows/build-msi.sh

This PowerShell helper packages a local openvpn.exe + optional wintun.dll:

  .\scripts\windows\build-openvpn.ps1 ``
      -OpenVPNBin C:\path\to\openvpn.exe ``
      -WintunDll C:\path\to\wintun.dll

The helper never resolves openvpn from PATH. Packaged layout:

  .build/native-engines/windows-amd64/openvpn/
    bin/openvpn.exe
    lib/wintun.dll          (optional but recommended)
    manifest.json
"@ | Set-Content -Path (Join-Path $OutputRoot "README.build.txt") -Encoding UTF8
    Write-Host "Scaffold written to $OutputRoot (no binary packaged)."
    Write-Host "On macOS use scripts/windows/build-engines.sh for the complete product."
    exit 0
}

Copy-Item -Force $OpenVPNBin (Join-Path $OutputRoot "bin\openvpn.exe")
if (-not [string]::IsNullOrWhiteSpace($WintunDll) -and (Test-Path $WintunDll)) {
    Copy-Item -Force $WintunDll (Join-Path $OutputRoot "lib\wintun.dll")
}

$enginePath = Join-Path $OutputRoot "bin\openvpn.exe"
$sha = (Get-FileHash -Algorithm SHA256 -Path $enginePath).Hash.ToLowerInvariant()
$files = @{}
$wintunPath = Join-Path $OutputRoot "lib\wintun.dll"
if (Test-Path $wintunPath) {
    $files["openvpn/lib/wintun.dll"] = (Get-FileHash -Algorithm SHA256 -Path $wintunPath).Hash.ToLowerInvariant()
}

$manifest = [ordered]@{
    id           = "openvpn"
    protocol     = "openvpn"
    version      = "packaged"
    os           = "windows"
    architecture = "amd64"
    executable   = "openvpn/bin/openvpn.exe"
    sha256       = $sha
    license      = "GPL-2.0-only WITH OpenSSL-exception"
    capabilities = @("tun", "userpass", "challenge", "split-route", "wintun")
    files        = $files
}
$manifest | ConvertTo-Json -Depth 5 | Set-Content -Path (Join-Path $OutputRoot "manifest.json") -Encoding UTF8
Write-Host "OpenVPN Windows package ready: $OutputRoot"
