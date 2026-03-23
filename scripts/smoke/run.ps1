$ErrorActionPreference = 'Stop'

$RootDir = Resolve-Path (Join-Path $PSScriptRoot '..\..')
$ServerDir = Join-Path $RootDir 'gateway-server'
$BaseConfigPath = if ($env:SMOKE_CONFIG_PATH) { $env:SMOKE_CONFIG_PATH } else { Join-Path $ServerDir 'configs\config.yaml' }
$StartGateway = if ($env:SMOKE_START_GATEWAY) { $env:SMOKE_START_GATEWAY } else { 'true' }
$WaitSeconds = if ($env:SMOKE_WAIT_SECONDS) { [int]$env:SMOKE_WAIT_SECONDS } else { 25 }
$LogFile = if ($env:SMOKE_LOG_FILE) { $env:SMOKE_LOG_FILE } else { Join-Path $RootDir '.smoke-gateway.log' }
$StdoutLogFile = [System.IO.Path]::ChangeExtension($LogFile, '.stdout.log')
$StderrLogFile = [System.IO.Path]::ChangeExtension($LogFile, '.stderr.log')
$SmokeRoot = if ($env:SMOKE_WORK_DIR) { $env:SMOKE_WORK_DIR } else { Join-Path ([System.IO.Path]::GetTempPath()) ("siptunnel-smoke-" + [guid]::NewGuid().ToString('N')) }
$SmokeDataDir = Join-Path $SmokeRoot 'data'
$SmokeFinalDir = Join-Path $SmokeDataDir 'final'
$SmokeNodeConfigPath = Join-Path $SmokeFinalDir 'node_config.json'
$SmokeConfigPath = Join-Path $SmokeRoot 'config.yaml'
$WindowsGatewayBinary = Join-Path $RootDir 'dist\bin\windows\amd64\gateway.exe'
$LinuxGatewayBinary = Join-Path $RootDir 'dist/bin/linux/amd64/gateway'

$gatewayProc = $null
$originalGatewayDataDir = $env:GATEWAY_DATA_DIR

function New-TcpProbeListener {
  $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
  $listener.Server.SetSocketOption([System.Net.Sockets.SocketOptionLevel]::Socket, [System.Net.Sockets.SocketOptionName]::ReuseAddress, $false)
  $listener.Start()
  return $listener
}

function Get-FreeTcpPort {
  $listener = New-TcpProbeListener
  try {
    return ([System.Net.IPEndPoint]$listener.LocalEndpoint).Port
  } finally {
    $listener.Stop()
  }
}

function Test-UdpPortFree {
  param([int]$Port)
  $udp = $null
  try {
    $udp = [System.Net.Sockets.UdpClient]::new($Port)
    return $true
  } catch {
    return $false
  } finally {
    if ($udp) {
      $udp.Close()
      $udp.Dispose()
    }
  }
}

function Get-FreeUdpPortRange {
  param([int]$Length = 102, [int]$Start = 30000, [int]$End = 60000)
  for ($candidate = $Start; $candidate -le ($End - $Length); $candidate += 103) {
    $ok = $true
    for ($port = $candidate; $port -lt ($candidate + $Length); $port++) {
      if (-not (Test-UdpPortFree -Port $port)) {
        $ok = $false
        break
      }
    }
    if ($ok) {
      return $candidate
    }
  }
  throw "unable to allocate free UDP port range of length $Length"
}

function New-SmokeConfig {
  param(
    [string]$SourcePath,
    [string]$TargetPath,
    [int]$HttpPort,
    [int]$SipPort,
    [int]$RtpStart,
    [int]$RtpEnd
  )
  $lines = [System.Collections.Generic.List[string]]::new()
  $lines.AddRange([string[]](Get-Content -LiteralPath $SourcePath -Encoding UTF8))
  $section = ''
  for ($i = 0; $i -lt $lines.Count; $i++) {
    $line = $lines[$i]
    $topLevel = [regex]::Match($line, '^(?<key>[A-Za-z0-9_]+):\s*$')
    if ($topLevel.Success) {
      $section = $topLevel.Groups['key'].Value
      continue
    }
    if ($line -match '^\S') {
      $section = ''
    }
    switch ($section) {
      'ui' {
        if ($line -match '^\s{2}listen_port:\s*') {
          $lines[$i] = "  listen_port: $HttpPort"
        }
      }
      'sip' {
        if ($line -match '^\s{2}listen_port:\s*') {
          $lines[$i] = "  listen_port: $SipPort"
        }
      }
      'rtp' {
        if ($line -match '^\s{2}port_start:\s*') {
          $lines[$i] = "  port_start: $RtpStart"
        } elseif ($line -match '^\s{2}port_end:\s*') {
          $lines[$i] = "  port_end: $RtpEnd"
        }
      }
    }
  }
  $dir = Split-Path -Parent $TargetPath
  if (-not (Test-Path -LiteralPath $dir)) {
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
  }
  $content = [string]::Join([Environment]::NewLine, $lines)
  Set-Content -LiteralPath $TargetPath -Value $content -Encoding UTF8
}


