param(
  [ValidateSet('native', 'matrix')]
  [string]$Mode = 'matrix',
  [ValidateSet('delivery', 'dev')]
  [string]$UiPolicy = 'delivery',
  [switch]$SkipSource,
  [switch]$SkipServerTests
)

$ErrorActionPreference = 'Stop'
$RootDir = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$ReportDir = Join-Path $RootDir 'artifacts/acceptance'
$null = New-Item -ItemType Directory -Force -Path $ReportDir
$Timestamp = (Get-Date).ToString('yyyyMMdd-HHmmss')
$ReportPath = Join-Path $ReportDir "phase1-strict-windows-acceptance-$Timestamp.md"
$steps = New-Object System.Collections.Generic.List[object]

function Get-GoModCompatVersion {
  $goModPath = Join-Path $RootDir 'gateway-server/go.mod'
  if (Test-Path $goModPath) {
    $goDirective = Select-String -Path $goModPath -Pattern '^go\s+([0-9]+\.[0-9]+(?:\.[0-9]+)?)\s*$' | Select-Object -First 1
    if ($goDirective -and $goDirective.Matches.Count -gt 0) {
      $value = $goDirective.Matches[0].Groups[1].Value
      if (-not [string]::IsNullOrWhiteSpace($value)) {
        return $value
      }
    }
  }
  return '1.23.0'
}

function Sync-GoModuleGraph {
  $compatVersion = Get-GoModCompatVersion
  Push-Location (Join-Path $RootDir 'gateway-server')
  try {
    & go mod tidy "-compat=$compatVersion"
    if ($LASTEXITCODE -ne 0) {
      throw "[server_targeted] go mod tidy failed with exit code $LASTEXITCODE"
    }
  }
  finally {
    Pop-Location
  }
}

function Invoke-Step {
  param([string]$Name, [scriptblock]$Action)
  $sw = [System.Diagnostics.Stopwatch]::StartNew()
  $status = 'PASS'
  $summary = ''
  try {
    & $Action
  }
  catch {
    $status = 'FAIL'
    $summary = $_.Exception.Message
  }
  finally {
    $sw.Stop()
  }
  $steps.Add([pscustomobject]@{
    name    = $Name
    status  = $status
    seconds = [int][Math]::Ceiling($sw.Elapsed.TotalSeconds)
    summary = $summary
  }) | Out-Null
  Write-Host "[$status] $Name ($([int][Math]::Ceiling($sw.Elapsed.TotalSeconds))s)"
  if ($status -eq 'FAIL') {
    throw $summary
  }
}

$goVersionRaw = (& go version) 2>$null
if ($IsWindows -and $goVersionRaw -match 'go version go1\.26\.') {
  Write-Warning '[runtime] Windows + Go 1.26.x has a known upstream net/http runtime crash pattern; this repo enables a local keep-alive mitigation, but protocol sign-off should prefer Go 1.25.x or the project baseline Go 1.23.x.'
}

if (-not $SkipSource) {
  Invoke-Step 'source_strict' { & (Join-Path $RootDir 'scripts/acceptance/verify_phase1_strict_source.ps1') }
}
if (-not $SkipServerTests) {
  Invoke-Step 'server_targeted' {
    Sync-GoModuleGraph
    Push-Location (Join-Path $RootDir 'gateway-server')
    try {
      & go test ./internal/protocol/... ./internal/server ./internal/selfcheck ./internal/config -count=1
      if ($LASTEXITCODE -ne 0) {
        throw "[server_targeted] go test failed with exit code $LASTEXITCODE"
      }
    }
    finally {
      Pop-Location
    }
  }
}
Invoke-Step 'build_release' { & (Join-Path $RootDir 'scripts/build-release.ps1') -Mode $Mode -UiPolicy $UiPolicy }

$lines = New-Object System.Collections.Generic.List[string]
$lines.Add('# GB28181 Phase 1 Strict Mode Windows Acceptance Report') | Out-Null
$lines.Add('') | Out-Null
$lines.Add("- GeneratedAt(Local): ``$((Get-Date).ToString('yyyy-MM-ddTHH:mm:ssK'))``") | Out-Null
$lines.Add('') | Out-Null
$lines.Add('| Step | Result | Seconds | Note |') | Out-Null
$lines.Add('|---|---|---:|---|') | Out-Null
foreach ($s in $steps) {
  $remark = if ([string]::IsNullOrWhiteSpace($s.summary)) { '-' } else { $s.summary.Replace('|', '\|') }
  $lines.Add("| ``$($s.name)`` | $($s.status) | $($s.seconds) | $remark |") | Out-Null
}
Set-Content -Path $ReportPath -Value $lines -Encoding UTF8
Write-Host $ReportPath
