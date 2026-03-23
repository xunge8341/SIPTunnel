$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')

Invoke-CiGate -Name 'unit_tests' -Action {
  Push-Location (Join-Path $RootDir 'gateway-server')
  try { go test ./... -short -count=1 } finally { Pop-Location }
}

Invoke-CiGate -Name 'protocol_codec_tests' -Action {
  Push-Location (Join-Path $RootDir 'gateway-server')
  try { go test ./internal/protocol/sip ./internal/protocol/rtpfile ./internal/control -count=1 } finally { Pop-Location }
}

Invoke-CiGate -Name 'e2e_smoke' -Action {
  & (Join-Path $RootDir 'scripts\smoke\run.ps1')
}

Invoke-CiGate -Name 'repo_consistency' -Action {
  & (Join-Path $RootDir 'scripts\check-consistency.ps1')
}

Invoke-CiGate -Name 'ui_quality' -Action {
  Push-Location (Join-Path $RootDir 'gateway-ui')
  try {
    npm run lint
    npm run typecheck
    npm run test
    npm run build
  } finally { Pop-Location }
}

Invoke-CiGate -Name 'network_config_validation' -Action {
  Push-Location (Join-Path $RootDir 'gateway-server')
  try {
    go test ./internal/config ./internal/selfcheck -run 'Test(ParseNetworkConfigYAML|ConfigYAMLSample_NetworkSectionValid|SIPConfigUDPMessageSizeRisk|RunnerRun_AllPass|RunnerRun_RTPTCPReservedWarn)' -count=1
  } finally { Pop-Location }
  $env:LISTEN_PORT = '18080'
  $env:MEDIA_PORT_START = '20000'
  $env:MEDIA_PORT_END = '20100'
  $env:NODE_ROLE = 'receiver'
  & (Join-Path $RootDir 'scripts\preflight.ps1')
}

Invoke-CiGate -Name 'benchmark_smoke' -Action {
  Push-Location (Join-Path $RootDir 'gateway-server')
  try {
    $env:GOMAXPROCS = '1'
    go test ./internal/protocol/sip ./internal/security ./internal/protocol/rtpfile ./internal/service/httpinvoke -run '^$' -bench 'Benchmark(SIPJSONDecodeValidate|Signer(Sign|Verify)|File(Split|Assemble)|HTTP(MapByTemplate|InvokeWrapper))$' -benchmem -benchtime=100ms -count=1
  } finally { Pop-Location }
}

Write-CiReport -Suite 'ci-quality-gates'
