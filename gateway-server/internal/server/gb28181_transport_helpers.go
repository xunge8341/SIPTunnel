package server

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"siptunnel/internal/nodeconfig"
	"siptunnel/internal/protocol/manscdp"
	"siptunnel/internal/protocol/siptext"
	"siptunnel/internal/service/filetransfer"
	"siptunnel/internal/service/siptcp"
	"siptunnel/internal/tunnelmapping"
)

func dynamicRelayBodyWait(prepared *mappingForwardRequest, start manscdp.DeviceControl) time.Duration {
	base := boundaryResponseStartWait()
	rangeBudget := boundaryRangeResponseStartWait()
	if prepared != nil {
		if strings.TrimSpace(prepared.Headers.Get(internalRangeFetchHeader)) == "1" || strings.TrimSpace(prepared.Headers.Get("Range")) != "" {
			if rangeBudget > base {
				base = rangeBudget
			}
		}
		if prepared.ResponseHeaderTimeout > base {
			base = prepared.ResponseHeaderTimeout
		}
		if prepared.RequestTimeout > base {
			base = prepared.RequestTimeout
		}
	}
	mode := normalizeResponseMode(start.ResponseMode)
	if mode == "RTP" && base < 2*time.Minute {
		base = 2 * time.Minute
	}
	if start.ContentLength > 0 {
		bitrate := boundaryRTPBitrate()
		if bitrate <= 0 {
			bitrate = int64(rtpTargetBitrateBps)
		}
		seconds := (start.ContentLength * 8) / bitrate
		if seconds < 1 {
			seconds = 1
		}
		est := 60*time.Second + time.Duration(seconds*2)*time.Second
		if est > base {
			base = est
		}
	}
	if base > 60*time.Minute {
		base = 60 * time.Minute
	}
	if prepared != nil && prepared.TargetURL != nil {
		rangePlayback := isRangePlaybackRequest(prepared, nil, nil)
		log.Printf("gb28181 relay stage=response_wait_policy target_url=%s method=%s internal_range=%t client_range=%t range_playback=%t content_length=%d base_wait_ms=%d range_wait_ms=%d request_timeout_ms=%d response_timeout_ms=%d final_wait_ms=%d bitrate_bps=%d", prepared.TargetURL.String(), prepared.Method, strings.TrimSpace(prepared.Headers.Get(internalRangeFetchHeader)) == "1", strings.TrimSpace(prepared.Headers.Get("Range")) != "", rangePlayback, start.ContentLength, boundaryResponseStartWait().Milliseconds(), boundaryRangeResponseStartWait().Milliseconds(), prepared.RequestTimeout.Milliseconds(), prepared.ResponseHeaderTimeout.Milliseconds(), base.Milliseconds(), boundaryRTPBitrate())
	}
	return base
}

func encodeXMLHeaders(h http.Header) []manscdp.HeaderKV {
	if len(h) == 0 {
		return nil
	}
	out := make([]manscdp.HeaderKV, 0, len(h))
	for k, vals := range h {
		out = append(out, manscdp.HeaderKV{Key: textproto.CanonicalMIMEHeaderKey(k), Value: strings.Join(vals, ",")})
	}
	return out
}

func compactLargeDownloadResponseStartHeadersForUDP(h http.Header) http.Header {
	if len(h) == 0 {
		return nil
	}
	preserveOrder := []string{
		"Content-Type",
		"Content-Range",
	}
	compacted := cloneSelectedHeaders(h, preserveOrder)
	maxValueLen := 96
	for key, vals := range compacted {
		filtered := vals[:0]
		for _, v := range vals {
			trimmed := strings.TrimSpace(v)
			if trimmed == "" {
				continue
			}
			if len(trimmed) <= maxValueLen {
				filtered = append(filtered, trimmed)
			}
		}
		if len(filtered) == 0 {
			delete(compacted, key)
			continue
		}
		compacted[key] = filtered
	}
	if ct := strings.TrimSpace(compacted.Get("Content-Type")); ct == "" {
		compacted.Del("Content-Type")
	}
	return compacted
}

func compactInternalRangeResponseStartHeadersForUDP(h http.Header) http.Header {
	if len(h) == 0 {
		return nil
	}
	preserveOrder := []string{
		"Content-Range",
	}
	compacted := cloneSelectedHeaders(h, preserveOrder)
	maxValueLen := 96
	for key, vals := range compacted {
		filtered := vals[:0]
		for _, v := range vals {
			trimmed := strings.TrimSpace(v)
			if trimmed == "" {
				continue
			}
			if len(trimmed) <= maxValueLen {
				filtered = append(filtered, trimmed)
			}
		}
		if len(filtered) == 0 {
			delete(compacted, key)
			continue
		}
		compacted[key] = filtered
	}
	return compacted
}

