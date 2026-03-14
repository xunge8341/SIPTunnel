param(
  [string]$BuildNonce = [guid]::NewGuid().ToString(),
  [switch]$CheckOnly
)

$ErrorActionPreference = 'Stop'

$RootDir = Split-Path -Parent $PSScriptRoot
$UiDir = Join-Path $RootDir 'gateway-ui'
$DistDir = Join-Path $UiDir 'dist'
$PackageJsonPath = Join-Path $UiDir 'package.json'

function Restore-PackageManifestFromGit {
  param(
    [string]$RepositoryRoot,
    [string]$RelativeManifestPath,
    [string]$ManifestPath
  )

  if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    return $false
  }

  git -C $RepositoryRoot rev-parse --is-inside-work-tree *> $null
  if ($LASTEXITCODE -ne 0) {
    return $false
  }

  git -C $RepositoryRoot ls-files --error-unmatch $RelativeManifestPath *> $null
  if ($LASTEXITCODE -ne 0) {
    return $false
  }

  Write-Host "[ui-build] package.json missing, restoring from git index: $RelativeManifestPath"
  git -C $RepositoryRoot checkout -- $RelativeManifestPath
  if ($LASTEXITCODE -ne 0) {
    throw "Failed to restore $RelativeManifestPath from git"
  }

  return (Test-Path $ManifestPath)
}

if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
  throw 'npm not found in PATH'
}

if (-not (Test-Path $UiDir)) {
  throw "UI directory not found: $UiDir"
}

if (-not (Test-Path $PackageJsonPath)) {
  $restored = Restore-PackageManifestFromGit -RepositoryRoot $RootDir -RelativeManifestPath 'gateway-ui/package.json' -ManifestPath $PackageJsonPath
  if (-not $restored) {
    throw "UI package manifest missing: $PackageJsonPath. Please restore gateway-ui/package.json before running UI build (for git repos you can run: git checkout -- gateway-ui/package.json)."
  }
}

if ($CheckOnly) {
  Write-Host '[ui-build] check-only mode completed; manifest and toolchain validation passed'
  exit 0
}

Push-Location $UiDir
try {
  if (-not (Test-Path 'node_modules')) {
    Write-Host '[ui-build] node_modules not found, running npm install'
    npm install
    if ($LASTEXITCODE -ne 0) {
      throw "npm install failed with exit code $LASTEXITCODE"
    }
  }

  if (Test-Path $DistDir) {
    Write-Host "[ui-build] removing stale dist at $DistDir"
    Remove-Item -Recurse -Force $DistDir
  }

  Write-Host '[ui-build] running UI type check (npm run typecheck)'
  npm run typecheck
  if ($LASTEXITCODE -ne 0) {
    throw "UI type check failed with exit code $LASTEXITCODE. Aborting before vite build."
  }

  Write-Host '[ui-build] running UI bundle build (npm run build:bundle)'
  npm run build:bundle
  if ($LASTEXITCODE -ne 0) {
    throw "UI build failed with exit code $LASTEXITCODE. Aborting before dist sync/embedding."
  }

  if (-not (Test-Path $DistDir)) {
    throw "UI build completed but dist directory was not generated: $DistDir"
  }

  $MarkerFile = Join-Path $DistDir '.siptunnel-build-nonce'
  Set-Content -Path $MarkerFile -Value $BuildNonce -Encoding UTF8

  Write-Host "[ui-build] build output: $DistDir"
  Write-Host "[ui-build] build nonce: $BuildNonce"
}
finally {
  Pop-Location
}
