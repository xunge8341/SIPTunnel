$ErrorActionPreference = 'Stop'
$RootDir = Split-Path -Parent $PSScriptRoot
$UiDir = Join-Path $RootDir 'gateway-ui'

if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
  throw 'npm not found in PATH'
}

Push-Location $UiDir
try {
  if (-not (Test-Path 'dist')) {
    Write-Host '[ui-preview] dist not found, running ui build first'
    & (Join-Path $RootDir 'scripts/ui-build.ps1')
  }

  npm run preview
}
finally {
  Pop-Location
}
