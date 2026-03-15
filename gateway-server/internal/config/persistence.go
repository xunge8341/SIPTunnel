package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	envStorageBackend = "GATEWAY_STORAGE_BACKEND"
	envSQLitePath     = "GATEWAY_SQLITE_PATH"
	envCleanupEvery   = "GATEWAY_CLEANUP_INTERVAL"
	envMaxLogSizeMB   = "GATEWAY_LOG_MAX_SIZE_MB"
	envMaxLogFiles    = "GATEWAY_LOG_MAX_FILES"
	envMaxLogAgeDays  = "GATEWAY_LOG_MAX_AGE_DAYS"
)

type PersistenceConfig struct {
	Backend         string
	SQLitePath      string
	CleanupInterval time.Duration
	Retention       RetentionConfig
	LogRetention    LogRetentionConfig
}

type RetentionConfig struct {
	MaxTaskRecords       int
	MaxTaskAgeDays       int
	MaxAuditRecords      int
	MaxAuditAgeDays      int
	MaxDiagnosticRecords int
	MaxDiagnosticAgeDays int
}

type LogRetentionConfig struct {
	MaxSizeMB  int
	MaxFiles   int
	MaxAgeDays int
}

func DefaultPersistenceConfig() PersistenceConfig {
	return PersistenceConfig{
		Backend:         "sqlite",
		SQLitePath:      filepath.Join(".", "data", "gateway.db"),
		CleanupInterval: 30 * time.Minute,
		Retention: RetentionConfig{
			MaxTaskRecords:       50000,
			MaxTaskAgeDays:       30,
			MaxAuditRecords:      100000,
			MaxAuditAgeDays:      30,
			MaxDiagnosticRecords: 5000,
			MaxDiagnosticAgeDays: 14,
		},
		LogRetention: LogRetentionConfig{MaxSizeMB: 20, MaxFiles: 5, MaxAgeDays: 7},
	}
}

func LoadPersistenceConfigFromEnv() PersistenceConfig {
	cfg := DefaultPersistenceConfig()
	cfg.Backend = stringOrDefault(os.Getenv(envStorageBackend), cfg.Backend)
	cfg.SQLitePath = stringOrDefault(os.Getenv(envSQLitePath), cfg.SQLitePath)
	cfg.CleanupInterval = durationOrDefault(os.Getenv(envCleanupEvery), cfg.CleanupInterval)
	cfg.LogRetention.MaxSizeMB = intOrDefault(os.Getenv(envMaxLogSizeMB), cfg.LogRetention.MaxSizeMB)
	cfg.LogRetention.MaxFiles = intOrDefault(os.Getenv(envMaxLogFiles), cfg.LogRetention.MaxFiles)
	cfg.LogRetention.MaxAgeDays = intOrDefault(os.Getenv(envMaxLogAgeDays), cfg.LogRetention.MaxAgeDays)
	return cfg
}

func intOrDefault(raw string, fallback int) int {
	v := strings.TrimSpace(raw)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func durationOrDefault(raw string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(raw)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}
