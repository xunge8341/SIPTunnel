# Phase 10 PowerShell build fix

- Fixes `Sync-GoModuleGraph` being invoked before the function is defined in `scripts/build.ps1`.
- Fixes PowerShell/Go argument handling by invoking `go mod tidy` with `"-compat=1.23.0"`.
- Keeps `build.sh` aligned with the same Go compatibility target.