func compactTunnelResponseHeaders(h http.Header, transport string) http.Header {
	if len(h) == 0 {
		return nil
	}
	transport = strings.ToUpper(strings.TrimSpace(transport))
	preserveOrder := []string{
		"Content-Type",
		"Content-Length",
		"Content-Encoding",
		"Cache-Control",
		"Etag",
		"Last-Modified",
		"Accept-Ranges",
		"Content-Range",
		"Content-Disposition",
	}
	if transport != "UDP" {
		return cloneSelectedHeaders(h, preserveOrder)
	}
	compacted := cloneSelectedHeaders(h, preserveOrder)
	maxValueLen := 256
	for key, vals := range compacted {
		filtered := vals[:0]
		for _, v := range vals {
			if len(strings.TrimSpace(v)) <= maxValueLen {
				filtered = append(filtered, v)
			}
		}
		if len(filtered) == 0 {
			delete(compacted, key)
			continue
		}
		compacted[key] = filtered
	}
	return compacted
}

func compactTunnelRequestHeaders(h http.Header, transport string) http.Header {
	if len(h) == 0 {
		return nil
	}
	udpTransport := strings.EqualFold(strings.TrimSpace(transport), "UDP")
	preserveOrder := []string{
		"Content-Type",
		// UDP 控制面请求不需要镜像 Content-Length：Body 长度已经由 XML 包体和下游 HTTP 构造共同决定。
		// 对于浏览器发起的登录/表单 POST，Accept 往往很长，但下游路由并不依赖它。
		// 去掉 Accept 可以直接压掉 web 登录这类 200B 级 POST 在 request_control_oversize 上只差几字节失败的问题。
		"Authorization",
		"Cookie",
		"If-None-Match",
		"If-Modified-Since",
		"Range",
		"If-Range",
		"Cache-Control",
		internalRangeFetchHeader,
		downloadProfileHeader,
		downloadTransferIDHeader,
	}
	if !udpTransport {
		preserveOrder = append(preserveOrder, "Accept")
		return cloneSelectedHeaders(h, preserveOrder)
	}
	compacted := cloneSelectedHeaders(h, preserveOrder)
	maxValueLen := 512
	for key, vals := range compacted {
		filtered := vals[:0]
		for _, v := range vals {
			trimmed := strings.TrimSpace(v)
			if trimmed == "" {
				continue
			}
			if len(trimmed) <= maxValueLen {
				filtered = append(filtered, trimmed)
			}
		}
		if len(filtered) == 0 {
			delete(compacted, key)
			continue
		}
		compacted[key] = filtered
	}
	return compacted
}

func compactTunnelRequestHeadersForUDPBudget(prepared *mappingForwardRequest, selected http.Header) http.Header {
	if prepared == nil {
		return selected
	}
	compacted := cloneHeader(selected)
	method := strings.ToUpper(strings.TrimSpace(prepared.Method))
	requestPath := ""
	if prepared.TargetURL != nil {
		requestPath = strings.TrimSpace(prepared.TargetURL.Path)
	}
	if (method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions) && len(prepared.Body) == 0 {
		compacted.Del("Content-Type")
	}
	if shouldTrimConditionalRequestHeaders(method, requestPath) {
		compacted.Del("If-None-Match")
		compacted.Del("If-Modified-Since")
		compacted.Del("Cache-Control")
	}
	if shouldTrimRangeHeaders(method, requestPath) {
		compacted.Del("Range")
		compacted.Del("If-Range")
		compacted.Del(internalRangeFetchHeader)
		compacted.Del(downloadProfileHeader)
	}
	compactContentTypeHeader(compacted)
	if cookie := strings.TrimSpace(compacted.Get("Cookie")); cookie != "" {
		trimmed := compactCookieHeaderValue(cookie, 256)
		if shouldUseAggressiveCookieCompaction(method, requestPath, len(prepared.Body)) {
			trimmed = compactCookieHeaderValue(trimmed, 96)
		}
		if strings.TrimSpace(trimmed) == "" {
			compacted.Del("Cookie")
		} else {
			compacted.Set("Cookie", trimmed)
		}
	}
	if shouldDropAuthorizationForBudget(method, requestPath, len(prepared.Body), compacted.Get("Cookie")) {
		compacted.Del("Authorization")
	}
	return compactTunnelRequestHeaders(compacted, "UDP")
}

