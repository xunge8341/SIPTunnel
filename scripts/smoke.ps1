$nested = Join-Path $PSScriptRoot 'smoke/run.ps1'
if (Test-Path $nested) {
  Unblock-File -Path $nested -ErrorAction SilentlyContinue
}
& $nested
