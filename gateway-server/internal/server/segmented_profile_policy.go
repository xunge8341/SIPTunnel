package server

import (
	"net/http"
	"strings"

	"siptunnel/internal/tunnelmapping"
)

// explicitSegmentedDownloadProfile 统一解析内部分段子请求显式带回来的 profile 头。
// 这样“segment child 继续沿用父请求策略”的规则只有一个事实来源，避免 lane 判定和下载计划各写一套。
func explicitSegmentedDownloadProfile(prepared *mappingForwardRequest) (segmentedDownloadProfile, bool) {
	if prepared == nil || prepared.Headers == nil {
		return segmentedDownloadProfile{}, false
	}
	switch strings.TrimSpace(prepared.Headers.Get(downloadProfileHeader)) {
	case "boundary-http":
		return boundaryHTTPSegmentedDownloadProfile(), true
	case "standard-http":
		return standardSegmentedDownloadProfile(), true
	case "boundary-rtp":
		return boundarySegmentedDownloadProfile(), true
	case "generic-rtp":
		return genericRTPSegmentedDownloadProfile(), true
	default:
		return segmentedDownloadProfile{}, false
	}
}

// segmentedDownloadProfileForResponse 按真实响应上下文选择当前下载策略族。
// 优先级固定为：显式 child-profile > generic 大下载 > RTP 边界 > secure-boundary HTTP > 标准兼容 fallback。
func segmentedDownloadProfileForResponse(prepared *mappingForwardRequest, req *http.Request, resp *http.Response) segmentedDownloadProfile {
	if profile, ok := explicitSegmentedDownloadProfile(prepared); ok {
		return profile
	}
	if isGenericLargeDownloadCandidate(prepared, req, resp) {
		return genericRTPSegmentedDownloadProfile()
	}
	mode := ""
	if resp != nil {
		mode = strings.TrimSpace(resp.Header.Get("X-Siptunnel-Response-Mode"))
	}
	if strings.EqualFold(mode, "RTP") {
		return boundarySegmentedDownloadProfile()
	}
	if currentTransportTuning().IsSecureBoundary() {
		return boundaryHTTPSegmentedDownloadProfile()
	}
	return standardSegmentedDownloadProfile()
}

// segmentedDownloadProfileForLane 在还没有响应体时，给控制面 lane / child 并发使用同一套策略家族判断。
func segmentedDownloadProfileForLane(prepared *mappingForwardRequest, mapping tunnelmapping.TunnelMapping) segmentedDownloadProfile {
	if profile, ok := explicitSegmentedDownloadProfile(prepared); ok {
		return profile
	}
	if normalizeResponseMode(mapping.ResponseMode) == responseModeRTP {
		return boundarySegmentedDownloadProfile()
	}
	if currentTransportTuning().IsSecureBoundary() {
		return boundaryHTTPSegmentedDownloadProfile()
	}
	return standardSegmentedDownloadProfile()
}
