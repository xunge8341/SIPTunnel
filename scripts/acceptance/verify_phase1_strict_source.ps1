param(
  [string]$RootDir = $(Split-Path -Parent (Split-Path -Parent $PSScriptRoot)),
  [string]$ReportDir = $(Join-Path (Split-Path -Parent (Split-Path -Parent $PSScriptRoot)) 'artifacts/acceptance')
)

$ErrorActionPreference = 'Stop'
$null = New-Item -ItemType Directory -Force -Path $ReportDir
$Timestamp = (Get-Date).ToString('yyyyMMdd-HHmmss')
$ReportBase = "phase1-strict-source-$Timestamp"
$ReportJson = Join-Path $ReportDir "$ReportBase.json"
$ReportMd = Join-Path $ReportDir "$ReportBase.md"

function Read-Text {
  param([string]$Path)
  if (Test-Path $Path) {
    return [string]([System.IO.File]::ReadAllText($Path))
  }
  return ''
}

function Has-Pattern {
  param([string]$Text, [string]$Pattern)
  return [regex]::IsMatch(([string]$Text), $Pattern, [System.Text.RegularExpressions.RegexOptions]::Multiline)
}

$serverText = Read-Text (Join-Path $RootDir 'gateway-server/internal/server/gb28181_tunnel.go')
$xmlText = Read-Text (Join-Path $RootDir 'gateway-server/internal/protocol/manscdp/xml.go')
$httpText = Read-Text (Join-Path $RootDir 'gateway-server/internal/server/http.go')
$mainText = Read-Text (Join-Path $RootDir 'gateway-server/cmd/gateway/main.go')
$mainStartupText = Read-Text (Join-Path $RootDir 'gateway-server/cmd/gateway/main_startup.go')
$startupTransportSummaryText = Read-Text (Join-Path $RootDir 'gateway-server/cmd/gateway/startup_transport_summary.go')
$uiTypesText = Read-Text (Join-Path $RootDir 'gateway-ui/src/types/gateway.ts')
$uiTunnelText = Read-Text (Join-Path $RootDir 'gateway-ui/src/views/TunnelConfigView.vue')
$uiNodesText = Read-Text (Join-Path $RootDir 'gateway-ui/src/views/NodesAndTunnelsView.vue')
$mockText = Read-Text (Join-Path $RootDir 'gateway-ui/src/api/mockGateway.ts')
$guardScriptText = Read-Text (Join-Path $RootDir 'scripts/ui-delivery-guard.mjs')
$uiBuildScriptText = Read-Text (Join-Path $RootDir 'scripts/ui-build.ps1')
$embedScriptText = Read-Text (Join-Path $RootDir 'scripts/embed-ui.ps1')
$acceptanceScriptText = Read-Text (Join-Path $RootDir 'scripts/acceptance/run_phase1_strict_acceptance.ps1')
$systemSettingsBackendText = (Read-Text (Join-Path $RootDir 'gateway-server/internal/server/http.go')) + "`n" + (Read-Text (Join-Path $RootDir 'gateway-server/internal/server/ops_system_settings_http.go'))
$systemSettingsUiText = (Read-Text (Join-Path $RootDir 'gateway-ui/src/types/gateway.ts')) + "`n" + (Read-Text (Join-Path $RootDir 'gateway-ui/src/api/gateway.ts')) + "`n" + (Read-Text (Join-Path $RootDir 'gateway-ui/src/views/SystemSettingsView.vue'))
$resourceUsageBackendText = (Read-Text (Join-Path $RootDir 'gateway-server/internal/server/ops_resource_usage_http.go')) + "`n" + (Read-Text (Join-Path $RootDir 'gateway-server/internal/server/ops_settings_logs.go'))
$resourceUsageUiText = (Read-Text (Join-Path $RootDir 'gateway-ui/src/types/gateway.ts')) + "`n" + (Read-Text (Join-Path $RootDir 'gateway-ui/src/views/SystemSettingsView.vue')) + "`n" + (Read-Text (Join-Path $RootDir 'gateway-ui/src/views/AlertsAndRateLimitView.vue'))

$checks = New-Object System.Collections.Generic.List[object]
function Add-Check {
  param([string]$Name, [bool]$Passed, [string]$Detail)
  $checks.Add([pscustomobject]@{
    name   = $Name
    passed = $Passed
    detail = $Detail
  }) | Out-Null
}

Add-Check 'MESSAGE is the control-plane entry point' ($serverText.Contains('case "MESSAGE":')) 'Expect a MESSAGE route branch in gb28181_tunnel.go.'

$hasSubscribeContentType = $serverText.Contains('SetHeader("Content-Type", manscdp.ContentType)')
$hasCatalogQueryBuilder = $serverText.Contains('BuildCatalogQuery') -or $xmlText.Contains('BuildCatalogQuery')
Add-Check 'Catalog SUBSCRIBE carries MANSCDP body' ($hasSubscribeContentType -and $hasCatalogQueryBuilder) 'Expect SUBSCRIBE to carry both Content-Type and XML body.'

