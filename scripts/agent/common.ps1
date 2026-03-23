$ErrorActionPreference = 'Stop'

function Get-AgentRootDir {
  return (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
}

function Get-AgentArtifactRoot {
  $root = Get-AgentRootDir
  if ($env:AGENT_ARTIFACT_ROOT) {
    return $env:AGENT_ARTIFACT_ROOT
  }
  return (Join-Path $root 'artifacts\agent')
}

function Get-AgentTimestamp {
  return (Get-Date).ToString('yyyyMMdd-HHmmss')
}

function Get-AgentNowUtc {
  return [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
}

function Ensure-AgentDir {
  param([string]$Path)
  if (-not (Test-Path -LiteralPath $Path)) {
    New-Item -ItemType Directory -Path $Path -Force | Out-Null
  }
}

function Get-TaskFileObject {
  param([string]$TaskFile)
  return (Get-Content -LiteralPath $TaskFile -Raw -Encoding UTF8 | ConvertFrom-Json)
}
