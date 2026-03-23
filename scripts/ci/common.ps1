$ErrorActionPreference = 'Stop'

$RootDir = Resolve-Path (Join-Path $PSScriptRoot '..\..')
$ArtifactDir = if ($env:ARTIFACT_DIR) { $env:ARTIFACT_DIR } else { Join-Path $RootDir 'artifacts\ci' }
$LogDir = Join-Path $ArtifactDir 'logs'
$ReportDir = Join-Path $ArtifactDir 'reports'
$MetricsDir = Join-Path $ArtifactDir 'metrics'
New-Item -ItemType Directory -Force -Path $LogDir, $ReportDir, $MetricsDir | Out-Null

$script:GateResults = New-Object System.Collections.Generic.List[object]
$script:GateFailures = 0

function Invoke-CiGate {
  param(
    [string]$Name,
    [scriptblock]$Action
  )
  $logFile = Join-Path $LogDir ($Name + '.log')
  $start = Get-Date
  $status = 'PASS'
  $summary = ''
  try {
    Push-Location $RootDir
    try {
      & $Action *>&1 | Tee-Object -FilePath $logFile | Out-Null
    } finally {
      Pop-Location
    }
  } catch {
    $status = 'FAIL'
    $summary = $_ | Out-String
    $script:GateFailures++
    Add-Content -LiteralPath $logFile -Value "`n[EXCEPTION] $summary"
  }
  $elapsed = [int]((Get-Date) - $start).TotalSeconds
  Write-Host "[$status] $Name ($elapsed s)"
  $script:GateResults.Add([pscustomobject]@{
    name = $Name
    status = $status
    duration_sec = $elapsed
    failure_summary = $summary.Trim()
  }) | Out-Null
}

function Write-CiReport {
  param([string]$Suite)
  $overall = if ($script:GateFailures -eq 0) { 'PASS' } else { 'FAIL' }
  $report = [ordered]@{
    suite = $Suite
    generated_at = [DateTime]::UtcNow.ToString('o')
    overall = $overall
    summary = [ordered]@{
      total = $script:GateResults.Count
      pass = @($script:GateResults | Where-Object { $_.status -eq 'PASS' }).Count
      fail = @($script:GateResults | Where-Object { $_.status -eq 'FAIL' }).Count
    }
    items = $script:GateResults
  }
  $jsonPath = Join-Path $ReportDir ($Suite + '-report.json')
  $mdPath = Join-Path $ReportDir ($Suite + '-report.md')
  $report | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $jsonPath -Encoding UTF8
  $lines = New-Object System.Collections.Generic.List[string]
  $lines.Add("# $Suite 报告") | Out-Null
  $lines.Add('') | Out-Null
  $lines.Add("- GeneratedAt(UTC): ``$($report.generated_at)``") | Out-Null
  $lines.Add("- Overall: **$overall**") | Out-Null
  $lines.Add('') | Out-Null
  $lines.Add('| Gate | Status | Duration(s) | Fail Summary |') | Out-Null
  $lines.Add('|---|---|---:|---|') | Out-Null
  foreach ($item in $script:GateResults) {
    $fail = if ([string]::IsNullOrWhiteSpace($item.failure_summary)) { '-' } else { ([string]$item.failure_summary).Replace("`r`n", '<br>').Replace("`n", '<br>') }
    $lines.Add("| ``$($item.name)`` | $($item.status) | $($item.duration_sec) | $fail |") | Out-Null
  }
  Set-Content -LiteralPath $mdPath -Value $lines -Encoding UTF8
  Write-Host "report: $mdPath"
  Write-Host "report: $jsonPath"
  if ($script:GateFailures -gt 0) {
    exit 1
  }
}