$hasGbDate = $serverText.Contains('2006-01-02T15:04:05.000') -or $serverText.Contains('formatGB28181Date')
Add-Check 'REGISTER 200 OK Date uses GB28181 format' $hasGbDate 'Expect yyyy-MM-dd''T''HH:mm:ss.SSS formatting.'

$hasStrictSdp = $xmlText.Contains('m=video ') -and $xmlText.Contains('a=rtpmap:96 PS/90000')
Add-Check 'INVITE/SDP uses video + PS/90000' $hasStrictSdp 'Expect SDP to use m=video and a=rtpmap:96 PS/90000.'

$hasAllow = $serverText.Contains('Allow') -and $serverText.Contains('MESSAGE') -and $serverText.Contains('SUBSCRIBE') -and $serverText.Contains('NOTIFY')
Add-Check 'Allow header is complete' $hasAllow 'Expect Allow to include MESSAGE, SUBSCRIBE, and NOTIFY.'

$writeOnlySecret = $httpText.Contains('register_auth_password_configured') -and $uiTypesText.Contains('register_auth_password_configured')
Add-Check 'Secret is write-only in backend and UI types' $writeOnlySecret 'Expect register_auth_password_configured in backend and UI types.'

$strictUi = $mockText.Contains('register_auth_password_configured') -and ($uiTunnelText.Contains('strict') -or $uiTunnelText.Contains('严格模式') -or $uiNodesText.Contains('strict') -or $uiNodesText.Contains('严格模式'))
Add-Check 'Mock and UI mention strict mode' $strictUi 'Expect mock and UI to be aligned with strict mode terminology.'

$noLegacyServerPort = -not $mainText.Contains('server.port')
Add-Check 'No legacy server.port compatibility remains' $noLegacyServerPort 'Expect runtime config to use only ui.listen_port.'

$startupHelperOk = (-not $mainStartupText.Contains('convergedGeneric')) -and (-not $mainStartupText.Contains('responseModePolicy :=')) -and $startupTransportSummaryText.Contains('func effectiveTransportTuningSummary(') -and $startupTransportSummaryText.Contains('func logAppliedTransportTuning(')
Add-Check 'Startup transport summary helper owns converged generic facts' $startupHelperOk 'Expect converged generic/startup-summary helpers to stay in startup_transport_summary.go so main_startup.go no longer drifts into undefined or unused locals.'

$opsProfileNeedles = @(
  'generic_download_total_mbps',
  'generic_download_per_transfer_mbps',
  'generic_download_window_mb',
  'adaptive_hot_cache_mb',
  'adaptive_hot_window_mb',
  'generic_download_segment_concurrency',
  'generic_download_rtp_reorder_window_packets',
  'generic_download_rtp_loss_tolerance_packets',
  'generic_download_rtp_gap_timeout_ms',
  'generic_download_rtp_fec_enabled',
  'generic_download_rtp_fec_group_packets'
)
$opsProfileMissing = $opsProfileNeedles | Where-Object { -not $systemSettingsBackendText.Contains($_) -or -not $systemSettingsUiText.Contains($_) }
Add-Check 'System settings human-unit ops profile stays aligned across backend/UI' ($opsProfileMissing.Count -eq 0) ('Missing: ' + ($(if ($opsProfileMissing.Count -gt 0) { $opsProfileMissing -join ', ' } else { 'none' })))

$resourceNeedles = @(
  'configured_generic_download_mbps',
  'configured_generic_per_transfer_mbps',
  'configured_adaptive_hot_cache_mb',
  'configured_adaptive_hot_window_mb',
  'configured_generic_download_window_mb',
  'configured_generic_segment_concurrency',
  'configured_generic_rtp_reorder_window_packets',
  'configured_generic_rtp_loss_tolerance_packets',
  'configured_generic_rtp_gap_timeout_ms',
  'configured_generic_rtp_fec_enabled',
  'configured_generic_rtp_fec_group_packets'
)
$resourceMissing = $resourceNeedles | Where-Object { -not $resourceUsageBackendText.Contains($_) -or -not $resourceUsageUiText.Contains($_) }
Add-Check 'Resource usage facts stay aligned across backend and ops UI' ($resourceMissing.Count -eq 0) ('Missing: ' + ($(if ($resourceMissing.Count -gt 0) { $resourceMissing -join ', ' } else { 'none' })))

$legacyHits = New-Object System.Collections.Generic.List[string]
$scanTargets = @(
  (Join-Path $RootDir 'gateway-server/internal'),
  (Join-Path $RootDir 'gateway-ui/src'),
  (Join-Path $RootDir 'README.md'),
  (Join-Path $RootDir 'docs/GB28181_PHASE1_STRICT_MODE_20260318.md')
)
foreach ($target in $scanTargets) {
  if (-not (Test-Path $target)) { continue }
  $item = Get-Item $target
  $files = @()
  if ($item -is [System.IO.FileInfo]) {
    $files = @($target)
  } else {
    $files = Get-ChildItem -Path $target -Recurse -File | Select-Object -ExpandProperty FullName
  }
  foreach ($file in $files) {
    $text = Read-Text $file
    if (Has-Pattern $text 'HttpInvoke|HttpResponseStart|HttpResponseInline') {
      $resolved = Resolve-Path $file
      $pathText = $resolved.Path
      if ($pathText.StartsWith($RootDir, [System.StringComparison]::OrdinalIgnoreCase)) {
        $pathText = $pathText.Substring($RootDir.Length).TrimStart('\\','/')
      }
      $legacyHits.Add($pathText) | Out-Null
    }
  }
}
$legacyDetail = 'none'
if ($legacyHits.Count -gt 0) {
  $legacyDetail = ($legacyHits | Select-Object -Unique) -join ', '
}
Add-Check 'No legacy HttpInvoke style names remain in active code/UI/current docs' ($legacyHits.Count -eq 0) ("Hits: $legacyDetail")

