param(
    [Parameter(Mandatory = $true)]
    [string]$EngineRoot,
    [string]$HelperPath = "",
    [string]$ServiceName = "VPNTorisNativeHelper"
)

$ErrorActionPreference = "Stop"
if ([string]::IsNullOrWhiteSpace($HelperPath)) {
    $HelperPath = Join-Path $PSScriptRoot "..\..\build\windows\vpntoris-native-helper.exe"
}

if (-not (Test-Path $HelperPath)) {
    throw "helper binary not found: $HelperPath"
}

$binPath = "`"$HelperPath`" service `"$EngineRoot`""
sc.exe create $ServiceName binPath= $binPath start= demand obj= LocalSystem | Out-Null
sc.exe description $ServiceName "VPNToris privileged native helper (named pipe)" | Out-Null
Write-Host "Service $ServiceName registered. Start with: sc.exe start $ServiceName"
Write-Host "Pipe: \\.\pipe\vpntoris-native-helper"
Write-Host "Engines: $EngineRoot\windows-amd64\..."
