package server

import (
	"crypto/md5"
	"fmt"
	"net"
	"net/http"
	"siptunnel/internal/nodeconfig"
	"siptunnel/internal/protocol/siptext"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

func buildDeviceURI(deviceID string) string {
	id := strings.TrimSpace(deviceID)
	if id == "" {
		id = "device"
	}
	return fmt.Sprintf("sip:%s", id)
}

func buildDeviceURIForRemote(deviceID, remoteAddr string) string {
	id := strings.TrimSpace(deviceID)
	if id == "" {
		id = "device"
	}
	hostPort := strings.TrimSpace(remoteAddr)
	if hostPort == "" {
		return fmt.Sprintf("sip:%s", id)
	}
	if host, port, err := net.SplitHostPort(hostPort); err == nil {
		if port != "" {
			return fmt.Sprintf("sip:%s@%s:%s", id, host, port)
		}
		return fmt.Sprintf("sip:%s@%s", id, host)
	}
	return fmt.Sprintf("sip:%s@%s", id, hostPort)
}

func buildLocalSIPURI(local nodeconfig.LocalNodeConfig, remoteAddr string) string {
	user := strings.TrimSpace(local.NodeID)
	if user == "" {
		user = "siptunnel"
	}
	contact := advertisedSIPCallbackForRemote(local, remoteAddr)
	if strings.TrimSpace(contact) == "" {
		return fmt.Sprintf("sip:%s", user)
	}
	return fmt.Sprintf("sip:%s@%s", user, contact)
}

func buildDialogCallID(seed string, local nodeconfig.LocalNodeConfig, remoteAddr string) string {
	host := advertisedSIPHost(local, remoteAddr)
	if strings.TrimSpace(host) == "" {
		host = "siptunnel.local"
	}
	base := strings.TrimSpace(seed)
	if base == "" {
		base = "dialog"
	}
	base = strings.ReplaceAll(base, " ", "-")
	seq := atomic.AddUint64(&dialogCallIDSeq, 1)
	return fmt.Sprintf("%s-%d-%d@%s", base, time.Now().UTC().UnixNano(), seq, host)
}

func dialogTag(callID, role string) string {
	sum := md5.Sum([]byte(strings.TrimSpace(callID) + ":" + strings.TrimSpace(role)))
	return fmt.Sprintf("%x", sum[:6])
}

func parseAddressURI(v string) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return ""
	}
	if idx := strings.Index(trimmed, "<"); idx >= 0 {
		trimmed = trimmed[idx+1:]
	}
	if idx := strings.Index(trimmed, ">"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(trimmed), "sip:") {
		trimmed = "sip:" + strings.TrimPrefix(trimmed, "sip:")
	}
	if idx := strings.Index(trimmed, ";"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	return strings.TrimSpace(trimmed)
}

func parseTagFromAddressHeader(v string) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return ""
	}
	for _, part := range strings.Split(trimmed, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "tag=") {
			return strings.TrimSpace(part[4:])
		}
	}
	return ""
}

func withSIPTag(uri, tag string) string {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return ""
	}
	base := fmt.Sprintf("<%s>", strings.Trim(uri, "<>"))
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return base
	}
	return base + ";tag=" + tag
}

func branchToken(callID string, cseq int, method string) string {
	sum := md5.Sum([]byte(fmt.Sprintf("%s:%d:%s", strings.TrimSpace(callID), cseq, strings.ToUpper(strings.TrimSpace(method)))))
	return fmt.Sprintf("%x", sum[:8])
}

func dialogViaHeader(local nodeconfig.LocalNodeConfig, transport, callID string, cseq int, method, remoteAddr string) string {
	proto := strings.ToUpper(strings.TrimSpace(transport))
	if proto != "UDP" {
		proto = "TCP"
	}
	host := advertisedSIPHost(local, remoteAddr)
	if strings.TrimSpace(host) == "" {
		host = "127.0.0.1"
	}
	port := local.SIPListenPort
	if port <= 0 {
		port = 5060
	}
	return fmt.Sprintf("SIP/2.0/%s %s:%d;branch=z9hG4bK-%s;rport", proto, host, port, branchToken(callID, cseq, method))
}

func newOutboundDialogState(local nodeconfig.LocalNodeConfig, remoteAddr, remoteDeviceID, transport, callID string) sipDialogState {
	localURI := buildLocalSIPURI(local, remoteAddr)
	return sipDialogState{
		callID:        callID,
		localURI:      localURI,
		remoteURI:     buildDeviceURIForRemote(remoteDeviceID, remoteAddr),
		contactURI:    localURI,
		localTag:      dialogTag(callID, "uac"),
		remoteTarget:  strings.TrimSpace(remoteAddr),
		transport:     strings.ToUpper(strings.TrimSpace(transport)),
		nextLocalCSeq: 1,
	}
}

func fillOutboundDialogHeaders(msg *siptext.Message, state sipDialogState, local nodeconfig.LocalNodeConfig, cseq int, method string) {
	if msg == nil {
		return
	}
	msg.SetHeader("Via", dialogViaHeader(local, state.transport, state.callID, cseq, method, state.remoteTarget))
	msg.SetHeader("From", withSIPTag(state.localURI, state.localTag))
	msg.SetHeader("To", withSIPTag(state.remoteURI, state.remoteTag))
	msg.SetHeader("Call-ID", state.callID)
	msg.SetHeader("CSeq", fmt.Sprintf("%d %s", cseq, strings.ToUpper(strings.TrimSpace(method))))
	msg.SetHeader("Max-Forwards", "70")
	if strings.TrimSpace(state.contactURI) != "" {
		msg.SetHeader("Contact", fmt.Sprintf("<%s>", strings.Trim(state.contactURI, "<>")))
	}
	msg.SetHeader("Allow", "INVITE, ACK, CANCEL, BYE, OPTIONS, MESSAGE, SUBSCRIBE, NOTIFY")
	msg.SetHeader("User-Agent", "SIPTunnel-Gateway/1.0")
}

