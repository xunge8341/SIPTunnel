param(
  [string]$Targets = "mapping-forward",
  [string]$MappingUrl = "http://127.0.0.1:18090/orders",
  [string]$MappingMethod = "POST",
  [int]$MappingBodySize = 4096,
  [int]$Concurrency = 64,
  [int]$Qps = 0,
  [string]$Duration = "60s",
  [string]$OutputDir = "./loadtest/results",
  [string]$Timeout = "5s",
  [string]$GatewayBaseUrl = "",
  [string]$DiagInterval = "0s",
  [bool]$AllowProbePath = $true
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
Push-Location (Join-Path $Root "gateway-server")
try {
  $args = @(
    "./cmd/loadtest",
    "-targets", $Targets,
    "-mapping-url", $MappingUrl,
    "-mapping-method", $MappingMethod,
    "-mapping-body-size", $MappingBodySize,
    "-concurrency", $Concurrency,
    "-qps", $Qps,
    "-duration", $Duration,
    "-output-dir", $OutputDir,
    "-timeout", $Timeout,
    "-gateway-base-url", $GatewayBaseUrl,
    "-diag-interval", $DiagInterval
  )
  if ($AllowProbePath) {
    $args += "-allow-probe-path"
  }
  go run @args
}
finally {
  Pop-Location
}
