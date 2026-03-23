$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')

$RootDir = Get-AgentRootDir
$claimed = & (Join-Path $PSScriptRoot 'claim-next-task.ps1')
$claimed = ([string]$claimed).Trim()
if ([string]::IsNullOrWhiteSpace($claimed)) {
  Write-Host '[INFO] no eligible task claimed'
  exit 0
}
$taskPath = Join-Path $RootDir $claimed
$evidenceDir = & (Join-Path $PSScriptRoot 'run-task.ps1') -TaskFile $taskPath
$evidenceDir = ([string]$evidenceDir).Trim().Split([Environment]::NewLine)[-1]
& (Join-Path $PSScriptRoot 'finalize-task.ps1') -TaskFile $taskPath -EvidenceDir $evidenceDir | Out-Null
Write-Output $claimed
Write-Output $evidenceDir
