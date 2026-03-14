param(
  [ValidateSet('native', 'matrix')]
  [string]$Mode = 'native',

  [ValidateSet('delivery', 'dev')]
  [string]$UiPolicy = 'delivery'
)

$ErrorActionPreference = 'Stop'
$RootDir = Split-Path -Parent $PSScriptRoot
$ServerDir = Join-Path $RootDir 'gateway-server'
$DistDir = Join-Path $RootDir 'dist/bin'
$EmbeddedUiDir = Join-Path $RootDir 'gateway-server/internal/server/embedded-ui'
$UiMetadataFile = Join-Path $EmbeddedUiDir '.siptunnel-ui-embed.json'
$Version = if ($env:VERSION) { $env:VERSION } else { 'dev' }
$Commit = if ($env:COMMIT) { $env:COMMIT } else { (git -C $RootDir rev-parse --short HEAD 2>$null) }
if (-not $Commit) { $Commit = 'unknown' }
$BuildTime = if ($env:BUILD_TIME) { $env:BUILD_TIME } else { (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ') }
$Ldflags = "-s -w -X main.version=$Version -X main.commit=$Commit -X main.buildTime=$BuildTime"

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
      $relative = $_.FullName.Substring($Directory.Length).TrimStart([char]'\', [char]'/') -replace '\\', '/'
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

function Assert-UiEmbedReady {
  param([string]$Policy)

  if ($Policy -eq 'dev') {
    Write-Host '[build] UI policy: dev (skip embedded UI guard)'
    return
  }

  Write-Host '[build] UI policy: delivery (require fresh embedded UI)'

  if (-not (Test-Path $UiMetadataFile)) {
    throw "[build] UI metadata missing: $UiMetadataFile. Run ./scripts/embed-ui.ps1 first."
  }

  $metadata = Get-Content -Path $UiMetadataFile -Raw | ConvertFrom-Json
  if (-not $metadata.embedded_hash_sha256) {
    throw "[build] UI metadata invalid: embedded_hash_sha256 missing in $UiMetadataFile"
  }

  $actualEmbeddedHash = Get-DirectoryContentHash -Directory $EmbeddedUiDir -ExcludeNames @('.siptunnel-ui-embed.json')
  $expectedEmbeddedHash = "$($metadata.embedded_hash_sha256)".ToLowerInvariant()
  $hashMatched = $actualEmbeddedHash -eq $expectedEmbeddedHash

  Write-Host "[build] UI embedded_at_utc: $($metadata.embedded_at_utc)"
  Write-Host "[build] UI source latest write: $($metadata.ui_source_latest_write_utc)"
  Write-Host "[build] UI hash expected: $expectedEmbeddedHash"
  Write-Host "[build] UI hash actual:   $actualEmbeddedHash"

  if (-not $hashMatched) {
    throw "[build] UI embed validation failed: embedded assets do not match metadata. Re-run ./scripts/embed-ui.ps1."
  }

  $latestUiWrite = $null
  if (Test-Path (Join-Path $RootDir 'gateway-ui')) {
    $uiFiles = Get-ChildItem -Path (Join-Path $RootDir 'gateway-ui') -Recurse -File |
      Where-Object {
        $_.FullName -notmatch '[\\/]dist[\\/]' -and
        $_.FullName -notmatch '[\\/]node_modules[\\/]'
      }
    if ($uiFiles.Count -gt 0) {
      $latestUiWrite = ($uiFiles | Sort-Object LastWriteTimeUtc -Descending | Select-Object -First 1).LastWriteTimeUtc
    }
  }

  $embeddedAt = $null
  if ($metadata.embedded_at_utc) {
    $embeddedAt = [DateTime]::Parse("$($metadata.embedded_at_utc)").ToUniversalTime()
  }

  $isLatest = $false
  if ($embeddedAt -and $latestUiWrite) {
    $isLatest = $embeddedAt -ge $latestUiWrite
  }

  Write-Host "[build] UI latest check: $isLatest"
  if (-not $isLatest) {
    throw "[build] UI embed is stale. Latest UI source write is newer than embedded timestamp. Run ./scripts/embed-ui.ps1."
  }

  Write-Host '[build] UI embed validation: PASS'
}

New-Item -ItemType Directory -Force -Path $DistDir | Out-Null
Assert-UiEmbedReady -Policy $UiPolicy

function Build-One {
  param([string]$Goos, [string]$Goarch)
  $Ext = if ($Goos -eq 'windows') { '.exe' } else { '' }
  $OutputDir = Join-Path $DistDir "$Goos/$Goarch"
  New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
  $Output = Join-Path $OutputDir "gateway$Ext"
  Write-Host "[build] $Goos/$Goarch -> $Output"

  Push-Location $ServerDir
  try {
    $env:CGO_ENABLED = '0'
    $env:GOOS = $Goos
    $env:GOARCH = $Goarch
    go build -trimpath -ldflags $Ldflags -o $Output ./cmd/gateway
  }
  finally {
    Pop-Location
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
  }
}

if ($Mode -eq 'native') {
  $hostOs = go env GOOS
  $hostArch = go env GOARCH
  Build-One $hostOs $hostArch
} else {
  Build-One 'linux' 'amd64'
  Build-One 'linux' 'arm64'
  Build-One 'windows' 'amd64'
  Build-One 'darwin' 'amd64'
}

Write-Host '[build] done'
