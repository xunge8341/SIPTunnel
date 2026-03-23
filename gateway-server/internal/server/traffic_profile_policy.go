package server

import (
	"net/http"
	"strings"
)

type requestTrafficProfile string

const (
	trafficProfileGeneric       requestTrafficProfile = "generic"
	trafficProfileRangePlayback requestTrafficProfile = "range_playback"
)

func classifyTrafficProfile(prepared *mappingForwardRequest, req *http.Request, resp *http.Response) requestTrafficProfile {
	if isRangePlaybackRequest(prepared, req, resp) {
		return trafficProfileRangePlayback
	}
	return trafficProfileGeneric
}

func isRangePlaybackRequest(prepared *mappingForwardRequest, req *http.Request, resp *http.Response) bool {
	if hasPlaybackIntent(prepared) {
		return true
	}
	if !hasClientRangeHeader(prepared, req) {
		return false
	}
	return isLikelyPlaybackTarget(prepared, req, resp)
}

func hasPlaybackIntent(prepared *mappingForwardRequest) bool {
	if prepared == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(prepared.Headers.Get(playbackIntentHeader)), "1")
}

func hasAnyRangeHeader(prepared *mappingForwardRequest, req *http.Request) bool {
	if hasClientRangeHeader(prepared, req) {
		return true
	}
	return prepared != nil && strings.TrimSpace(prepared.Headers.Get(internalRangeFetchHeader)) == "1"
}

func hasClientRangeHeader(prepared *mappingForwardRequest, req *http.Request) bool {
	if prepared != nil && strings.TrimSpace(prepared.Headers.Get("Range")) != "" {
		return true
	}
	if req != nil && strings.TrimSpace(req.Header.Get("Range")) != "" {
		return true
	}
	return false
}

func isLikelyPlaybackTarget(prepared *mappingForwardRequest, req *http.Request, resp *http.Response) bool {
	for _, path := range []string{requestPath(prepared, req)} {
		lower := strings.ToLower(strings.TrimSpace(path))
		for _, suffix := range []string{".mp4", ".m4s", ".m4v", ".mov", ".webm", ".mp3", ".aac", ".flac", ".wav", ".ts"} {
			if strings.HasSuffix(lower, suffix) {
				return true
			}
		}
	}
	ct := responseContentType(resp)
	if ct == "" && prepared != nil {
		ct = strings.ToLower(strings.TrimSpace(prepared.Headers.Get("Accept")))
	}
	if strings.HasPrefix(ct, "video/") || strings.HasPrefix(ct, "audio/") || strings.Contains(ct, "mp4") || strings.Contains(ct, "mpegurl") {
		return true
	}
	if prepared != nil {
		accept := strings.ToLower(strings.TrimSpace(prepared.Headers.Get("Accept")))
		if strings.Contains(accept, "video/") || strings.Contains(accept, "audio/") {
			return true
		}
	}
	if hasClientRangeHeader(prepared, req) && !responseSuggestsAttachment(resp) {
		path := strings.TrimSpace(requestPath(prepared, req))
		if path == "" || path == "/" {
			return true
		}
	}
	return false
}

func responseSuggestsAttachment(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	cd := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Disposition")))
	return strings.Contains(cd, "attachment")
}

func requestPath(prepared *mappingForwardRequest, req *http.Request) string {
	if prepared != nil && prepared.TargetURL != nil {
		return prepared.TargetURL.Path
	}
	if req != nil && req.URL != nil {
		return req.URL.Path
	}
	return ""
}

func responseContentType(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
}
