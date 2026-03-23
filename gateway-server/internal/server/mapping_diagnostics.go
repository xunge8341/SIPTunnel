package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"siptunnel/internal/tunnelmapping"
)

const mappingDiagnosticLargeBodyThreshold int64 = 256 * 1024

func shouldLogMappingRequestPlan(req *http.Request, mapping tunnelmapping.TunnelMapping) bool {
	if req == nil {
		return false
	}
	switch req.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	if req.ContentLength >= mappingDiagnosticLargeBodyThreshold {
		return true
	}
	if mapping.MaxRequestBodyBytes >= mappingDiagnosticLargeBodyThreshold || mapping.MaxResponseBodyBytes >= (4*mappingDiagnosticLargeBodyThreshold) {
		return true
	}
	return false
}

func mappingForwarderModeName(f mappingForwarder) string {
	switch f.(type) {
	case directHTTPMappingForwarder:
		return "direct-http"
	case tunneledHTTPMappingForwarder:
		return "gb28181-tunnel"
	default:
		if f == nil {
			return "unknown"
		}
		return fmt.Sprintf("%T", f)
	}
}

func mappingLocalEndpoint(mapping tunnelmapping.TunnelMapping) string {
	host := strings.TrimSpace(mapping.LocalBindIP)
	if host == "" {
		host = "0.0.0.0"
	}
	return host + ":" + strconv.Itoa(mapping.LocalBindPort)
}

func mappingTargetEndpoint(mapping tunnelmapping.TunnelMapping) string {
	host := strings.TrimSpace(mapping.RemoteTargetIP)
	if host == "" {
		host = "<unset>"
	}
	base := strings.TrimSpace(mapping.RemoteBasePath)
	if base == "" {
		base = "/"
	}
	return host + ":" + strconv.Itoa(mapping.RemoteTargetPort) + base
}

func mappingPreparedBodyMode(prepared *mappingForwardRequest) string {
	if prepared == nil {
		return "unknown"
	}
	switch {
	case len(prepared.Body) > 0:
		return "buffered"
	case prepared.BodyStream != nil && prepared.BodyStream != http.NoBody:
		return "streamed"
	default:
		return "empty"
	}
}

func mappingPreparedBodyBytes(prepared *mappingForwardRequest) int64 {
	if prepared == nil {
		return 0
	}
	if len(prepared.Body) > 0 {
		return int64(len(prepared.Body))
	}
	if prepared.BodyContentLength > 0 {
		return prepared.BodyContentLength
	}
	return 0
}

func mappingRequestContentLength(req *http.Request) int64 {
	if req == nil {
		return -1
	}
	return req.ContentLength
}