$infoHits = New-Object System.Collections.Generic.List[string]
if (Has-Pattern $serverText 'NewRequest\("INFO"') {
  $infoHits.Add('NewRequest("INFO")') | Out-Null
}
if (Has-Pattern $serverText 'case\s+"INFO"\s*:') {
  $infoHits.Add('case "INFO":') | Out-Null
}
$infoDetail = 'none'
if ($infoHits.Count -gt 0) {
  $infoDetail = ($infoHits | Select-Object -Unique) -join ', '
}
Add-Check 'No INFO request path remains in source' ($infoHits.Count -eq 0) ("Hits: $infoDetail")

$legacyUiFiles = @(
  'gateway-ui/src/views/ConfigGovernanceView.vue',
  'gateway-ui/src/views/ConfigTransferView.vue',
  'gateway-ui/src/views/NetworkConfigView.vue',
  'gateway-ui/src/views/NodeStatusView.vue',
  'gateway-ui/src/views/__tests__/NodeStatusView.spec.ts',
  'gateway-ui/src/api/__tests__/gatewayConfigTransfer.spec.ts'
)
$legacyUiHits = $legacyUiFiles | Where-Object { Test-Path (Join-Path $RootDir $_) }
Add-Check 'No legacy cleared UI files remain in source tree' ($legacyUiHits.Count -eq 0) ('Hits: ' + ($(if ($legacyUiHits.Count -gt 0) { $legacyUiHits -join ', ' } else { 'none' })))

$guardIntegrated = ((Read-Text (Join-Path $RootDir 'scripts/ui-delivery-guard.mjs')).Length -gt 0) -and $uiBuildScriptText.Contains('ui-delivery-guard.mjs') -and $embedScriptText.Contains('delivery_guard_status')
Add-Check 'UI delivery guard is wired into build and embed scripts' $guardIntegrated 'Expect scripts/ui-delivery-guard.mjs plus build/embed script integration.'

$serverTargetedAligned = $acceptanceScriptText.Contains("Invoke-Step 'server_targeted'") -and $acceptanceScriptText.Contains('Sync-GoModuleGraph') -and $acceptanceScriptText.Contains('go mod tidy') -and $acceptanceScriptText.Contains('go test ./internal/protocol/... ./internal/server ./internal/selfcheck ./internal/config -count=1')
Add-Check 'Windows strict acceptance syncs go.mod/go.sum then runs server_targeted before release build' $serverTargetedAligned 'Expect run_phase1_strict_acceptance.ps1 to execute Sync-GoModuleGraph (go mod tidy) before server_targeted go test and before build_release.'

$overall = 'PASS'
if (($checks | Where-Object { -not $_.passed }).Count -gt 0) {
  $overall = 'FAIL'
}

$report = [ordered]@{
  generated_at = (Get-Date).ToString('yyyy-MM-ddTHH:mm:ssK')
  overall      = $overall
  summary      = [ordered]@{
    total = $checks.Count
    pass  = ($checks | Where-Object { $_.passed }).Count
    fail  = ($checks | Where-Object { -not $_.passed }).Count
  }
  checks       = $checks
}
$report | ConvertTo-Json -Depth 6 | Set-Content -Path $ReportJson -Encoding UTF8

$lines = New-Object System.Collections.Generic.List[string]
$lines.Add('# GB28181 Phase 1 Strict Mode Source Acceptance Report') | Out-Null
$lines.Add('') | Out-Null
$lines.Add("- GeneratedAt(Local): ``$($report.generated_at)``") | Out-Null
$lines.Add("- Overall: **$overall**") | Out-Null
$lines.Add('') | Out-Null
$lines.Add('| Check | Result | Detail |') | Out-Null
$lines.Add('|---|---|---|') | Out-Null
foreach ($c in $checks) {
  $result = if ($c.passed) { 'PASS' } else { 'FAIL' }
  $detail = ($c.detail -replace '\|', '\\|')
  $lines.Add("| $($c.name) | $result | $detail |") | Out-Null
}
$lines.Add('') | Out-Null
$lines.Add('This script enforces Phase 1 strict mode and fails when legacy INFO(HttpInvoke/HttpResponse*) style paths remain active.') | Out-Null
Set-Content -Path $ReportMd -Value $lines -Encoding UTF8

Write-Host $ReportMd
Write-Host $ReportJson
if ($overall -ne 'PASS') {
  exit 1
}
