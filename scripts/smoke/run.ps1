$ErrorActionPreference = 'Stop'

$RootDir = Resolve-Path (Join-Path $PSScriptRoot '..\..')
$ServerDir = Join-Path $RootDir 'gateway-server'
$ConfigPath = if ($env:SMOKE_CONFIG_PATH) { $env:SMOKE_CONFIG_PATH } else { Join-Path $ServerDir 'configs\config.yaml' }
$GatewayPort = if ($env:GATEWAY_PORT) { $env:GATEWAY_PORT } else { '18080' }
$BaseUrl = if ($env:SMOKE_BASE_URL) { $env:SMOKE_BASE_URL } else { "http://127.0.0.1:$GatewayPort" }
$StartGateway = if ($env:SMOKE_START_GATEWAY) { $env:SMOKE_START_GATEWAY } else { 'true' }
$WaitSeconds = if ($env:SMOKE_WAIT_SECONDS) { [int]$env:SMOKE_WAIT_SECONDS } else { 25 }
$LogFile = if ($env:SMOKE_LOG_FILE) { $env:SMOKE_LOG_FILE } else { Join-Path $RootDir '.smoke-gateway.log' }

$gatewayProc = $null

function Wait-Healthz {
  param([string]$Url, [int]$TimeoutSeconds)
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ((Get-Date) -lt $deadline) {
    try {
      $resp = Invoke-WebRequest -Uri "$Url/healthz" -UseBasicParsing -TimeoutSec 2
      if ($resp.StatusCode -eq 200) {
        return $true
      }
    } catch {
      Start-Sleep -Seconds 1
    }
  }
  return $false
}

try {
  if ($StartGateway.ToLower() -eq 'true') {
    Write-Host '[smoke] starting gateway-server for smoke test...'
    $gatewayProc = Start-Process -FilePath 'go' -ArgumentList @('run', './cmd/gateway', '--config', $ConfigPath) -WorkingDirectory $ServerDir -RedirectStandardOutput $LogFile -RedirectStandardError $LogFile -PassThru
    if (-not (Wait-Healthz -Url $BaseUrl -TimeoutSeconds $WaitSeconds)) {
      throw "[smoke] gateway start timeout, log=$LogFile"
    }
  }

  Push-Location $ServerDir
  try {
    & go run ./cmd/opssmoke --base-url $BaseUrl --config $ConfigPath
  } finally {
    Pop-Location
  }
} finally {
  if ($gatewayProc -and -not $gatewayProc.HasExited) {
    Stop-Process -Id $gatewayProc.Id -Force -ErrorAction SilentlyContinue
  }
}
