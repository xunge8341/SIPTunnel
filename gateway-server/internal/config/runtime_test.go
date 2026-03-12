package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStoragePathsFromEnv(t *testing.T) {
	t.Setenv(envDataDir, filepath.Join("root", "data"))
	t.Setenv(envFinalDir, filepath.Join("override", "final"))

	paths := LoadStoragePathsFromEnv()
	if paths.TempDir != filepath.Join("root", "data", "temp") {
		t.Fatalf("unexpected temp dir: %s", paths.TempDir)
	}
	if paths.FinalDir != filepath.Join("override", "final") {
		t.Fatalf("unexpected final dir override: %s", paths.FinalDir)
	}
	if paths.AuditDir != filepath.Join("root", "data", "audit") {
		t.Fatalf("unexpected audit dir: %s", paths.AuditDir)
	}
}

func TestEnsureWritableCreatesDirs(t *testing.T) {
	base := t.TempDir()
	paths := StoragePaths{
		TempDir:  filepath.Join(base, "temp"),
		FinalDir: filepath.Join(base, "final"),
		AuditDir: filepath.Join(base, "audit"),
		LogDir:   filepath.Join(base, "log"),
	}
	if err := paths.EnsureWritable(); err != nil {
		t.Fatalf("EnsureWritable failed: %v", err)
	}
	for _, dir := range []string{paths.TempDir, paths.FinalDir, paths.AuditDir, paths.LogDir} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("expected dir exists %s: %v", dir, err)
		}
	}
}
