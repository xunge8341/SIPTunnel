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
$ServerDir = Join-Path $RootDir 'gateway-server'
$DistDir = Join-Path $RootDir 'dist/bin'
$EmbeddedUiDir = Join-Path $RootDir 'gateway-server/internal/server/embedded-ui'
$UiMetadataFile = Join-Path $EmbeddedUiDir '.siptunnel-ui-embed.json'
$Version = if ($env:VERSION) { $env:VERSION } else { 'dev' }
$Commit = $env:COMMIT
if (-not $Commit) {
  $gitDir = Join-Path $RootDir '.git'
  if (Test-Path $gitDir) {
    try {
      $Commit = (git -C $RootDir rev-parse --short HEAD 2>$null)
    }
    catch {
      $Commit = $null
    }
  }
}
if (-not $Commit) { $Commit = 'unknown' }
$BuildTime = if ($env:BUILD_TIME) { $env:BUILD_TIME } else { (Get-Date).ToString('yyyy-MM-dd HH:mm:ss.fff') }
$Ldflags = "-s -w -X 'main.version=$Version' -X 'main.commit=$Commit' -X 'main.buildTime=$BuildTime'"

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

  $metadataEmbeddedAt = if ($metadata.embedded_at_local) { "$($metadata.embedded_at_local)" } else { "$($metadata.embedded_at_utc)" }
  $metadataLatestWrite = if ($metadata.ui_source_latest_write_local) { "$($metadata.ui_source_latest_write_local)" } else { "$($metadata.ui_source_latest_write_utc)" }
  Write-Host "[build] UI embedded_at_local: $metadataEmbeddedAt"
  Write-Host "[build] UI source latest write: $metadataLatestWrite"
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
      $latestUiWrite = ($uiFiles | Sort-Object LastWriteTime -Descending | Select-Object -First 1).LastWriteTime
    }
  }

  function Parse-LocalBuildTimestamp {
    param([string]$Value)
    if ([string]::IsNullOrWhiteSpace($Value)) {
      return $null
    }
    $formats = @('yyyy-MM-dd HH:mm:ss.fff', 'yyyy-MM-dd HH:mm:ss', 'yyyy-MM-ddTHH:mm:ssK', 'yyyy-MM-ddTHH:mm:ss.fffK')
    foreach ($fmt in $formats) {
      try {
        return [DateTime]::ParseExact(
          $Value.Trim(),
          $fmt,
          [System.Globalization.CultureInfo]::InvariantCulture,
          [System.Globalization.DateTimeStyles]::AssumeLocal
        )
      }
      catch {
      }
    }
    return [DateTime]::Parse($Value.Trim(), [System.Globalization.CultureInfo]::InvariantCulture, [System.Globalization.DateTimeStyles]::AssumeLocal)
  }
  $embeddedAtRaw = if ($metadata.embedded_at_local) { "$($metadata.embedded_at_local)" } else { "$($metadata.embedded_at_utc)" }
  $metadataLatestUiWriteRaw = if ($metadata.ui_source_latest_write_local) { "$($metadata.ui_source_latest_write_local)" } else { "$($metadata.ui_source_latest_write_utc)" }
  $embeddedAt = Parse-LocalBuildTimestamp $embeddedAtRaw
  $metadataLatestUiWrite = Parse-LocalBuildTimestamp $metadataLatestUiWriteRaw
  $staleTolerance = [TimeSpan]::FromSeconds(2)

  $isLatest = $false
  if ($embeddedAt) {
    $candidates = @()
    if ($latestUiWrite) { $candidates += $latestUiWrite }
    if ($metadataLatestUiWrite) { $candidates += $metadataLatestUiWrite }
    if ($candidates.Count -eq 0) {
      $isLatest = $true
    } else {
      $latestCandidate = ($candidates | Sort-Object -Descending | Select-Object -First 1)
      $isLatest = $embeddedAt.Add($staleTolerance) -ge $latestCandidate
    }
  }

  Write-Host "[build] UI latest check: $isLatest"
  if (-not $isLatest) {
    throw "[build] UI embed is stale. Latest UI source write is newer than embedded timestamp beyond tolerance. Run ./scripts/embed-ui.ps1."
  }

  Write-Host '[build] UI embed validation: PASS'
}

function Get-GoModCompatVersion {
  if ($env:GO_MOD_TIDY_COMPAT) {
    return "$($env:GO_MOD_TIDY_COMPAT)"
  }

  $goModPath = Join-Path $ServerDir 'go.mod'
  if (Test-Path $goModPath) {
    $goDirective = Select-String -Path $goModPath -Pattern '^go\s+([0-9]+\.[0-9]+(?:\.[0-9]+)?)\s*$' | Select-Object -First 1
    if ($goDirective -and $goDirective.Matches.Count -gt 0) {
      $value = $goDirective.Matches[0].Groups[1].Value
      if (-not [string]::IsNullOrWhiteSpace($value)) {
        return $value
      }
    }
  }

  return '1.23.0'
}

function Sync-GoModuleGraph {
  $compatVersion = Get-GoModCompatVersion
  Write-Host "[build] syncing Go module graph (go mod tidy -compat=$compatVersion)"
  Push-Location $ServerDir
  try {
    & go mod tidy "-compat=$compatVersion"
    if ($LASTEXITCODE -ne 0) {
      throw "[build] go mod tidy failed with exit code $LASTEXITCODE"
    }
  }
  finally {
    Pop-Location
  }
}

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
    & go build -trimpath -ldflags $Ldflags -o $Output ./cmd/gateway
    if ($LASTEXITCODE -ne 0) {
      throw "[build] go build failed for $Goos/$Goarch with exit code $LASTEXITCODE"
    }
  }
  finally {
    Pop-Location
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
  }
}

New-Item -ItemType Directory -Force -Path $DistDir | Out-Null
Assert-UiEmbedReady -Policy $UiPolicy
Sync-GoModuleGraph

if ($Mode -eq 'native') {
  $hostOs = go env GOOS
  $hostArch = go env GOARCH
  Build-One $hostOs $hostArch
} else {
  Build-One 'windows' 'amd64'
  Build-One 'linux' 'amd64'
  Build-One 'linux' 'arm64'
}

Write-Host '[build] done'
