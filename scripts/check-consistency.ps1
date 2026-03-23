$ErrorActionPreference = 'Stop'
$RootDir = Split-Path -Parent $PSScriptRoot
python "$RootDir/scripts/check-consistency.py"
