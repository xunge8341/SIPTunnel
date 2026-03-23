package tunnelmapping

import "strings"

const (
	DefaultResourceType = "SERVICE"
	DefaultNodeType     = "SERVER"
)

func IsGBCode20(value string) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) != 20 {
		return false
	}
	for _, ch := range trimmed {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func NormalizeResourceType(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "CAMERA":
		return "CAMERA"
	case "SERVER", "SERVICE", "HTTP_SERVICE", "HTTP_API":
		return "SERVICE"
	default:
		return DefaultResourceType
	}
}

func NormalizeNodeType(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "SERVER", "PLATFORM":
		return "SERVER"
	default:
		return DefaultNodeType
	}
}

type BodyLimitProfile struct {
	MaxInlineResponseBody int64
	MaxRequestBodyBytes   int64
	MaxResponseBodyBytes  int64
	PolicyLabel           string
}

func DeriveBodyLimitProfile(responseMode string, supportsLargeRequest bool) BodyLimitProfile {
	mode := strings.ToUpper(strings.TrimSpace(responseMode))
	profile := BodyLimitProfile{
		MaxInlineResponseBody: 64 * 1024,
		MaxRequestBodyBytes:   64 * 1024,
		MaxResponseBodyBytes:  64 * 1024,
		PolicyLabel:           "SIP 小体量",
	}
	if supportsLargeRequest {
		profile.MaxRequestBodyBytes = 8 * 1024 * 1024
	}
	switch mode {
	case "INLINE":
		profile.PolicyLabel = "SIP 内联"
		profile.MaxResponseBodyBytes = profile.MaxInlineResponseBody
	default:
		profile.PolicyLabel = "RTP 回传"
		profile.MaxResponseBodyBytes = 512 * 1024 * 1024
		if !supportsLargeRequest {
			profile.MaxRequestBodyBytes = 512 * 1024
		}
	}
	return profile
}

func NormalizeResponseMode(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "INLINE":
		return "INLINE"
	case "RTP":
		return "RTP"
	default:
		return "AUTO"
	}
}
