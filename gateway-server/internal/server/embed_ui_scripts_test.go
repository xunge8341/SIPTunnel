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
		"npm run build",
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
		"& (Join-Path $RootDir 'scripts/ui-build.ps1') -BuildNonce $BuildNonce",
		"if ($LASTEXITCODE -ne 0)",
		"build marker missing",
		"build marker nonce mismatch",
		"Remove-Item -Recurse -Force $TargetDir",
		"Copy-Item -Recurse -Force (Join-Path $DistDir '*') $TargetDir",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("embed-ui.ps1 should contain %q", want)
		}
	}
}
