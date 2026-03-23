param(
  [ValidateSet('smoke','1h','6h','24h')]
  [string]$Mode = $(if ($env:LONGRUN_MODE) { $env:LONGRUN_MODE } else { 'smoke' })
)

$ErrorActionPreference = 'Stop'
$RootDir = Resolve-Path (Join-Path $PSScriptRoot '..\..')
$env:LONGRUN_ENABLE = if ($env:LONGRUN_ENABLE) { $env:LONGRUN_ENABLE } else { '1' }
$env:LONGRUN_MODE = $Mode
if (-not $env:LONGRUN_REPORT_DIR) {
  $env:LONGRUN_REPORT_DIR = Join-Path $RootDir 'gateway-server\tests\longrun\output'
}
Push-Location (Join-Path $RootDir 'gateway-server')
try {
  go test ./tests/longrun -run TestLongRunStability -count=1 -v
} finally {
  Pop-Location
}
Write-Host "[INFO] longrun completed; reports in $($env:LONGRUN_REPORT_DIR)"
