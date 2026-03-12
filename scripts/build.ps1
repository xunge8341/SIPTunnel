param(
  [ValidateSet('native', 'matrix')]
  [string]$Mode = 'native'
)

$ErrorActionPreference = 'Stop'
$RootDir = Split-Path -Parent $PSScriptRoot
$ServerDir = Join-Path $RootDir 'gateway-server'
$DistDir = Join-Path $RootDir 'dist'
$Version = if ($env:VERSION) { $env:VERSION } else { 'dev' }
$Commit = if ($env:COMMIT) { $env:COMMIT } else { (git -C $RootDir rev-parse --short HEAD 2>$null) }
if (-not $Commit) { $Commit = 'unknown' }
$BuildTime = if ($env:BUILD_TIME) { $env:BUILD_TIME } else { (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ') }
$Ldflags = "-s -w -X main.version=$Version -X main.commit=$Commit -X main.buildTime=$BuildTime"

New-Item -ItemType Directory -Force -Path $DistDir | Out-Null

function Build-One {
  param([string]$Goos, [string]$Goarch)
  $Ext = if ($Goos -eq 'windows') { '.exe' } else { '' }
  $Output = Join-Path $DistDir "gateway-$Goos-$Goarch$Ext"
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
  Build-One 'windows' 'amd64'
  Build-One 'darwin' 'amd64'
}

Write-Host '[build] done'