func fillInboundResponseHeaders(resp *siptext.Message, state sipDialogState) {
	if resp == nil {
		return
	}
	resp.SetHeader("From", withSIPTag(state.remoteURI, state.remoteTag))
	resp.SetHeader("To", withSIPTag(state.localURI, state.localTag))
	resp.SetHeader("Call-ID", state.callID)
	if strings.TrimSpace(state.contactURI) != "" {
		resp.SetHeader("Contact", fmt.Sprintf("<%s>", strings.Trim(state.contactURI, "<>")))
	}
	resp.SetHeader("Allow", "INVITE, ACK, CANCEL, BYE, OPTIONS, MESSAGE, SUBSCRIBE, NOTIFY")
	resp.SetHeader("Server", "SIPTunnel-Gateway/1.0")
}

func parseCSeqNumber(v string) int {
	parts := strings.Fields(strings.TrimSpace(v))
	if len(parts) == 0 {
		return 0
	}
	n, _ := strconv.Atoi(parts[0])
	return n
}

func nextDialogCSeq(state *sipDialogState, fallback int, method string) int {
	if state == nil {
		if fallback <= 0 {
			return 1
		}
		return fallback
	}
	if state.nextLocalCSeq < fallback {
		state.nextLocalCSeq = fallback
	}
	if state.nextLocalCSeq <= 0 {
		state.nextLocalCSeq = 1
	}
	cseq := state.nextLocalCSeq
	state.nextLocalCSeq++
	if strings.EqualFold(strings.TrimSpace(method), "INVITE") {
		state.inviteCSeq = cseq
	}
	return cseq
}

func dialogACKCSeq(state *sipDialogState, fallback int) int {
	if state != nil && state.inviteCSeq > 0 {
		return state.inviteCSeq
	}
	if fallback > 0 {
		return fallback
	}
	return 1
}

func formatGB28181Date(t time.Time) string {
	return t.In(time.Local).Format("2006-01-02T15:04:05.000")
}

func callIDRequest(method, callID string, cseq int) *siptext.Message {
	msg := siptext.NewRequest(method, buildDeviceURI(callID))
	msg.SetHeader("Call-ID", callID)
	msg.SetHeader("CSeq", fmt.Sprintf("%d %s", cseq, strings.ToUpper(strings.TrimSpace(method))))
	msg.SetHeader("Max-Forwards", "70")
	return msg
}
func advertisedSIPCallbackForRemote(local nodeconfig.LocalNodeConfig, remoteAddr string) string {
	host := advertisedSIPHost(local, remoteAddr)
	if host == "" || local.SIPListenPort <= 0 {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(local.SIPListenPort))
}

func advertisedSIPHost(local nodeconfig.LocalNodeConfig, remoteAddr string) string {
	staticCandidates := []string{
		strings.TrimSpace(local.SIPListenIP),
		registeredSIPUDPHost(),
		strings.TrimSpace(local.RTPListenIP),
	}
	for _, candidate := range staticCandidates {
		if candidate == "" || candidate == "0.0.0.0" || candidate == "::" {
			continue
		}
		return candidate
	}
	if host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr)); err == nil {
		if host == "127.0.0.1" || host == "::1" || strings.EqualFold(host, "localhost") {
			return "127.0.0.1"
		}
	}
	candidate := discoverRouteLocalIP(remoteAddr)
	if candidate == "" || candidate == "0.0.0.0" || candidate == "::" {
		return ""
	}
	return candidate
}

func registeredSIPUDPHost() string {
	globalSIPUDPTransport.mu.Lock()
	conn := globalSIPUDPTransport.conn
	globalSIPUDPTransport.mu.Unlock()
	if conn == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(conn.LocalAddr().String()))
	if err != nil {
		return ""
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		return ""
	}
	return host
}

func discoverRouteLocalIP(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return ""
	}
	conn, err := net.Dial("udp", remoteAddr)
	if err != nil {
		return ""
	}
	defer conn.Close()
	host, _, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return ""
	}
	if host == "0.0.0.0" || host == "::" {
		return ""
	}
	return strings.TrimSpace(host)
}

func advertisedRTPIP(local nodeconfig.LocalNodeConfig) string {
	for _, candidate := range []string{strings.TrimSpace(local.RTPListenIP), strings.TrimSpace(local.SIPListenIP)} {
		if candidate != "" && candidate != "0.0.0.0" && candidate != "::" {
			return candidate
		}
	}
	return "127.0.0.1"
}

func nodeOrZero(fn func() nodeconfig.LocalNodeConfig) nodeconfig.LocalNodeConfig {
	if fn == nil {
		return nodeconfig.LocalNodeConfig{}
	}
	return fn()
}

func shouldPreferInlineRelay(prepared *mappingForwardRequest) bool {
	if prepared == nil || prepared.TargetURL == nil {
		return false
	}
	method := strings.ToUpper(strings.TrimSpace(prepared.Method))
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	path := strings.ToLower(strings.TrimSpace(prepared.TargetURL.Path))
	if path == "/favicon.ico" {
		return true
	}
	// For browser-delivered pages, asset bodies are frequently much larger than what
	// is practical to carry in a single SIP INFO over UDP once XML framing and base64
	// expansion are applied. Keep AUTO mode so the responder can prefer RTP for the
	// actual response size instead of forcing INLINE up front.
	return false
}
