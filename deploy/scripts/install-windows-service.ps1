param(
  [string]$ServiceName = 'SIPTunnelGateway',
  [string]$InstallDir = 'C:\SIPTunnel',
  [string]$BinaryPath = '',
  [string]$ConfigPath = '',
  [string]$DataDir = '',
  [string]$DisplayName = 'SIP Tunnel Gateway',
  [string]$Description = 'SIPTunnel 网关服务',
  [ValidateSet('Automatic','Manual','Disabled')]
  [string]$StartupType = 'Automatic',
  [switch]$StartAfterInstall,
  [switch]$Force
)

$ErrorActionPreference = 'Stop'

function Write-Log { param([string]$Message) Write-Host "[install-windows-service] $Message" }
function Throw-IfMissingPath {
  param([string]$PathValue,[string]$Label)
  if (-not $PathValue -or -not (Test-Path $PathValue)) {
    throw "$Label 不存在: $PathValue"
  }
}

if (-not $BinaryPath) { $BinaryPath = Join-Path $InstallDir 'gateway.exe' }
if (-not $ConfigPath) { $ConfigPath = Join-Path $InstallDir 'configs\config.yaml' }
if (-not $DataDir) { $DataDir = Join-Path $InstallDir 'data' }

Throw-IfMissingPath -PathValue $BinaryPath -Label '二进制'
Throw-IfMissingPath -PathValue $ConfigPath -Label '配置文件'

if (-not (Test-Path $DataDir)) {
  New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
}

$service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($service) {
  if (-not $Force) {
    throw "服务已存在: $ServiceName。若需覆盖，请加 -Force。"
  }
  if ($service.Status -ne 'Stopped') {
    Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
  }
  & sc.exe delete $ServiceName | Out-Null
  Start-Sleep -Seconds 2
  Write-Log "已删除旧服务: $ServiceName"
}

$workDir = Split-Path -Parent $BinaryPath
$bin = '"{0}" --config "{1}"' -f $BinaryPath, $ConfigPath

New-Service -Name $ServiceName -BinaryPathName $bin -DisplayName $DisplayName -Description $Description -StartupType $StartupType | Out-Null

& sc.exe failure $ServiceName reset= 86400 actions= restart/5000/restart/5000/restart/5000 | Out-Null
& sc.exe failureflag $ServiceName 1 | Out-Null
& sc.exe config $ServiceName obj= LocalSystem | Out-Null
& sc.exe sidtype $ServiceName unrestricted | Out-Null

$regPath = "HKLM:\SYSTEM\CurrentControlSet\Services\$ServiceName"
New-ItemProperty -Path $regPath -Name AppDirectory -Value $workDir -PropertyType String -Force | Out-Null
New-ItemProperty -Path $regPath -Name DataDirectory -Value $DataDir -PropertyType String -Force | Out-Null

Write-Log "服务已安装: $ServiceName"
Write-Log "二进制: $BinaryPath"
Write-Log "配置: $ConfigPath"
Write-Log "数据目录: $DataDir"

if ($StartAfterInstall) {
  Start-Service -Name $ServiceName
  Write-Log "服务已启动: $ServiceName"
}
