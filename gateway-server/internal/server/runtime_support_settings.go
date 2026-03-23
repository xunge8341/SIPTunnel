package server

type protectionSettings struct {
	AlertRules          []string `json:"alert_rules"`
	CircuitBreakerRules []string `json:"circuit_breaker_rules"`
	FailureThreshold    int      `json:"failure_threshold"`
	RecoveryWindowSec   int      `json:"recovery_window_sec"`
}

func defaultProtectionSettings() protectionSettings {
	return protectionSettings{
		AlertRules:          []string{"最近 15 分钟失败请求 > 20", "慢请求数 > 10"},
		CircuitBreakerRules: []string{"连续失败超过阈值触发熔断", "恢复窗口结束后半开试探"},
		FailureThreshold:    20,
		RecoveryWindowSec:   60,
	}
}

func defaultOpsLimits() OpsLimits {
	return OpsLimits{RPS: 200, Burst: 400, MaxConcurrent: 100}
}

func normalizeProtectionSettings(input protectionSettings) protectionSettings {
	defaults := defaultProtectionSettings()
	if len(input.AlertRules) == 0 {
		input.AlertRules = defaults.AlertRules
	}
	if len(input.CircuitBreakerRules) == 0 {
		input.CircuitBreakerRules = defaults.CircuitBreakerRules
	}
	if input.FailureThreshold <= 0 {
		input.FailureThreshold = defaults.FailureThreshold
	}
	if input.RecoveryWindowSec <= 0 {
		input.RecoveryWindowSec = defaults.RecoveryWindowSec
	}
	return input
}

func normalizeOpsLimits(input OpsLimits) OpsLimits {
	defaults := defaultOpsLimits()
	if input.RPS <= 0 {
		input.RPS = defaults.RPS
	}
	if input.Burst <= 0 {
		input.Burst = defaults.Burst
	}
	if input.MaxConcurrent <= 0 {
		input.MaxConcurrent = defaults.MaxConcurrent
	}
	return input
}
