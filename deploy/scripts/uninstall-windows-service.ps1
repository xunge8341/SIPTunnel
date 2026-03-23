param(
  [string]$ServiceName = 'SIPTunnelGateway'
)

$ErrorActionPreference = 'Stop'

function Write-Log { param([string]$Message) Write-Host "[uninstall-windows-service] $Message" }

$service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if (-not $service) {
  Write-Log "服务不存在: $ServiceName"
  exit 0
}

if ($service.Status -ne 'Stopped') {
  Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
}
& sc.exe delete $ServiceName | Out-Null
Write-Log "服务已删除: $ServiceName"
