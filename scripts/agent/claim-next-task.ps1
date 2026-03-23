$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')

$RootDir = Get-AgentRootDir
$QueueDir = Join-Path $RootDir 'agent\tasks'
$LockFile = Join-Path $QueueDir '.queue.lock'
if (Test-Path -LiteralPath $LockFile) {
  Write-Error "Queue lock exists: $LockFile"
}

$InProgressDir = Join-Path $QueueDir 'in_progress'
$DoneDir = Join-Path $QueueDir 'done'
Ensure-AgentDir $InProgressDir
$doneIds = @{}
Get-ChildItem -LiteralPath $DoneDir -Filter *.json -File -ErrorAction SilentlyContinue | ForEach-Object {
  $doneIds[$_.BaseName] = $true
}

$existing = @(Get-ChildItem -LiteralPath $InProgressDir -Filter *.json -File -ErrorAction SilentlyContinue | Sort-Object Name)
if ($existing.Count -gt 0) {
  $resume = $existing[0]
  $payload = [ordered]@{
    task_file = (Resolve-Path -Relative $resume.FullName).TrimStart('.','\').Replace('\','/')
    task_id = $resume.BaseName
    locked_at = [DateTime]::UtcNow.ToString('o')
    reason = 'resume-in-progress'
  }
  $payload | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $LockFile -Encoding UTF8
  Write-Output $payload.task_file
  exit 0
}

$priorityRank = @{ P0 = 0; P1 = 1; P2 = 2 }
$candidates = New-Object System.Collections.Generic.List[object]
foreach ($state in @('active', 'backlog')) {
  $dir = Join-Path $QueueDir $state
  if (-not (Test-Path -LiteralPath $dir)) { continue }
  Get-ChildItem -LiteralPath $dir -Filter *.json -File | Sort-Object Name | ForEach-Object {
    $data = Get-Content -LiteralPath $_.FullName -Raw -Encoding UTF8 | ConvertFrom-Json
    $deps = @($data.depends_on)
    $ready = $true
    foreach ($dep in $deps) {
      if (-not $doneIds.ContainsKey([string]$dep)) {
        $ready = $false
        break
      }
    }
    if (-not $ready) { return }
    $pr = if ($priorityRank.ContainsKey([string]$data.priority)) { $priorityRank[[string]$data.priority] } else { 9 }
    $stateRank = if ($state -eq 'active') { 0 } else { 1 }
    $queueOrder = if ($null -ne $data.queue_order) { [int]$data.queue_order } else { 999999 }
    $candidates.Add([pscustomobject]@{
      priority_rank = $pr
      queue_order = $queueOrder
      state_rank = $stateRank
      file_name = $_.Name
      state = $state
      path = $_.FullName
      data = $data
    }) | Out-Null
  }
}

if ($candidates.Count -eq 0) {
  Write-Output ''
  exit 0
}

$chosen = $candidates | Sort-Object priority_rank, queue_order, state_rank, file_name | Select-Object -First 1
$target = Join-Path $InProgressDir ([IO.Path]::GetFileName($chosen.path))
$chosen.data.status = 'in_progress'
$chosen.data | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $target -Encoding UTF8
Remove-Item -LiteralPath $chosen.path -Force
$relative = (Resolve-Path -Relative $target).TrimStart('.','\').Replace('\','/')
([ordered]@{
  task_file = $relative
  task_id = [string]$chosen.data.id
  locked_at = [DateTime]::UtcNow.ToString('o')
  priority = [string]$chosen.data.priority
}) | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $LockFile -Encoding UTF8
Write-Output $relative
