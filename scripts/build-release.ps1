param(
  [ValidateSet('native', 'matrix')]
  [string]$Mode = 'native',

  [ValidateSet('delivery', 'dev')]
  [string]$UiPolicy = 'delivery'
)

$ErrorActionPreference = 'Stop'

$RootDir = Split-Path -Parent $PSScriptRoot
$UiDir = Join-Path $RootDir 'gateway-ui'
$DistDir = Join-Path $UiDir 'dist'
$EmbeddedUiDir = Join-Path $RootDir 'gateway-server/internal/server/embedded-ui'
$UiMetadataFile = Join-Path $EmbeddedUiDir '.siptunnel-ui-embed.json'
$BackendOutputRoot = Join-Path $RootDir 'dist/bin'
$ReleaseRoot = Join-Path $RootDir 'dist/release'
$Timestamp = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ')
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
      $relative = $_.FullName.Substring($Directory.Length).TrimStart('\\', '/') -replace '\\', '/'
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

$deliverySummary = [ordered]@{
  mode = $Mode
  ui_policy = $UiPolicy
  build_nonce = $buildNonce
  created_at_utc = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
  ui_build_success = $summary.ui_build_success
  ui_embed_dir = (Resolve-Path $EmbeddedUiDir).Path
  embed_validation = $summary.embed_validation
  backend_output_path = (Resolve-Path $BackendOutputRoot).Path
  final_delivery_package = (Resolve-Path $FinalDeliveryDir).Path
}
$deliverySummary | ConvertTo-Json | Set-Content -Path (Join-Path $FinalDeliveryDir 'build-summary.json') -Encoding UTF8

Write-Host ''
Write-Host '================ Release Build Summary ================'
Write-Host "UI 构建成功: $($deliverySummary.ui_build_success)"
Write-Host "UI 嵌入目录: $($deliverySummary.ui_embed_dir)"
Write-Host "嵌入校验结果: $($deliverySummary.embed_validation)"
Write-Host "后端输出路径: $($deliverySummary.backend_output_path)"
Write-Host "最终交付包位置: $($deliverySummary.final_delivery_package)"
Write-Host '======================================================='
