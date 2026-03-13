param(
  [string]$Version = 'dev',
  [string]$OutputRoot = 'dist/windows',
  [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'
$RepoRoot = Split-Path -Parent $PSScriptRoot
$OutputRootAbs = Join-Path $RepoRoot $OutputRoot
$PackageDir = Join-Path $OutputRootAbs "SIPTunnel-$Version-windows-amd64"
$ZipPath = "$PackageDir.zip"
$GatewayExe = Join-Path $RepoRoot 'dist/bin/windows/amd64/gateway.exe'

if (-not $SkipBuild) {
  Write-Host '[package] build windows binary'
  & (Join-Path $RepoRoot 'scripts/build.ps1') -Mode matrix
}

if (-not (Test-Path $GatewayExe)) {
  throw "gateway.exe not found: $GatewayExe"
}

Write-Host "[package] prepare layout at $PackageDir"
Remove-Item -Force -Recurse $PackageDir -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $PackageDir | Out-Null
foreach ($dir in @('configs', 'data', 'logs', 'docs', 'scripts')) {
  New-Item -ItemType Directory -Force -Path (Join-Path $PackageDir $dir) | Out-Null
}

Copy-Item $GatewayExe (Join-Path $PackageDir 'gateway.exe') -Force
Copy-Item (Join-Path $RepoRoot 'gateway-server/configs/config.default.example.yaml') (Join-Path $PackageDir 'configs/config.yaml') -Force
Copy-Item (Join-Path $RepoRoot 'README.md') (Join-Path $PackageDir 'docs/README.md') -Force
Copy-Item (Join-Path $RepoRoot 'docs/windows-operations.md') (Join-Path $PackageDir 'docs/windows-operations.md') -Force
Copy-Item (Join-Path $RepoRoot 'scripts/smoke.ps1') (Join-Path $PackageDir 'docs/smoke.ps1') -Force

$StartScript = @'
param(
  [string]$Config = ".\configs\config.yaml"
)
$ErrorActionPreference = 'Stop'
$exe = Join-Path $PSScriptRoot 'gateway.exe'
& $exe --config $Config
'@
Set-Content -Path (Join-Path $PackageDir 'start-gateway.ps1') -Value $StartScript -Encoding UTF8

$ServiceScript = @'
param(
  [ValidateSet("install", "remove")]
  [string]$Action = "install",
  [string]$ServiceName = "SIPTunnelGateway",
  [string]$ConfigPath = ".\configs\config.yaml"
)

# skeleton only: adjust paths/credentials before production use.
$exe = Join-Path $PSScriptRoot '..\gateway.exe'
if ($Action -eq 'install') {
  New-Service -Name $ServiceName -BinaryPathName "`"$exe`" --config `"$ConfigPath`"" -DisplayName $ServiceName -StartupType Automatic
  Write-Host "service installed: $ServiceName"
} else {
  Stop-Service -Name $ServiceName -ErrorAction SilentlyContinue
  sc.exe delete $ServiceName | Out-Null
  Write-Host "service removed: $ServiceName"
}
'@
Set-Content -Path (Join-Path $PackageDir 'scripts/service-skeleton.ps1') -Value $ServiceScript -Encoding UTF8

Write-Host "[package] create zip $ZipPath"
Remove-Item -Force $ZipPath -ErrorAction SilentlyContinue
Compress-Archive -Path "$PackageDir/*" -DestinationPath $ZipPath -Force
Write-Host '[package] done'
