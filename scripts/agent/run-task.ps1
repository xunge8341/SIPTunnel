param(
  [Parameter(Mandatory = $true)][string]$TaskFile,
  [string]$Profile,
  [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')

$task = Get-TaskFileObject -TaskFile $TaskFile
$taskId = $task.id
$evidenceDir = Join-Path (Get-AgentArtifactRoot) (Join-Path $taskId (Get-AgentTimestamp))
Ensure-AgentDir $evidenceDir
Write-Host "[INFO] task=$taskId"
Write-Host "[INFO] evidence=$evidenceDir"

$params = @{
  TaskFile = $TaskFile
  EvidenceDir = $evidenceDir
}
if ($Profile) {
  $params.Profile = $Profile
}
if ($DryRun) {
  $params.DryRun = $true
}
& (Join-Path $PSScriptRoot 'run-gates.ps1') @params
& (Join-Path $PSScriptRoot 'collect-evidence.ps1') -TaskFile $TaskFile -EvidenceDir $evidenceDir
Write-Host $evidenceDir
