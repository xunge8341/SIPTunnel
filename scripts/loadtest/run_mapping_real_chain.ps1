param(
  [string]$Targets = "mapping-forward",
  [Parameter(Mandatory = $true)][string]$MappingUrl,
  [string]$MappingMethod = "POST",
  [int]$MappingBodySize = 4096,
  [int]$Concurrency = 64,
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
try {
  go run ./cmd/loadtest `
    -targets $Targets `
    -mapping-url $MappingUrl `
    -mapping-method $MappingMethod `
    -mapping-body-size $MappingBodySize `
    -concurrency $Concurrency `
    -qps $Qps `
    -duration $Duration `
    -output-dir $OutputDir `
    -timeout $Timeout `
    -gateway-base-url $GatewayBaseUrl `
    -diag-interval $DiagInterval `
    -strict-real-mapping
}
finally {
  Pop-Location
}