function New-SmokeNodeConfig {
  param(
    [string]$TargetPath,
    [int]$SipPort,
    [int]$RtpStart,
    [int]$RtpEnd
  )
  $payload = @{
    local_node = @{
      node_id = 'gateway-a-01'
      node_name = 'Smoke Gateway'
      node_role = 'gateway'
      network_mode = 'SENDER_SIP__RECEIVER_RTP'
      sip_listen_ip = '0.0.0.0'
      sip_listen_port = $SipPort
      sip_transport = 'TCP'
      rtp_listen_ip = '0.0.0.0'
      rtp_port_start = $RtpStart
      rtp_port_end = $RtpEnd
      rtp_transport = 'UDP'
    }
    peers = @()
  }
  $dir = Split-Path -Parent $TargetPath
  if (-not (Test-Path -LiteralPath $dir)) {
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
  }
  $json = $payload | ConvertTo-Json -Depth 6
  Set-Content -LiteralPath $TargetPath -Value $json -Encoding UTF8
}


function Assert-SmokeConfig {
  param(
    [string]$ConfigPath,
    [int]$HttpPort,
    [int]$SipPort,
    [int]$RtpStart,
    [int]$RtpEnd
  )
  $content = Get-Content -LiteralPath $ConfigPath -Raw -Encoding UTF8
  $checks = @(
    @{ Pattern = "(?m)^  listen_port: $HttpPort\\s*$"; Label = 'ui.listen_port' },
    @{ Pattern = "(?m)^  listen_port: $SipPort\\s*$"; Label = 'sip.listen_port' },
    @{ Pattern = "(?m)^  port_start: $RtpStart\\s*$"; Label = 'rtp.port_start' },
    @{ Pattern = "(?m)^  port_end: $RtpEnd\\s*$"; Label = 'rtp.port_end' }
  )
  foreach ($check in $checks) {
    if (-not [regex]::IsMatch($content, $check.Pattern)) {
      throw "[smoke] generated config missing expected $($check.Label), config=$ConfigPath"
    }
  }
}

function Resolve-GatewayCommand {
  if ($env:SMOKE_GATEWAY_CMD) {
    return [pscustomobject]@{ FilePath = $env:SMOKE_GATEWAY_CMD; ArgumentPrefix = @() ; Source = "env" }
  }
  $isWindowsHost = ($env:OS -eq 'Windows_NT')
  if ($isWindowsHost -and (Test-Path -LiteralPath $WindowsGatewayBinary)) {
    return [pscustomobject]@{ FilePath = $WindowsGatewayBinary; ArgumentPrefix = @() ; Source = "dist" }
  }
  if ((-not $isWindowsHost) -and (Test-Path -LiteralPath $LinuxGatewayBinary)) {
    return [pscustomobject]@{ FilePath = $LinuxGatewayBinary; ArgumentPrefix = @() ; Source = "dist" }
  }
  return [pscustomobject]@{ FilePath = 'go'; ArgumentPrefix = @('run', './cmd/gateway'); Source = "go-run" }
}

