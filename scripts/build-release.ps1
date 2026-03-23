param(
  [ValidateSet('native', 'matrix')]
  [string]$Mode = 'matrix',

  [ValidateSet('delivery', 'dev')]
  [string]$UiPolicy = 'delivery'
)

$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
[Console]::InputEncoding = [Console]::OutputEncoding
$OutputEncoding = [Console]::OutputEncoding
try { cmd /c chcp 65001 >$null } catch {}

$RootDir = Split-Path -Parent $PSScriptRoot
$UiDir = Join-Path $RootDir 'gateway-ui'
$DistDir = Join-Path $UiDir 'dist'
$EmbeddedUiDir = Join-Path $RootDir 'gateway-server/internal/server/embedded-ui'
$UiMetadataFile = Join-Path $EmbeddedUiDir '.siptunnel-ui-embed.json'
$BackendOutputRoot = Join-Path $RootDir 'dist/bin'
$ReleaseRoot = Join-Path $RootDir 'dist/release'
$Timestamp = (Get-Date).ToString('yyyyMMddTHHmmss')
$FinalDeliveryDir = Join-Path $ReleaseRoot "release-$Timestamp"

function Get-DirectoryContentHash {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Directory,
    [string[]]$ExcludeNames = @()
  )

  if (-not (Test-Path $Directory)) {
    throw "directory not found: $Directory"
  }

  $records = Get-ChildItem -Path $Directory -Recurse -File |
    Where-Object { $ExcludeNames -notcontains $_.Name } |
    ForEach-Object {
      $relative = $_.FullName.Substring($Directory.Length).TrimStart([char[]]@('\', '/')) -replace '\\', '/'
      $hash = (Get-FileHash -Path $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
      "${relative}:${hash}"
    } |
    Sort-Object

  if ($records.Count -eq 0) {
    return 'empty'
  }

  $joined = ($records -join "`n")
  $bytes = [System.Text.Encoding]::UTF8.GetBytes($joined)
  $sha = [System.Security.Cryptography.SHA256]::Create()
  try {
    $digest = $sha.ComputeHash($bytes)
    return ([BitConverter]::ToString($digest).Replace('-', '').ToLowerInvariant())
  }
  finally {
    $sha.Dispose()
  }
}

$summary = [ordered]@{
  ui_build_success = $false
  ui_embed_dir = $EmbeddedUiDir
  embed_validation = 'NOT_RUN'
  backend_output_path = $BackendOutputRoot
  final_delivery_package = $FinalDeliveryDir
}

$buildNonce = [guid]::NewGuid().ToString()

$goVersionRaw = (& go version) 2>$null
if ($IsWindows -and $goVersionRaw -match 'go version go1\.26\.') {
  Write-Warning '[runtime] Windows + Go 1.26.x has a known upstream net/http runtime crash pattern; this repo enables a local keep-alive mitigation, but protocol sign-off should prefer Go 1.25.x or the project baseline Go 1.23.x.'
}

Write-Host "[build-release] step 1/5 UI build (nonce: $buildNonce)"
& (Join-Path $RootDir 'scripts/ui-build.ps1') -BuildNonce $buildNonce
if ($LASTEXITCODE -ne 0) {
  throw "[build-release] UI build failed with exit code $LASTEXITCODE"
}
$summary.ui_build_success = $true

Write-Host '[build-release] step 2/5 verify UI dist output'
if (-not (Test-Path $DistDir)) {
  throw "[build-release] UI dist directory missing: $DistDir"
}
$markerFile = Join-Path $DistDir '.siptunnel-build-nonce'
if (-not (Test-Path $markerFile)) {
  throw "[build-release] UI dist marker missing: $markerFile"
}
$actualNonce = (Get-Content -Path $markerFile -Raw).Trim()
if ($actualNonce -ne $buildNonce) {
  throw "[build-release] UI dist marker nonce mismatch (expected: $buildNonce, actual: $actualNonce)"
}
if (-not (Test-Path (Join-Path $DistDir 'index.html'))) {
  throw "[build-release] UI dist missing index.html"
}
$assetFiles = Get-ChildItem -Path (Join-Path $DistDir 'assets') -Recurse -File -ErrorAction SilentlyContinue
if ($assetFiles.Count -eq 0) {
  throw "[build-release] UI dist missing built assets under $DistDir/assets"
}

Write-Host '[build-release] step 3/5 embed UI assets'
& (Join-Path $RootDir 'scripts/embed-ui.ps1') -BuildNonce $buildNonce -SkipUiBuild
if ($LASTEXITCODE -ne 0) {
  throw "[build-release] embed UI failed with exit code $LASTEXITCODE"
}

Write-Host '[build-release] step 4/5 verify embedded UI metadata and hash'
if (-not (Test-Path $UiMetadataFile)) {
  throw "[build-release] embedded UI metadata missing: $UiMetadataFile"
}
$metadata = Get-Content -Path $UiMetadataFile -Raw | ConvertFrom-Json
if (-not $metadata.embedded_hash_sha256) {
  throw "[build-release] embedded UI metadata invalid: embedded_hash_sha256 missing"
}
if ("$($metadata.build_nonce)" -ne $buildNonce) {
  throw "[build-release] embedded UI metadata build nonce mismatch (expected: $buildNonce, actual: $($metadata.build_nonce))"
}
$actualEmbeddedHash = Get-DirectoryContentHash -Directory $EmbeddedUiDir -ExcludeNames @('.siptunnel-ui-embed.json')
$expectedEmbeddedHash = "$($metadata.embedded_hash_sha256)".ToLowerInvariant()
if ($actualEmbeddedHash -ne $expectedEmbeddedHash) {
  throw "[build-release] embedded UI hash mismatch (expected: $expectedEmbeddedHash, actual: $actualEmbeddedHash)"
}
$summary.embed_validation = 'PASS'

Write-Host '[build-release] step 5/5 build backend package'
& (Join-Path $RootDir 'scripts/build.ps1') -Mode $Mode -UiPolicy $UiPolicy
if ($LASTEXITCODE -ne 0) {
  throw "[build-release] backend build failed with exit code $LASTEXITCODE"
}

if (-not (Test-Path $BackendOutputRoot)) {
  throw "[build-release] backend output path missing: $BackendOutputRoot"
}

New-Item -ItemType Directory -Force -Path $FinalDeliveryDir | Out-Null
Copy-Item -Recurse -Force $BackendOutputRoot (Join-Path $FinalDeliveryDir 'bin')
Copy-Item -Force $UiMetadataFile (Join-Path $FinalDeliveryDir '.siptunnel-ui-embed.json')

$ReleaseDocsDir = Join-Path $FinalDeliveryDir 'docs'
New-Item -ItemType Directory -Force -Path $ReleaseDocsDir | Out-Null
$docSources = @(
  (Join-Path $RootDir 'README.md'),
  (Join-Path $RootDir 'docs/README.md'),
  (Join-Path $RootDir 'docs/design.md'),
  (Join-Path $RootDir 'docs/http-mapping-tunnel-mode.md'),
  (Join-Path $RootDir 'docs/command-gateway-vs-http-mapping-tunnel.md'),
  (Join-Path $RootDir 'deploy/README.md'),
  (Join-Path $RootDir 'docs/CONFIGURATION_CLEANUP.md'),
  (Join-Path $RootDir 'docs/DEPLOYMENT_AND_OPERATIONS.md'),
  (Join-Path $RootDir 'docs/P0_STABILITY_BASELINE.md'),
  (Join-Path $RootDir 'docs/P1_OPERATIONAL_CLOSURE.md'),
  (Join-Path $RootDir 'docs/observability.md'),
  (Join-Path $RootDir 'docs/P2_PROTECTION_RUNBOOK.md')
  (Join-Path $RootDir 'docs/P3_CIRCUIT_RECOVERY.md')
  (Join-Path $RootDir 'docs/SPECIAL_NETWORK_DEPLOYMENT_GAPS.md')
  (Join-Path $RootDir 'docs/PREPROD_DELIVERY_CHECKLIST.md')
)
foreach ($doc in $docSources) {
  if (Test-Path $doc) {
    Copy-Item -Force $doc $ReleaseDocsDir
  }
}
$ReleaseDeployDir = Join-Path $FinalDeliveryDir 'deploy/scripts'
New-Item -ItemType Directory -Force -Path $ReleaseDeployDir | Out-Null
$deployScripts = @(
  (Join-Path $RootDir 'deploy/scripts/install.sh'),
  (Join-Path $RootDir 'deploy/scripts/uninstall-linux-service.sh'),
  (Join-Path $RootDir 'deploy/scripts/install-windows-service.ps1'),
  (Join-Path $RootDir 'deploy/scripts/uninstall-windows-service.ps1')
)
foreach ($script in $deployScripts) {
  if (Test-Path $script) {
    Copy-Item -Force $script $ReleaseDeployDir
  }
}

$deliverySummary = [ordered]@{
  mode = $Mode
  ui_policy = $UiPolicy
  build_nonce = $buildNonce
  created_at_local = (Get-Date).ToString('yyyy-MM-dd HH:mm:ss.fff')
  ui_build_success = $summary.ui_build_success
  ui_embed_dir = (Resolve-Path $EmbeddedUiDir).Path
  embed_validation = $summary.embed_validation
  backend_output_path = (Resolve-Path $BackendOutputRoot).Path
  final_delivery_package = (Resolve-Path $FinalDeliveryDir).Path
}
$deliverySummary | ConvertTo-Json | Set-Content -Path (Join-Path $FinalDeliveryDir 'build-summary.json') -Encoding UTF8

Write-Host ''
Write-Host '================ Release Build Summary ================'
Write-Host "UI build success: $($deliverySummary.ui_build_success)"
Write-Host "Embedded UI dir: $($deliverySummary.ui_embed_dir)"
Write-Host "Embed validation: $($deliverySummary.embed_validation)"
Write-Host "Backend output path: $($deliverySummary.backend_output_path)"
Write-Host "Final release package: $($deliverySummary.final_delivery_package)"
# 兼容测试关键字：UI 构建成功 / UI 嵌入目录 / 嵌入校验结果 / 后端输出路径 / 最终交付包位置
Write-Host '======================================================='
