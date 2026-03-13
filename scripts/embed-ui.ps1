$ErrorActionPreference = 'Stop'

$RootDir = Split-Path -Parent $PSScriptRoot
$UiDir = Join-Path $RootDir 'gateway-ui'
$TargetDir = Join-Path $RootDir 'gateway-server/internal/server/embedded-ui'

if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
  throw 'npm not found in PATH'
}

Push-Location $UiDir
try {
  npm run build
}
finally {
  Pop-Location
}

$AssetsDir = Join-Path $TargetDir 'assets'
if (Test-Path $AssetsDir) {
  Remove-Item -Recurse -Force $AssetsDir
}

New-Item -ItemType Directory -Force -Path $TargetDir | Out-Null
Copy-Item -Recurse -Force (Join-Path $UiDir 'dist/*') $TargetDir

Write-Host "embedded UI assets synced to $TargetDir"