function Invoke-GoCommand {
  param(
    [string[]]$ArgumentList,
    [string]$FailurePrefix
  )
  $stdoutPath = Join-Path $SmokeRoot ('go-' + [guid]::NewGuid().ToString('N') + '.stdout.log')
  $stderrPath = Join-Path $SmokeRoot ('go-' + [guid]::NewGuid().ToString('N') + '.stderr.log')
  Push-Location $ServerDir
  try {
    $proc = Start-Process -FilePath 'go' -ArgumentList $ArgumentList -WorkingDirectory $ServerDir -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath -PassThru -Wait
    $stdout = @()
    $stderr = @()
    if (Test-Path -LiteralPath $stdoutPath) {
      $stdout = @(Get-Content -LiteralPath $stdoutPath -Encoding UTF8)
    }
    if (Test-Path -LiteralPath $stderrPath) {
      $stderr = @(Get-Content -LiteralPath $stderrPath -Encoding UTF8)
    }
    $combined = @($stdout + $stderr | Where-Object { $_ -ne $null -and $_ -ne '' })
    $detail = (($combined | Out-String).Trim())
    return [pscustomobject]@{
      ExitCode = $proc.ExitCode
      Output = $combined
      Failed = ($proc.ExitCode -ne 0)
      Message = if ($proc.ExitCode -ne 0) {
        if ([string]::IsNullOrWhiteSpace($detail)) { 'unknown error' } else { $detail }
      } else {
        ''
      }
    }
  } finally {
    Pop-Location
    Remove-Item -LiteralPath $stdoutPath -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $stderrPath -Force -ErrorAction SilentlyContinue
  }
}

function Get-GoModCompatVersion {
  if ($env:GO_MOD_TIDY_COMPAT) {
    return "$($env:GO_MOD_TIDY_COMPAT)"
  }

  $goModPath = Join-Path $RootDir 'gateway-server/go.mod'
  if (Test-Path $goModPath) {
    $goDirective = Select-String -Path $goModPath -Pattern '^go\s+([0-9]+\.[0-9]+(?:\.[0-9]+)?)\s*$' | Select-Object -First 1
    if ($goDirective -and $goDirective.Matches.Count -gt 0) {
      $value = $goDirective.Matches[0].Groups[1].Value
      if (-not [string]::IsNullOrWhiteSpace($value)) {
        return $value
      }
    }
  }

  return '1.23.0'
}

function Ensure-GoModuleReady {
  $compatVersion = Get-GoModCompatVersion
  $tidy = Invoke-GoCommand -ArgumentList @('mod', 'tidy', "-compat=$compatVersion") -FailurePrefix '[smoke] go mod tidy failed'
  if ($tidy.Failed) {
    throw "[smoke] go mod tidy failed: $($tidy.Message)"
  }
}

function Test-SmokeConfig {
  param(
    [string]$ConfigPath,
    [pscustomobject]$GatewayCommand
  )
  if ($GatewayCommand.Source -eq 'go-run') {
    $result = Invoke-GoCommand -ArgumentList @($GatewayCommand.ArgumentPrefix + @('validate-config', '-f', $ConfigPath)) -FailurePrefix '[smoke] generated config validation failed'
    if ($result.Failed) {
      throw "[smoke] generated config validation failed: $($result.Message)"
    }
    return
  }
  $stdoutPath = Join-Path $SmokeRoot ('gateway-validate-' + [guid]::NewGuid().ToString('N') + '.stdout.log')
  $stderrPath = Join-Path $SmokeRoot ('gateway-validate-' + [guid]::NewGuid().ToString('N') + '.stderr.log')
  try {
    $proc = Start-Process -FilePath $GatewayCommand.FilePath -ArgumentList @('validate-config', '-f', $ConfigPath) -WorkingDirectory $ServerDir -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath -PassThru -Wait
    if ($proc.ExitCode -ne 0) {
      $detail = @()
      if (Test-Path -LiteralPath $stdoutPath) { $detail += Get-Content -LiteralPath $stdoutPath -Encoding UTF8 }
      if (Test-Path -LiteralPath $stderrPath) { $detail += Get-Content -LiteralPath $stderrPath -Encoding UTF8 }
      $msg = (($detail | Where-Object { $_ } | Out-String).Trim())
      if ([string]::IsNullOrWhiteSpace($msg)) { $msg = 'unknown error' }
      throw "[smoke] generated config validation failed: $msg"
    }
  } finally {
    Remove-Item -LiteralPath $stdoutPath -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $stderrPath -Force -ErrorAction SilentlyContinue
  }
}

