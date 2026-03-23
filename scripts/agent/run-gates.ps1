param(
  [string]$TaskFile,
  [string]$EvidenceDir,
  [string]$Profile,
  [string[]]$Gates,
  [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')

$RootDir = Get-AgentRootDir
if (-not $EvidenceDir) {
  $EvidenceDir = Join-Path (Get-AgentArtifactRoot) (Join-Path 'manual' (Get-AgentTimestamp))
}
Ensure-AgentDir $EvidenceDir
Ensure-AgentDir (Join-Path $EvidenceDir 'logs')
Ensure-AgentDir (Join-Path $EvidenceDir 'gates')

if ((-not $Gates -or $Gates.Count -eq 0) -and $TaskFile) {
  $task = Get-TaskFileObject -TaskFile $TaskFile
  $Gates = @($task.required_gates)
}
if ((-not $Gates -or $Gates.Count -eq 0) -and $Profile) {
  $catalog = Get-Content -LiteralPath (Join-Path $RootDir 'agent\gates.json') -Raw -Encoding UTF8 | ConvertFrom-Json
  $Gates = @($catalog.default_profiles.$Profile)
}
if (-not $Gates -or $Gates.Count -eq 0) {
  $catalog = Get-Content -LiteralPath (Join-Path $RootDir 'agent\gates.json') -Raw -Encoding UTF8 | ConvertFrom-Json
  $Gates = @($catalog.default_profiles.default)
}

$results = New-Object System.Collections.Generic.List[object]
$failures = 0

function Invoke-AgentGate {
  param(
    [string]$Name,
    [scriptblock]$Action
  )
  $gateDir = Join-Path $EvidenceDir (Join-Path 'gates' $Name)
  Ensure-AgentDir $gateDir
  $logFile = Join-Path $EvidenceDir (Join-Path 'logs' ($Name + '.log'))
  $start = Get-Date
  $status = 'PASS'
  $summary = ''
  try {
    if ($DryRun) {
      Set-Content -LiteralPath $logFile -Value "[DRY-RUN] $Name" -Encoding UTF8
    } else {
      & $Action *>&1 | Tee-Object -FilePath $logFile | Out-Null
    }
  } catch {
    $status = 'FAIL'
    $summary = $_ | Out-String
    $failures++
    Add-Content -LiteralPath $logFile -Value "`n[EXCEPTION] $summary"
  }
  $elapsed = [int]((Get-Date) - $start).TotalSeconds
  Write-Host "[$status] $Name ($elapsed s)"
  $results.Add([pscustomobject]@{
    name = $Name
    status = $status
    duration_sec = $elapsed
    log_path = $logFile
    failure_summary = $summary.Trim()
  }) | Out-Null
}

foreach ($gate in $Gates) {
  switch ($gate) {
    'repo_consistency' {
      Invoke-AgentGate -Name $gate -Action {
        $env:ARTIFACT_DIR = (Join-Path $EvidenceDir (Join-Path 'gates' $gate))
        & (Join-Path $RootDir 'scripts\check-consistency.ps1')
      }
    }
    'strict_source' {
      Invoke-AgentGate -Name $gate -Action {
        & (Join-Path $RootDir 'scripts\acceptance\verify_phase1_strict_source.ps1') -RootDir $RootDir -ReportDir (Join-Path $EvidenceDir (Join-Path 'gates' $gate))
      }
    }
    'smoke' {
      Invoke-AgentGate -Name $gate -Action {
        $env:SMOKE_WORK_DIR = (Join-Path $EvidenceDir (Join-Path 'gates' $gate))
        $env:SMOKE_LOG_FILE = (Join-Path $EvidenceDir (Join-Path 'gates' $gate 'smoke.log'))
        & (Join-Path $RootDir 'scripts\smoke\run.ps1')
      }
    }
    'longrun_smoke' {
      Invoke-AgentGate -Name $gate -Action {
        $env:LONGRUN_REPORT_DIR = (Join-Path $EvidenceDir (Join-Path 'gates' $gate 'longrun'))
        & (Join-Path $RootDir 'scripts\longrun\run.ps1') -Mode smoke
      }
    }
    'ci_quality' {
      Invoke-AgentGate -Name $gate -Action {
        $env:ARTIFACT_DIR = (Join-Path $EvidenceDir (Join-Path 'gates' $gate))
        & (Join-Path $RootDir 'scripts\ci\run_quality_gates.ps1')
      }
    }
    'ui_delivery_guard' {
      Invoke-AgentGate -Name $gate -Action {
        Push-Location $RootDir
        try {
          node .\scripts\ui-delivery-guard.mjs
        } finally {
          Pop-Location
        }
      }
    }
    default {
      $failures++
      $results.Add([pscustomobject]@{
        name = $gate
        status = 'FAIL'
        duration_sec = 0
        log_path = 'unsupported'
        failure_summary = 'unsupported gate on windows or gate not implemented'
      }) | Out-Null
      Write-Host "[FAIL] $gate (unsupported)"
    }
  }
}

$report = [ordered]@{
  generated_at = [DateTime]::UtcNow.ToString('o')
  platform = 'windows'
  task_file = $(if ($TaskFile) { $TaskFile } else { $null })
  profile = $(if ($Profile) { $Profile } else { $null })
  overall = $(if ($failures -eq 0) { 'PASS' } else { 'FAIL' })
  summary = [ordered]@{
    total = $results.Count
    pass = @($results | Where-Object { $_.status -eq 'PASS' }).Count
    fail = @($results | Where-Object { $_.status -eq 'FAIL' }).Count
  }
  items = $results
}
$reportJson = Join-Path $EvidenceDir 'gate-results.json'
$reportMd = Join-Path $EvidenceDir 'gate-results.md'
$report | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $reportJson -Encoding UTF8

$lines = New-Object System.Collections.Generic.List[string]
$lines.Add('# Agent Gate Results') | Out-Null
$lines.Add('') | Out-Null
$lines.Add("- GeneratedAt(UTC): ``$($report.generated_at)``") | Out-Null
$lines.Add('- Platform: `windows`') | Out-Null
$lines.Add("- Overall: **$($report.overall)**") | Out-Null
$lines.Add('') | Out-Null
$lines.Add('| Gate | Status | Duration(s) | Log | Fail Summary |') | Out-Null
$lines.Add('|---|---|---:|---|---|') | Out-Null
foreach ($item in $results) {
  $fail = if ([string]::IsNullOrWhiteSpace($item.failure_summary)) { '-' } else { ([string]$item.failure_summary).Replace("`r`n", '<br>').Replace("`n", '<br>') }
  $lines.Add("| ``$($item.name)`` | $($item.status) | $($item.duration_sec) | ``$($item.log_path)`` | $fail |") | Out-Null
}
Set-Content -LiteralPath $reportMd -Value $lines -Encoding UTF8
Write-Host $reportMd
Write-Host $reportJson
if ($failures -gt 0) {
  exit 1
}
