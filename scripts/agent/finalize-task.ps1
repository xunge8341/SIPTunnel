param(
  [Parameter(Mandatory = $true)][string]$TaskFile,
  [Parameter(Mandatory = $true)][string]$EvidenceDir
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')

$RootDir = Get-AgentRootDir
$QueueDir = Join-Path $RootDir 'agent\tasks'
$LockFile = Join-Path $QueueDir '.queue.lock'
$taskPath = if ([System.IO.Path]::IsPathRooted($TaskFile)) { $TaskFile } else { Join-Path $RootDir $TaskFile }
$report = Get-Content -LiteralPath (Join-Path $EvidenceDir 'gate-results.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$overall = [string]$report.overall
$destDir = Join-Path $QueueDir ($(if ($overall -eq 'PASS') { 'done' } else { 'blocked' }))
Ensure-AgentDir $destDir

$data = Get-Content -LiteralPath $taskPath -Raw -Encoding UTF8 | ConvertFrom-Json
$data.status = $(if ($overall -eq 'PASS') { 'done' } else { 'blocked' })
$data | Add-Member -NotePropertyName last_execution -NotePropertyValue ([ordered]@{
  overall = $overall
  evidence_dir = (Resolve-Path -Relative $EvidenceDir).TrimStart('.','\').Replace('\','/')
  finalized_at = [DateTime]::UtcNow.ToString('o')
}) -Force
$destPath = Join-Path $destDir ([IO.Path]::GetFileName($taskPath))
$data | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $destPath -Encoding UTF8
if ((Resolve-Path $taskPath).Path -ne (Resolve-Path $destPath).Path) {
  Remove-Item -LiteralPath $taskPath -Force
}

$summary = [ordered]@{
  task_id = [string]$data.id
  final_status = [string]$data.status
  task_file = (Resolve-Path -Relative $destPath).TrimStart('.','\').Replace('\','/')
  evidence_dir = (Resolve-Path -Relative $EvidenceDir).TrimStart('.','\').Replace('\','/')
  overall = $overall
  finalized_at = [string]$data.last_execution.finalized_at
}
$summary | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $EvidenceDir 'finalize-summary.json') -Encoding UTF8
$lines = @(
  '# Agent Finalize Summary',
  '',
  "- TaskId: ``$($summary.task_id)``",
  "- FinalStatus: **$($summary.final_status)**",
  "- Overall: **$($summary.overall)**",
  "- TaskFile: ``$($summary.task_file)``",
  "- EvidenceDir: ``$($summary.evidence_dir)``",
  "- FinalizedAt(UTC): ``$($summary.finalized_at)``",
  ''
)
Set-Content -LiteralPath (Join-Path $EvidenceDir 'finalize-summary.md') -Value $lines -Encoding UTF8
if (Test-Path -LiteralPath $LockFile) {
  Remove-Item -LiteralPath $LockFile -Force
}
Write-Output $summary.task_file
