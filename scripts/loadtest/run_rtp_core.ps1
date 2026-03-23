param(
  [string]$Targets = "rtp-upload",
  [string]$RTPAddress = "127.0.0.1:25000",
  [string]$RTPTransport = "UDP",
  [int]$FileSize = 1048576,
  [int]$ChunkSize = 65536,
  [int]$Concurrency = 32,
  [int]$Qps = 0,
  [string]$Duration = "60s",
  [string]$OutputDir = "./loadtest/results",
  [string]$Timeout = "5s",
  [string]$GatewayBaseUrl = "",
  [string]$DiagInterval = "0s"
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
Push-Location (Join-Path $Root "gateway-server")

$EffectiveChunkSize = $ChunkSize
if (-not $PSBoundParameters.ContainsKey("ChunkSize") -and $RTPTransport.ToUpperInvariant() -eq "UDP") {
  $EffectiveChunkSize = 61440
}
try {
  go run ./cmd/loadtest `
    -targets $Targets `
    -rtp-address $RTPAddress `
    -transfer-mode $RTPTransport `
    -file-size $FileSize `
    -chunk-size $EffectiveChunkSize `
    -concurrency $Concurrency `
    -qps $Qps `
    -duration $Duration `
    -output-dir $OutputDir `
    -timeout $Timeout `
    -gateway-base-url $GatewayBaseUrl `
    -diag-interval $DiagInterval
}
finally {
  Pop-Location
}