func compactTunnelRequestHeadersForUDPSevereBudget(prepared *mappingForwardRequest, selected http.Header) http.Header {
	if prepared == nil {
		return selected
	}
	method := strings.ToUpper(strings.TrimSpace(prepared.Method))
	requestPath := ""
	if prepared.TargetURL != nil {
		requestPath = strings.TrimSpace(prepared.TargetURL.Path)
	}
	if !shouldUseAggressiveCookieCompaction(method, requestPath, len(prepared.Body)) {
		return compactTunnelRequestHeadersForUDPBudget(prepared, selected)
	}
	budgeted := compactTunnelRequestHeadersForUDPBudget(prepared, selected)
	severe := make(http.Header)
	if contentType := strings.TrimSpace(compactContentTypeHeaderValue(budgeted.Get("Content-Type"))); contentType != "" {
		severe.Set("Content-Type", contentType)
	}
	if cookie := strings.TrimSpace(budgeted.Get("Cookie")); cookie != "" {
		if trimmed := compactCookieHeaderValue(cookie, 64); trimmed != "" {
			severe.Set("Cookie", trimmed)
		}
	}
	if severe.Get("Cookie") == "" {
		if authorization := strings.TrimSpace(budgeted.Get("Authorization")); authorization != "" {
			severe.Set("Authorization", authorization)
		}
	}
	if transferID := strings.TrimSpace(budgeted.Get(downloadTransferIDHeader)); transferID != "" {
		severe.Set(downloadTransferIDHeader, transferID)
	}
	return compactTunnelRequestHeaders(severe, "UDP")
}

func headerKeySummary(h http.Header) string {
	if len(h) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(h))
	for key := range h {
		if strings.TrimSpace(key) == "" {
			continue
		}
		keys = append(keys, strings.ToLower(textproto.CanonicalMIMEHeaderKey(key)))
	}
	if len(keys) == 0 {
		return "-"
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

func compactContentTypeHeader(h http.Header) {
	if len(h) == 0 {
		return
	}
	contentType := strings.TrimSpace(compactContentTypeHeaderValue(h.Get("Content-Type")))
	if contentType == "" {
		h.Del("Content-Type")
		return
	}
	h.Set("Content-Type", contentType)
}

func compactContentTypeHeaderValue(v string) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "multipart/") {
		return trimmed
	}
	if strings.HasPrefix(lower, "application/json") {
		return "application/json"
	}
	if strings.HasPrefix(lower, "application/x-www-form-urlencoded") {
		return "application/x-www-form-urlencoded"
	}
	if idx := strings.Index(trimmed, ";"); idx >= 0 {
		return strings.TrimSpace(trimmed[:idx])
	}
	return trimmed
}

func shouldTrimConditionalRequestHeaders(method string, requestPath string) bool {
	if method == http.MethodGet || method == http.MethodHead {
		return false
	}
	path := strings.ToLower(strings.TrimSpace(requestPath))
	return strings.Contains(path, "/login") || strings.Contains(path, "/signin") || strings.Contains(path, "/cas/") || strings.Contains(path, "/auth") || strings.Contains(path, "/session")
}

func shouldTrimRangeHeaders(method string, requestPath string) bool {
	if method == http.MethodGet || method == http.MethodHead {
		return false
	}
	path := strings.ToLower(strings.TrimSpace(requestPath))
	return strings.Contains(path, "/login") || strings.Contains(path, "/signin") || strings.Contains(path, "/cas/") || strings.Contains(path, "/auth") || strings.Contains(path, "/session")
}

func shouldDropAuthorizationForBudget(method string, requestPath string, bodyLen int, cookie string) bool {
	if strings.TrimSpace(cookie) == "" || bodyLen <= 0 {
		return false
	}
	if method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch {
		return false
	}
	path := strings.ToLower(strings.TrimSpace(requestPath))
	return strings.Contains(path, "/login") || strings.Contains(path, "/signin") || strings.Contains(path, "/cas/") || strings.Contains(path, "/auth") || strings.Contains(path, "/session")
}

