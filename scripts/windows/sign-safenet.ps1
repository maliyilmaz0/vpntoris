param(
    [Parameter(Mandatory = $true)]
    [string[]]$InputPath,
    [string]$CertificateThumbprint,
    [ValidateSet("CurrentUser", "LocalMachine")]
    [string]$CertificateStoreLocation,
    [string]$TimestampUrl
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$envPath = Join-Path $repoRoot ".env"
if (Test-Path $envPath) {
    foreach ($line in Get-Content $envPath) {
        $trimmed = $line.Trim()
        if ($trimmed.Length -eq 0 -or $trimmed.StartsWith("#") -or -not $trimmed.Contains("=")) {
            continue
        }
        $parts = $trimmed.Split("=", 2)
        $value = $parts[1].Trim() -replace '^(?:"(.*)"|''(.*)'')$', '$1$2'
        if ([string]::IsNullOrEmpty([Environment]::GetEnvironmentVariable($parts[0]))) {
            [Environment]::SetEnvironmentVariable($parts[0], $value)
        }
    }
}

if ([string]::IsNullOrWhiteSpace($CertificateThumbprint)) {
    $CertificateThumbprint = $env:VPNTORIS_WINDOWS_CERT_THUMBPRINT
}

if ([string]::IsNullOrWhiteSpace($CertificateStoreLocation)) {
    $CertificateStoreLocation = $env:VPNTORIS_WINDOWS_CERT_STORE_LOCATION
}

if ([string]::IsNullOrWhiteSpace($CertificateStoreLocation)) {
    $CertificateStoreLocation = "CurrentUser"
}

if ($CertificateStoreLocation -notin @("CurrentUser", "LocalMachine")) {
    throw "VPNTORIS_WINDOWS_CERT_STORE_LOCATION must be CurrentUser or LocalMachine."
}

if ([string]::IsNullOrWhiteSpace($TimestampUrl)) {
    $TimestampUrl = $env:VPNTORIS_WINDOWS_TIMESTAMP_URL
}

if ([string]::IsNullOrWhiteSpace($CertificateThumbprint)) {
    throw "VPNTORIS_WINDOWS_CERT_THUMBPRINT is required."
}

if ([string]::IsNullOrWhiteSpace($TimestampUrl)) {
    $TimestampUrl = "http://timestamp.digicert.com"
}

$normalizedThumbprint = ($CertificateThumbprint -replace "\s", "").ToUpperInvariant()
$certificate = Get-ChildItem "Cert:\$CertificateStoreLocation\My" | Where-Object { $_.Thumbprint -eq $normalizedThumbprint } | Select-Object -First 1

if ($null -eq $certificate) {
    throw "The signing certificate was not found in Cert:\$CertificateStoreLocation\My."
}

if (-not $certificate.HasPrivateKey) {
    throw "The selected certificate has no SafeNet-backed private key available."
}

$kitsRoot = Join-Path ${env:ProgramFiles(x86)} "Windows Kits\10\bin"
$signtool = Get-ChildItem $kitsRoot -Filter signtool.exe -Recurse | Where-Object { $_.FullName -match "\\x64\\signtool.exe$" } | Sort-Object FullName -Descending | Select-Object -First 1

if ($null -eq $signtool) {
    throw "signtool.exe was not found. Install the Windows SDK."
}

$storeArguments = @()
if ($CertificateStoreLocation -eq "LocalMachine") {
    $storeArguments += "/sm"
}

foreach ($path in $InputPath) {
    $resolved = (Resolve-Path $path).Path
    & $signtool.FullName sign /fd SHA256 /td SHA256 /tr $TimestampUrl /s MY @storeArguments /sha1 $normalizedThumbprint $resolved
    if ($LASTEXITCODE -ne 0) {
        throw "Signing failed for $resolved."
    }
    & $signtool.FullName verify /pa /all /v $resolved
    if ($LASTEXITCODE -ne 0) {
        throw "Signature verification failed for $resolved."
    }
}
