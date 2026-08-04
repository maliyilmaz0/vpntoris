param(
    [Parameter(Mandatory = $true)]
    [string[]]$InputPath,
    [string]$CertificateThumbprint = $env:WINDOWS_SIGNING_CERT_THUMBPRINT,
    [ValidateSet("CurrentUser", "LocalMachine")]
    [string]$CertificateStoreLocation = "CurrentUser",
    [string]$TimestampUrl = $env:WINDOWS_TIMESTAMP_URL
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($CertificateThumbprint)) {
    throw "WINDOWS_SIGNING_CERT_THUMBPRINT is required."
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
