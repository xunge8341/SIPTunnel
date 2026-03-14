$ErrorActionPreference = 'Stop'

$RootDir = Split-Path -Parent $PSScriptRoot
$UiDir = Join-Path $RootDir 'gateway-ui'
$DistDir = Join-Path $UiDir 'dist'
$TargetDir = Join-Path $RootDir 'gateway-server/internal/server/embedded-ui'

$BuildNonce = [guid]::NewGuid().ToString()

Write-Host "[embed-ui] running UI build with nonce: $BuildNonce"
& (Join-Path $RootDir 'scripts/ui-build.ps1') -BuildNonce $BuildNonce
if ($LASTEXITCODE -ne 0) {
  throw "[embed-ui] UI build step failed with exit code $LASTEXITCODE. Embedding aborted."
}

if (-not (Test-Path $DistDir)) {
  throw "[embed-ui] UI build output missing: $DistDir. Embedding aborted."
}

$MarkerFile = Join-Path $DistDir '.siptunnel-build-nonce'
if (-not (Test-Path $MarkerFile)) {
  throw "[embed-ui] build marker missing: $MarkerFile. Refusing to embed stale dist."
}

$ActualNonce = (Get-Content -Path $MarkerFile -Raw).Trim()
if ([string]::IsNullOrWhiteSpace($ActualNonce) -or $ActualNonce -ne $BuildNonce) {
  throw "[embed-ui] build marker nonce mismatch (expected: $BuildNonce, actual: $ActualNonce). Refusing to embed stale dist."
}

Write-Host '[embed-ui] build marker validated, syncing embedded assets'

if (Test-Path $TargetDir) {
  Remove-Item -Recurse -Force $TargetDir
}

New-Item -ItemType Directory -Force -Path $TargetDir | Out-Null
Copy-Item -Recurse -Force (Join-Path $DistDir '*') $TargetDir

if (Test-Path (Join-Path $TargetDir '.siptunnel-build-nonce')) {
  Remove-Item -Force (Join-Path $TargetDir '.siptunnel-build-nonce')
}

Write-Host "embedded UI assets synced to $TargetDir"
