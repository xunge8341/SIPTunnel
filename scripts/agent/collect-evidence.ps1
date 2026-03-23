param(
  [Parameter(Mandatory = $true)][string]$TaskFile,
  [Parameter(Mandatory = $true)][string]$EvidenceDir
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')

$RootDir = Get-AgentRootDir
Ensure-AgentDir $EvidenceDir
Ensure-AgentDir (Join-Path $EvidenceDir 'config-snapshot')
Copy-Item -LiteralPath $TaskFile -Destination (Join-Path $EvidenceDir 'task-snapshot.json') -Force
Copy-Item -LiteralPath (Join-Path $RootDir 'agent\mission.json') -Destination (Join-Path $EvidenceDir 'config-snapshot\mission.json') -Force
Copy-Item -LiteralPath (Join-Path $RootDir 'agent\constraints.json') -Destination (Join-Path $EvidenceDir 'config-snapshot\constraints.json') -Force
Copy-Item -LiteralPath (Join-Path $RootDir 'agent\modules.json') -Destination (Join-Path $EvidenceDir 'config-snapshot\modules.json') -Force
Copy-Item -LiteralPath (Join-Path $RootDir 'agent\gates.json') -Destination (Join-Path $EvidenceDir 'config-snapshot\gates.json') -Force

$task = Get-TaskFileObject -TaskFile $TaskFile
$execution = [ordered]@{
  task_id = $task.id
  task_title = $task.title
  task_file = $TaskFile
  collected_at = (Get-AgentNowUtc)
  platform = 'windows'
}
$executionPath = Join-Path $EvidenceDir 'execution-summary.json'
$execution | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $executionPath -Encoding UTF8

$gatePath = Join-Path $EvidenceDir 'gate-results.json'
$gate = $null
if (Test-Path -LiteralPath $gatePath) {
  $gate = Get-Content -LiteralPath $gatePath -Raw -Encoding UTF8 | ConvertFrom-Json
}
$lines = New-Object System.Collections.Generic.List[string]
$lines.Add('# Agent Execution Summary') | Out-Null
$lines.Add('') | Out-Null
$lines.Add("- TaskId: ``$($task.id)``") | Out-Null
$lines.Add("- Title: $($task.title)") | Out-Null
$lines.Add('- Platform: `windows`') | Out-Null
$lines.Add("- CollectedAt(UTC): ``$($execution.collected_at)``") | Out-Null
$lines.Add("- TaskSnapshot: ``$(Join-Path $EvidenceDir 'task-snapshot.json')``") | Out-Null
$lines.Add("- GateResults: ``$gatePath``") | Out-Null
$lines.Add('') | Out-Null
if ($gate) {
  $lines.Add('## Gate Summary') | Out-Null
  $lines.Add('') | Out-Null
  $lines.Add("- Overall: **$($gate.overall)**") | Out-Null
  $lines.Add("- Total: $($gate.summary.total)") | Out-Null
  $lines.Add("- Pass: $($gate.summary.pass)") | Out-Null
  $lines.Add("- Fail: $($gate.summary.fail)") | Out-Null
  $lines.Add('') | Out-Null
}
$summaryPath = Join-Path $EvidenceDir 'summary.md'
Set-Content -LiteralPath $summaryPath -Value $lines -Encoding UTF8
Write-Host $summaryPath
Write-Host $executionPath
