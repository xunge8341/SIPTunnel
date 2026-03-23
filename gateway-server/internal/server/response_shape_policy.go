package server

import (
	"net/http"
	"strings"

	"siptunnel/internal/tunnelmapping"
)

type responseShape string

const (
	responseShapeTinyControl     responseShape = "tiny_control"
	responseShapeSmallPageData   responseShape = "small_page_data"
	responseShapeUncertainStream responseShape = "uncertain_streaming"
	responseShapeBulkDownload    responseShape = "bulk_download"
)

func classifyResponseShape(prepared *mappingForwardRequest, resp *http.Response, contentLength int64, inlineBudget int64) responseShape {
	if prepared == nil {
		return responseShapeSmallPageData
	}
	method := strings.ToUpper(strings.TrimSpace(prepared.Method))
	if method != http.MethodGet && method != http.MethodHead {
		return responseShapeBulkDownload
	}
	path := ""
	query := ""
	if prepared.TargetURL != nil {
		path = strings.ToLower(strings.TrimSpace(prepared.TargetURL.Path))
		query = strings.ToLower(strings.TrimSpace(prepared.TargetURL.RawQuery))
	}
	if strings.Contains(path, "socket.io") || strings.Contains(query, "transport=polling") || strings.Contains(path, "/events") || strings.Contains(path, "/stream") {
		return responseShapeUncertainStream
	}
	ct := ""
	if resp != nil {
		ct = strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
		if strings.Contains(strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Disposition"))), "attachment") {
			return responseShapeBulkDownload
		}
		if strings.EqualFold(strings.TrimSpace(resp.Header.Get("Transfer-Encoding")), "chunked") || strings.Contains(ct, "text/event-stream") {
			return responseShapeUncertainStream
		}
	}
	if strings.Contains(ct, "application/octet-stream") || strings.Contains(path, "/download") || strings.Contains(path, "/export") || strings.Contains(query, "download=") {
		return responseShapeBulkDownload
	}
	if contentLength == 0 || method == http.MethodHead {
		return responseShapeTinyControl
	}
	if isTextualResponseType(ct) {
		if contentLength > 0 && contentLength <= minInt64(inlineBudget, 256) {
			return responseShapeTinyControl
		}
		if contentLength > 0 && (inlineBudget <= 0 || contentLength <= inlineBudget) {
			return responseShapeSmallPageData
		}
		if contentLength < 0 {
			return responseShapeUncertainStream
		}
	}
	if contentLength > 0 && inlineBudget > 0 && contentLength <= inlineBudget {
		return responseShapeSmallPageData
	}
	return responseShapeBulkDownload
}

func isTextualResponseType(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if ct == "" {
		return false
	}
	return strings.HasPrefix(ct, "text/") || strings.Contains(ct, "json") || strings.Contains(ct, "xml") || strings.Contains(ct, "javascript") || strings.Contains(ct, "x-www-form-urlencoded")
}

func preferredResponseShape(mapping tunnelmapping.TunnelMapping) responseShape {
	if normalizeResponseMode(mapping.ResponseMode) == responseModeRTP {
		return responseShapeBulkDownload
	}
	return responseShapeSmallPageData
}

func minInt64(a, b int64) int64 {
	if a <= 0 {
		return b
	}
	if b <= 0 || a < b {
		return a
	}
	return b
}
