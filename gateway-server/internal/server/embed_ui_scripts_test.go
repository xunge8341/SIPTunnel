package server

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func readRepoScript(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..", "..")
	data, err := os.ReadFile(filepath.Join(repoRoot, "scripts", name))
	if err != nil {
		t.Fatalf("read %s failed: %v", name, err)
	}
	return string(data)
}

func TestUIBuildPowerShellScriptFailFastOnBuildError(t *testing.T) {
	script := readRepoScript(t, "ui-build.ps1")
	for _, want := range []string{
		"$PackageJsonPath = Join-Path $UiDir 'package.json'",
		"if (-not (Test-Path $UiDir))",
		"throw \"UI directory not found:",
		"if (-not (Test-Path $PackageJsonPath))",
		"throw \"UI package manifest missing:",
		"npm run typecheck",
		"throw \"UI type check failed with exit code",
		"npm run build:bundle",
		"if ($LASTEXITCODE -ne 0)",
		"throw \"UI build failed with exit code",
		"Remove-Item -Recurse -Force $DistDir",
		".siptunnel-build-nonce",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("ui-build.ps1 should contain %q", want)
		}
	}
}

func TestEmbedUIPowerShellScriptRefusesStaleDist(t *testing.T) {
	script := readRepoScript(t, "embed-ui.ps1")
	for _, want := range []string{
		"[switch]$SkipUiBuild",
		"if (-not $SkipUiBuild)",
		"& (Join-Path $RootDir 'scripts/ui-build.ps1') -BuildNonce $BuildNonce",
		"if ($LASTEXITCODE -ne 0)",
		"build marker missing",
		"build marker nonce mismatch",
		"ui-delivery-guard.mjs",
		"delivery_guard_status",
		"Remove-Item -Recurse -Force $TargetDir",
		"Copy-Item -Recurse -Force (Join-Path $DistDir '*') $TargetDir",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("embed-ui.ps1 should contain %q", want)
		}
	}
}

func TestBuildReleasePowerShellScriptSuccessSummary(t *testing.T) {
	script := readRepoScript(t, "build-release.ps1")
	for _, want := range []string{
		"[build-release] step 1/5 UI build",
		"[build-release] step 2/5 verify UI dist output",
		"[build-release] step 3/5 embed UI assets",
		"[build-release] step 4/5 verify embedded UI metadata and hash",
		"[build-release] step 5/5 build backend package",
		"================ Release Build Summary ================",
		"UI build success:",
		"Embedded UI dir:",
		"Embed validation:",
		"Backend output path:",
		"Final release package:",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("build-release.ps1 should contain %q", want)
		}
	}
}

func TestBuildReleasePowerShellScriptFailFast(t *testing.T) {
	script := readRepoScript(t, "build-release.ps1")
	for _, want := range []string{
		"throw \"[build-release] UI build failed with exit code",
		"throw \"[build-release] UI dist marker missing",
		"throw \"[build-release] UI dist marker nonce mismatch",
		"throw \"[build-release] embedded UI metadata missing",
		"throw \"[build-release] embedded UI hash mismatch",
		"throw \"[build-release] backend build failed with exit code",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("build-release.ps1 should contain %q", want)
		}
	}
}

func TestPhase1AcceptancePowerShellScriptRunsServerTargetedBeforeBuildRelease(t *testing.T) {
	script := readRepoScript(t, filepath.Join("acceptance", "run_phase1_strict_acceptance.ps1"))
	for _, want := range []string{
		"[switch]$SkipServerTests",
		"Invoke-Step 'server_targeted'",
		"go test ./internal/protocol/... ./internal/server ./internal/selfcheck ./internal/config -count=1",
		"Invoke-Step 'build_release'",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("run_phase1_strict_acceptance.ps1 should contain %q", want)
		}
	}
	if strings.Index(script, "Invoke-Step 'server_targeted'") > strings.Index(script, "Invoke-Step 'build_release'") {
		t.Fatal("expected server_targeted step to run before build_release")
	}
}
