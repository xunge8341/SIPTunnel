param(
  [string]$BuildNonce = [guid]::NewGuid().ToString()
)

$ErrorActionPreference = 'Stop'

$RootDir = Split-Path -Parent $PSScriptRoot
$UiDir = Join-Path $RootDir 'gateway-ui'
$DistDir = Join-Path $UiDir 'dist'

if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
  throw 'npm not found in PATH'
}

Push-Location $UiDir
try {
  if (-not (Test-Path 'node_modules')) {
    Write-Host '[ui-build] node_modules not found, running npm install'
    npm install
    if ($LASTEXITCODE -ne 0) {
      throw "npm install failed with exit code $LASTEXITCODE"
    }
  }

  if (Test-Path $DistDir) {
    Write-Host "[ui-build] removing stale dist at $DistDir"
    Remove-Item -Recurse -Force $DistDir
  }

  npm run build
  if ($LASTEXITCODE -ne 0) {
    throw "UI build failed with exit code $LASTEXITCODE. Aborting before dist sync/embedding."
  }

  if (-not (Test-Path $DistDir)) {
    throw "UI build completed but dist directory was not generated: $DistDir"
  }

  $MarkerFile = Join-Path $DistDir '.siptunnel-build-nonce'
  Set-Content -Path $MarkerFile -Value $BuildNonce -Encoding UTF8

  Write-Host "[ui-build] build output: $DistDir"
  Write-Host "[ui-build] build nonce: $BuildNonce"
}
finally {
  Pop-Location
}