func shouldUseAggressiveCookieCompaction(method string, requestPath string, bodyLen int) bool {
	if bodyLen <= 0 {
		return false
	}
	if method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch {
		return false
	}
	path := strings.ToLower(strings.TrimSpace(requestPath))
	return strings.Contains(path, "/login") || strings.Contains(path, "/signin") || strings.Contains(path, "/cas/") || strings.Contains(path, "/auth") || strings.Contains(path, "/session")
}

func compactCookieHeaderValue(v string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	parts := strings.Split(v, ";")
	prioritized := make([]string, 0, len(parts))
	others := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		name := trimmed
		if idx := strings.Index(trimmed, "="); idx >= 0 {
			name = strings.TrimSpace(trimmed[:idx])
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if isImportantCookieName(name) {
			prioritized = append(prioritized, trimmed)
		} else {
			others = append(others, trimmed)
		}
	}
	ordered := append(prioritized, others...)
	kept := make([]string, 0, len(ordered))
	currentLen := 0
	for _, item := range ordered {
		addLen := len(item)
		if len(kept) > 0 {
			addLen += 2
		}
		if currentLen+addLen > maxLen {
			continue
		}
		kept = append(kept, item)
		currentLen += addLen
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, "; ")
}

func isImportantCookieName(name string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	if trimmed == "" {
		return false
	}
	importantPrefixes := []string{
		"jsessionid",
		"session",
		"sessionid",
		"phpsessid",
		"asp.net_sessionid",
		"auth",
		"token",
		"csrf",
		"xsrf",
		"castgc",
		"tgc",
		"sid",
	}
	for _, prefix := range importantPrefixes {
		if strings.HasPrefix(trimmed, prefix) || strings.Contains(trimmed, prefix) {
			return true
		}
	}
	return false
}

func cloneHeader(src http.Header) http.Header {
	if len(src) == 0 {
		return nil
	}
	out := make(http.Header, len(src))
	for k, vals := range src {
		copied := make([]string, 0, len(vals))
		for _, v := range vals {
			copied = append(copied, v)
		}
		out[k] = copied
	}
	return out
}
func compactTunnelResponseStartHeadersForUDP(h http.Header) http.Header {
	if len(h) == 0 {
		return h
	}
	// For UDP control-plane response starts, prefer the absolute minimum headers
	// required by the relay/runtime. ContentLength already has a dedicated XML field,
	// so do not mirror Content-Length again into Headers.
	preserveOrder := []string{
		"Content-Type",
		"Content-Encoding",
		"Content-Range",
	}
	compacted := cloneSelectedHeaders(h, preserveOrder)
	maxValueLen := 192
	for key, vals := range compacted {
		filtered := vals[:0]
		for _, v := range vals {
			trimmed := strings.TrimSpace(v)
			if trimmed == "" {
				continue
			}
			if len(trimmed) <= maxValueLen {
				filtered = append(filtered, trimmed)
			}
		}
		if len(filtered) == 0 {
			delete(compacted, key)
			continue
		}
		compacted[key] = filtered
	}
	return compacted
}

func cloneSelectedHeaders(src http.Header, keys []string) http.Header {
	if len(src) == 0 {
		return nil
	}
	out := make(http.Header)
	for _, key := range keys {
		canonical := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(key))
		if canonical == "" {
			continue
		}
		vals := src.Values(canonical)
		if len(vals) == 0 {
			continue
		}
		for _, v := range vals {
			out.Add(canonical, v)
		}
	}
	return out
}

func decodeXMLHeaders(items []manscdp.HeaderKV) http.Header {
	out := make(http.Header)
	for _, item := range items {
		key := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(item.Key))
		if key == "" {
			continue
		}
		out.Add(key, item.Value)
	}
	return out
}

func parseDeviceIDFromSubject(subject string) string {
	trimmed := strings.TrimSpace(subject)
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, ",")
	if len(parts) == 0 {
		return ""
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	if idx := strings.Index(last, ":"); idx >= 0 {
		last = last[:idx]
	}
	return strings.TrimSpace(last)
}

