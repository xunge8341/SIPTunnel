package tunnelmapping

import (
	"testing"

	"siptunnel/internal/config"
)

func TestValidateMappingCapabilityValid(t *testing.T) {
	mapping := validMapping()
	mapping.MaxRequestBodyBytes = 512 * 1024
	mapping.MaxResponseBodyBytes = 2 * 1024 * 1024

	cap := config.DeriveCapability(config.NetworkModeAToBSIPBToARTP)
	result := ValidateMappingCapability(mapping, config.NetworkModeAToBSIPBToARTP, cap)
	if result.HasErrors() {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", result.Warnings)
	}
}

func TestValidateMappingCapabilityWarning(t *testing.T) {
	mapping := validMapping()
	mapping.AllowedMethods = []string{"PUT"}
	mapping.MaxRequestBodyBytes = 512 * 1024

	cap := config.DeriveCapability(config.NetworkModeAToBSIPBToARTP)
	result := ValidateMappingCapability(mapping, config.NetworkModeAToBSIPBToARTP, cap)
	if result.HasErrors() {
		t.Fatalf("expected warning only, got errors %v", result.Errors)
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("expected warnings, got none")
	}
}

func TestValidateMappingCapabilityInvalid(t *testing.T) {
	mapping := validMapping()
	mapping.MaxRequestBodyBytes = 2 * 1024 * 1024

	cap := config.DeriveCapability(config.NetworkModeAToBSIPBToARTP)
	result := ValidateMappingCapability(mapping, config.NetworkModeAToBSIPBToARTP, cap)
	if !result.HasErrors() {
		t.Fatalf("expected errors for unsupported large request body")
	}
}
