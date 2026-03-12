$ErrorActionPreference = 'Stop'

$ListenPort = if ($env:LISTEN_PORT) { [int]$env:LISTEN_PORT } elseif ($env:GATEWAY_PORT) { [int]$env:GATEWAY_PORT } else { 18080 }
$MediaPortStart = if ($env:MEDIA_PORT_START) { [int]$env:MEDIA_PORT_START } else { 20000 }
$MediaPortEnd = if ($env:MEDIA_PORT_END) { [int]$env:MEDIA_PORT_END } else { 20100 }
$NodeRole = if ($env:NODE_ROLE) { $env:NODE_ROLE } else { 'receiver' }

function Assert-PortRange {
  param([string]$Name, [int]$Value)
  if ($Value -lt 1 -or $Value -gt 65535) {
    throw "[ERROR] $Name must be in [1,65535], got: $Value"
  }
}

Assert-PortRange 'LISTEN_PORT' $ListenPort
Assert-PortRange 'MEDIA_PORT_START' $MediaPortStart
Assert-PortRange 'MEDIA_PORT_END' $MediaPortEnd

if ($MediaPortStart -gt $MediaPortEnd) {
  throw '[ERROR] MEDIA_PORT_START must <= MEDIA_PORT_END'
}

if ($NodeRole -notin @('receiver', 'sender')) {
  throw "[ERROR] NODE_ROLE must be receiver or sender, got: $NodeRole"
}

$inUse = Get-NetTCPConnection -State Listen -LocalPort $ListenPort -ErrorAction SilentlyContinue
if ($inUse) {
  throw "[ERROR] LISTEN_PORT=$ListenPort is already in use"
}

Write-Host '[OK] preflight passed'
Write-Host "       LISTEN_PORT=$ListenPort"
Write-Host "       MEDIA_PORT_RANGE=$MediaPortStart-$MediaPortEnd"
Write-Host "       NODE_ROLE=$NodeRole"
