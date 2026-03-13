$ErrorActionPreference = 'Stop'
$RootDir = Split-Path -Parent $PSScriptRoot
$UiDir = Join-Path $RootDir 'gateway-ui'

if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
  throw 'npm not found in PATH'
}

Push-Location $UiDir
try {
  if (-not (Test-Path 'node_modules')) {
    Write-Host '[ui-build] node_modules not found, running npm install'
    npm install
  }

  npm run build
  Write-Host "[ui-build] build output: $(Join-Path $UiDir 'dist')"
}
finally {
  Pop-Location
}
