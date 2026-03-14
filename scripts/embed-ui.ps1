$ErrorActionPreference = 'Stop'

$RootDir = Split-Path -Parent $PSScriptRoot
$UiDir = Join-Path $RootDir 'gateway-ui'
$DistDir = Join-Path $UiDir 'dist'
$TargetDir = Join-Path $RootDir 'gateway-server/internal/server/embedded-ui'
$MetadataFile = Join-Path $TargetDir '.siptunnel-ui-embed.json'

$BuildNonce = [guid]::NewGuid().ToString()

Write-Host "[embed-ui] running UI build with nonce: $BuildNonce"
& (Join-Path $RootDir 'scripts/ui-build.ps1') -BuildNonce $BuildNonce
if ($LASTEXITCODE -ne 0) {
  throw "[embed-ui] UI build step failed with exit code $LASTEXITCODE. Embedding aborted."
}

if (-not (Test-Path $DistDir)) {
  throw "[embed-ui] UI build output missing: $DistDir. Embedding aborted."
}

$MarkerFile = Join-Path $DistDir '.siptunnel-build-nonce'
if (-not (Test-Path $MarkerFile)) {
  throw "[embed-ui] build marker missing: $MarkerFile. Refusing to embed stale dist."
}

$ActualNonce = (Get-Content -Path $MarkerFile -Raw).Trim()
if ([string]::IsNullOrWhiteSpace($ActualNonce) -or $ActualNonce -ne $BuildNonce) {
  throw "[embed-ui] build marker nonce mismatch (expected: $BuildNonce, actual: $ActualNonce). Refusing to embed stale dist."
}

Write-Host '[embed-ui] build marker validated, syncing embedded assets'

if (Test-Path $TargetDir) {
  Remove-Item -Recurse -Force $TargetDir
}

New-Item -ItemType Directory -Force -Path $TargetDir | Out-Null
Copy-Item -Recurse -Force (Join-Path $DistDir '*') $TargetDir

if (Test-Path (Join-Path $TargetDir '.siptunnel-build-nonce')) {
  Remove-Item -Force (Join-Path $TargetDir '.siptunnel-build-nonce')
}


$ErrorsDir = Join-Path $TargetDir 'errors'
New-Item -ItemType Directory -Force -Path $ErrorsDir | Out-Null
if (-not (Test-Path (Join-Path $ErrorsDir '404.html'))) {
  Set-Content -Path (Join-Path $ErrorsDir '404.html') -Encoding UTF8 -Value @'
<!doctype html>
<html><head><meta charset="utf-8"><title>404 Not Found</title></head><body><h1>404 Not Found</h1><p>页面未找到 / Requested resource was not found.</p></body></html>
'@
}
if (-not (Test-Path (Join-Path $ErrorsDir '500.html'))) {
  Set-Content -Path (Join-Path $ErrorsDir '500.html') -Encoding UTF8 -Value @'
<!doctype html>
<html><head><meta charset="utf-8"><title>500 Internal Server Error</title></head><body><h1>500 Internal Server Error</h1><p>UI fallback page is temporarily unavailable.</p></body></html>
'@
}


if (-not (Test-Path (Join-Path $TargetDir 'favicon.svg'))) {
  Set-Content -Path (Join-Path $TargetDir 'favicon.svg') -Encoding UTF8 -Value @'
<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128"><rect width="128" height="128" rx="24" fill="#1677ff"/><text x="64" y="74" text-anchor="middle" font-family="Arial, sans-serif" font-size="44" fill="#fff">ST</text></svg>
'@
}

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

function Get-LatestWriteTimeUtcIso {
  param([Parameter(Mandatory = $true)][string[]]$Paths)

  $items = @()
  foreach ($path in $Paths) {
    if (Test-Path $path) {
      $items += Get-ChildItem -Path $path -Recurse -File -ErrorAction SilentlyContinue
    }
  }

  if ($items.Count -eq 0) {
    return $null
  }

  $latest = ($items | Sort-Object LastWriteTimeUtc -Descending | Select-Object -First 1).LastWriteTimeUtc
  return $latest.ToString('yyyy-MM-ddTHH:mm:ssZ')
}

$embeddedAt = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
$distHash = Get-DirectoryContentHash -Directory $DistDir -ExcludeNames @('.siptunnel-build-nonce')
$embeddedHash = Get-DirectoryContentHash -Directory $TargetDir -ExcludeNames @('.siptunnel-ui-embed.json')
$uiSourceLatest = Get-LatestWriteTimeUtcIso -Paths @(
  (Join-Path $UiDir 'src'),
  (Join-Path $UiDir 'public'),
  (Join-Path $UiDir 'index.html'),
  (Join-Path $UiDir 'package.json'),
  (Join-Path $UiDir 'package-lock.json'),
  (Join-Path $UiDir 'vite.config.ts')
)

$metadata = [ordered]@{
  schema_version = 1
  generated_by = 'scripts/embed-ui.ps1'
  build_nonce = $BuildNonce
  embedded_at_utc = $embeddedAt
  ui_source_latest_write_utc = $uiSourceLatest
  dist_hash_sha256 = $distHash
  embedded_hash_sha256 = $embeddedHash
  dist_dir = (Resolve-Path $DistDir).Path
  embedded_dir = (Resolve-Path $TargetDir).Path
}

$metadata | ConvertTo-Json | Set-Content -Path $MetadataFile -Encoding UTF8

Write-Host "[embed-ui] UI source latest write: $uiSourceLatest"
Write-Host "[embed-ui] embedded at (UTC): $embeddedAt"
Write-Host "[embed-ui] embedded hash: $embeddedHash"
Write-Host "[embed-ui] metadata: $MetadataFile"

Write-Host "embedded UI assets synced to $TargetDir"
