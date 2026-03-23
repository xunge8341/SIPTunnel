package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	envDataDir  = "GATEWAY_DATA_DIR"
	envTempDir  = "GATEWAY_TEMP_DIR"
	envFinalDir = "GATEWAY_FINAL_DIR"
	envAuditDir = "GATEWAY_AUDIT_DIR"
	envLogDir   = "GATEWAY_LOG_DIR"
)

type StoragePaths struct {
	TempDir  string
	FinalDir string
	AuditDir string
	LogDir   string
}

func DefaultStoragePaths() StoragePaths {
	base := filepath.Join(".", "data")
	return StoragePaths{
		TempDir:  filepath.Join(base, "temp"),
		FinalDir: filepath.Join(base, "final"),
		AuditDir: filepath.Join(base, "audit"),
		LogDir:   filepath.Join(base, "logs"),
	}
}

func LoadStoragePathsFromEnv() StoragePaths {
	paths := DefaultStoragePaths()
	if base := strings.TrimSpace(os.Getenv(envDataDir)); base != "" {
		paths.TempDir = filepath.Join(base, "temp")
		paths.FinalDir = filepath.Join(base, "final")
		paths.AuditDir = filepath.Join(base, "audit")
		paths.LogDir = filepath.Join(base, "logs")
	}
	paths.TempDir = stringOrDefault(os.Getenv(envTempDir), paths.TempDir)
	paths.FinalDir = stringOrDefault(os.Getenv(envFinalDir), paths.FinalDir)
	paths.AuditDir = stringOrDefault(os.Getenv(envAuditDir), paths.AuditDir)
	paths.LogDir = stringOrDefault(os.Getenv(envLogDir), paths.LogDir)
	return paths
}

func (p StoragePaths) EnsureWritable() error {
	for name, dir := range map[string]string{
		"temp_dir":  p.TempDir,
		"final_dir": p.FinalDir,
		"audit_dir": p.AuditDir,
		"log_dir":   p.LogDir,
	} {
		if strings.TrimSpace(dir) == "" {
			return fmt.Errorf("%s is empty, please set %s", name, envNameForKey(name))
		}
		if err := ensureDirWritable(dir); err != nil {
			return fmt.Errorf("%s check failed for %q: %w", name, dir, err)
		}
	}
	return nil
}

func ensureDirWritable(dir string) error {
	cleaned := filepath.Clean(dir)
	if err := os.MkdirAll(cleaned, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	f, err := os.CreateTemp(cleaned, ".gateway-write-check-*")
	if err != nil {
		return fmt.Errorf("directory is not writable: %w", err)
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

func stringOrDefault(raw string, fallback string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return fallback
	}
	return filepath.Clean(v)
}

func envNameForKey(key string) string {
	switch key {
	case "temp_dir":
		return envTempDir
	case "final_dir":
		return envFinalDir
	case "audit_dir":
		return envAuditDir
	case "log_dir":
		return envLogDir
	default:
		return ""
	}
}