function Wait-GatewayReady {
  param([string]$Url, [int]$TimeoutSeconds, [System.Diagnostics.Process]$Process)
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ((Get-Date) -lt $deadline) {
    if ($Process -and $Process.HasExited) {
      throw "[smoke] gateway exited early with code $($Process.ExitCode), stdout=$StdoutLogFile stderr=$StderrLogFile"
    }
    try {
      $healthz = Invoke-RestMethod -Uri "$Url/healthz" -TimeoutSec 2
      if ($healthz.data.status -eq 'ok') {
        $readyz = Invoke-RestMethod -Uri "$Url/readyz" -TimeoutSec 2
        if ($readyz.data.status -eq 'ready') {
          return $true
        }
      }
    } catch {
      Start-Sleep -Seconds 1
    }
  }
  return $false
}

try {
  $GatewayPort = if ($env:GATEWAY_PORT) { [int]$env:GATEWAY_PORT } else { Get-FreeTcpPort }
  $SipPort = if ($env:SMOKE_SIP_PORT) { [int]$env:SMOKE_SIP_PORT } else { Get-FreeTcpPort }
  $RtpStart = if ($env:SMOKE_RTP_START) { [int]$env:SMOKE_RTP_START } else { Get-FreeUdpPortRange }
  $RtpEnd = $RtpStart + 101
  $BaseUrl = if ($env:SMOKE_BASE_URL) { $env:SMOKE_BASE_URL } else { "http://127.0.0.1:$GatewayPort" }

  New-SmokeConfig -SourcePath $BaseConfigPath -TargetPath $SmokeConfigPath -HttpPort $GatewayPort -SipPort $SipPort -RtpStart $RtpStart -RtpEnd $RtpEnd
  Assert-SmokeConfig -ConfigPath $SmokeConfigPath -HttpPort $GatewayPort -SipPort $SipPort -RtpStart $RtpStart -RtpEnd $RtpEnd
  New-SmokeNodeConfig -TargetPath $SmokeNodeConfigPath -SipPort $SipPort -RtpStart $RtpStart -RtpEnd $RtpEnd
  Ensure-GoModuleReady
  $gatewayCommand = Resolve-GatewayCommand
  Test-SmokeConfig -ConfigPath $SmokeConfigPath -GatewayCommand $gatewayCommand
  $env:GATEWAY_DATA_DIR = $SmokeDataDir

  if ($StartGateway.ToLower() -eq 'true') {
    $effectiveWaitSeconds = $WaitSeconds
    if ($gatewayCommand.Source -eq 'go-run' -and $effectiveWaitSeconds -lt 45) {
      $effectiveWaitSeconds = 45
    }
    Write-Host '[smoke] starting gateway-server for smoke test...'
    Write-Host "[smoke] config: $SmokeConfigPath"
    Write-Host "[smoke] data dir: $SmokeDataDir"
    Write-Host "[smoke] base url: $BaseUrl"
    Write-Host "[smoke] gateway command source: $($gatewayCommand.Source)"
    Write-Host "[smoke] stdout log: $StdoutLogFile"
    Write-Host "[smoke] stderr log: $StderrLogFile"
    $gatewayArgs = @($gatewayCommand.ArgumentPrefix + @('--config', $SmokeConfigPath))
    $gatewayProc = Start-Process -FilePath $gatewayCommand.FilePath -ArgumentList $gatewayArgs -WorkingDirectory $ServerDir -RedirectStandardOutput $StdoutLogFile -RedirectStandardError $StderrLogFile -PassThru
    if (-not (Wait-GatewayReady -Url $BaseUrl -TimeoutSeconds $effectiveWaitSeconds -Process $gatewayProc)) {
      throw "[smoke] gateway start timeout or readyz check failed, stdout=$StdoutLogFile stderr=$StderrLogFile"
    }
  }

  $smokeResult = Invoke-GoCommand -ArgumentList @('run', './cmd/opssmoke', '--base-url', $BaseUrl, '--config', $SmokeConfigPath) -FailurePrefix '[smoke] opssmoke failed'
  if ($smokeResult.Failed) {
    throw "[smoke] opssmoke failed: $($smokeResult.Message)"
  }
  foreach ($line in $smokeResult.Output) {
    Write-Host $line
  }
} finally {
  if ($gatewayProc -and -not $gatewayProc.HasExited) {
    Stop-Process -Id $gatewayProc.Id -Force -ErrorAction SilentlyContinue
  }
  if ($null -ne $originalGatewayDataDir) {
    $env:GATEWAY_DATA_DIR = $originalGatewayDataDir
  } else {
    Remove-Item Env:\GATEWAY_DATA_DIR -ErrorAction SilentlyContinue
  }
}
