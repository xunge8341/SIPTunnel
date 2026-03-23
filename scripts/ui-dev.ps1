param(
  [ValidateSet('mock', 'real')]
  [string]$Mode = 'mock'
)

$ErrorActionPreference = 'Stop'
$RootDir = Split-Path -Parent $PSScriptRoot
$UiDir = Join-Path $RootDir 'gateway-ui'

if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
  throw 'npm not found in PATH'
}

Push-Location $UiDir
try {
  if (-not (Test-Path 'node_modules')) {
    Write-Host '[ui-dev] node_modules not found, running npm install'
    npm install
  }

  if ($Mode -eq 'real') {
    $ApiBaseUrl = if ($env:VITE_API_BASE_URL) { $env:VITE_API_BASE_URL } else { 'http://127.0.0.1:18080/api' }
    Write-Host "[ui-dev] starting in real API mode: $ApiBaseUrl"
    $env:VITE_API_MODE = 'real'
    $env:VITE_API_BASE_URL = $ApiBaseUrl
    npm run dev
  }
  else {
    Write-Host '[ui-dev] starting in mock API mode'
    Remove-Item Env:VITE_API_MODE -ErrorAction SilentlyContinue
    Remove-Item Env:VITE_API_BASE_URL -ErrorAction SilentlyContinue
    npm run dev
  }
}
finally {
  Pop-Location
}