func parseDeviceIDFromURI(uri string) string {
	trimmed := strings.TrimSpace(uri)
	trimmed = strings.TrimPrefix(trimmed, "sip:")
	if idx := strings.Index(trimmed, "@"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	return strings.TrimSpace(trimmed)
}

func resolvePeerDeviceID(msg *siptext.Message) string {
	if msg == nil {
		return ""
	}
	for _, candidate := range []string{msg.Header("Contact"), msg.Header("From"), msg.Header("To"), msg.RequestURI, msg.Header("X-Device-ID")} {
		if deviceID := parseDeviceIDFromAddressLike(candidate); deviceID != "" {
			return deviceID
		}
	}
	return ""
}

func parseContactAddr(v string) string {
	trimmed := normalizeAddressLike(v)
	if idx := strings.Index(trimmed, "@"); idx >= 0 {
		return strings.TrimSpace(trimmed[idx+1:])
	}
	return strings.TrimSpace(trimmed)
}

func parseDeviceIDFromAddressLike(v string) string {
	trimmed := normalizeAddressLike(v)
	if idx := strings.Index(trimmed, "@"); idx >= 0 {
		return strings.TrimSpace(trimmed[:idx])
	}
	return parseDeviceIDFromURI(trimmed)
}

func normalizeAddressLike(v string) string {
	trimmed := strings.TrimSpace(v)
	if idx := strings.Index(trimmed, ">"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	trimmed = strings.Trim(trimmed, "<>")
	trimmed = strings.TrimPrefix(trimmed, "sip:")
	if idx := strings.Index(trimmed, ";"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	return strings.TrimSpace(trimmed)
}

func parseDeviceIDFromAddr(addr string) string {
	if host, _, err := net.SplitHostPort(strings.TrimSpace(addr)); err == nil {
		return strings.TrimSpace(host)
	}
	return strings.TrimSpace(addr)
}

func registrationKey(remoteAddr, deviceID string) string {
	if strings.TrimSpace(deviceID) != "" {
		return strings.TrimSpace(deviceID)
	}
	return strings.TrimSpace(remoteAddr)
}

func parseRelaySDP(body []byte) (string, int, string) {
	var ip, deviceID string
	port := 0
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "c=IN IP4 "):
			ip = strings.TrimSpace(strings.TrimPrefix(line, "c=IN IP4 "))
		case strings.HasPrefix(line, "m=video "):
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				port, _ = strconv.Atoi(parts[1])
			}
		case strings.HasPrefix(strings.ToLower(line), "a=deviceid:"):
			deviceID = strings.TrimSpace(line[len("a=deviceid:"):])
		}
	}
	return ip, port, deviceID
}

func transportFromVia(via string) string {
	upper := strings.ToUpper(strings.TrimSpace(via))
	if strings.Contains(upper, "/UDP") {
		return "UDP"
	}
	return "TCP"
}

func buildVirtualTargetURL(mapping tunnelmapping.TunnelMapping, requestPath, rawQuery string) string {
	base := &url.URL{Scheme: "http", Host: net.JoinHostPort(strings.TrimSpace(mapping.RemoteTargetIP), strconv.Itoa(mapping.RemoteTargetPort)), Path: joinURLPath(mapping.RemoteBasePath, requestPath), RawQuery: strings.TrimSpace(rawQuery)}
	return base.String()
}

func joinURLPath(basePath, requestPath string) string {
	base := strings.TrimSpace(basePath)
	if base == "" {
		base = "/"
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	req := strings.TrimSpace(requestPath)
	if req == "" || req == "/" {
		return base
	}
	if !strings.HasPrefix(req, "/") {
		req = "/" + req
	}
	return strings.TrimRight(base, "/") + req
}

func sendSIPPayloadNoResponse(ctx context.Context, transport, remoteAddr string, payload []byte, local nodeconfig.LocalNodeConfig, portPool filetransfer.RTPPortPool, requestID string) error {
	_ = portPool
	_ = requestID
	transport = strings.ToUpper(strings.TrimSpace(transport))
	switch transport {
	case "UDP":
		return SendSIPUDPNoResponse(remoteAddr, payload)
	default:
		dialer := net.Dialer{Timeout: tunnelRelayTimeout}
		if ip := net.ParseIP(strings.TrimSpace(local.SIPListenIP)); ip != nil && !ip.IsUnspecified() {
			dialer.LocalAddr = &net.TCPAddr{IP: ip}
		}
		conn, err := dialer.DialContext(ctx, "tcp", remoteAddr)
		if err != nil {
			return err
		}
		defer conn.Close()
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.SetNoDelay(true)
			_ = tcp.SetKeepAlive(true)
			_ = tcp.SetKeepAlivePeriod(30 * time.Second)
		}
		if err := conn.SetDeadline(time.Now().Add(tunnelRelayTimeout)); err != nil {
			return err
		}
		_, err = conn.Write(siptcp.Encode(payload))
		return err
	}
}
