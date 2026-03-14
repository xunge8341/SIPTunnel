package tunnelmapping

import (
	"fmt"
	"strings"

	"siptunnel/internal/config"
)

const (
	SmallBodyLimitBytes             int64 = 1 * 1024 * 1024
	recommendedMaxRequestBodyBytes  int64 = 16 * 1024 * 1024
	recommendedMaxResponseBodyBytes int64 = 64 * 1024 * 1024
)

type CapabilityValidationResult struct {
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

func (r CapabilityValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

func ValidateMappingCapability(mapping TunnelMapping, mode config.NetworkMode, capability config.Capability) CapabilityValidationResult {
	result := CapabilityValidationResult{}
	modeName := mode.Normalize()

	if !capability.SupportsLargeRequestBody && mapping.MaxRequestBodyBytes > SmallBodyLimitBytes {
		result.Errors = append(result.Errors, fmt.Sprintf("mapping %s max_request_body_bytes=%d exceeds mode %s limit=%d", mapping.MappingID, mapping.MaxRequestBodyBytes, modeName, SmallBodyLimitBytes))
	}
	if !capability.SupportsLargeResponseBody && mapping.MaxResponseBodyBytes > SmallBodyLimitBytes {
		result.Errors = append(result.Errors, fmt.Sprintf("mapping %s max_response_body_bytes=%d exceeds mode %s limit=%d", mapping.MappingID, mapping.MaxResponseBodyBytes, modeName, SmallBodyLimitBytes))
	}
	if mapping.RequireStreamingResponse && !capability.SupportsStreamingResponse {
		result.Errors = append(result.Errors, fmt.Sprintf("mapping %s requires streaming response but mode %s does not support it", mapping.MappingID, modeName))
	}

	if !capability.SupportsBidirectionalHTTPTunnel {
		for _, method := range mapping.AllowedMethods {
			normalized := strings.ToUpper(strings.TrimSpace(method))
			switch normalized {
			case "CONNECT", "TRACE":
				result.Errors = append(result.Errors, fmt.Sprintf("mapping %s method %s is not allowed in mode %s", mapping.MappingID, normalized, modeName))
			case "PUT", "PATCH", "DELETE":
				result.Warnings = append(result.Warnings, fmt.Sprintf("mapping %s method %s may be unstable in restricted mode %s", mapping.MappingID, normalized, modeName))
			}
		}
	}

	if mapping.MaxRequestBodyBytes > recommendedMaxRequestBodyBytes {
		result.Warnings = append(result.Warnings, fmt.Sprintf("mapping %s max_request_body_bytes=%d exceeds recommended=%d", mapping.MappingID, mapping.MaxRequestBodyBytes, recommendedMaxRequestBodyBytes))
	}
	if mapping.MaxResponseBodyBytes > recommendedMaxResponseBodyBytes {
		result.Warnings = append(result.Warnings, fmt.Sprintf("mapping %s max_response_body_bytes=%d exceeds recommended=%d", mapping.MappingID, mapping.MaxResponseBodyBytes, recommendedMaxResponseBodyBytes))
	}

	return result
}

func ValidateMappingsCapability(mappings []TunnelMapping, mode config.NetworkMode, capability config.Capability) CapabilityValidationResult {
	combined := CapabilityValidationResult{}
	for _, mapping := range mappings {
		result := ValidateMappingCapability(mapping, mode, capability)
		combined.Errors = append(combined.Errors, result.Errors...)
		combined.Warnings = append(combined.Warnings, result.Warnings...)
	}
	return combined
}
