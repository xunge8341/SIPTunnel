package server

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/nodeconfig"
	"siptunnel/internal/protocol/sip"
	"siptunnel/internal/protocol/siptext"
	"siptunnel/internal/security"
	"siptunnel/internal/service/filetransfer"
	"siptunnel/internal/service/sipcontrol"
	"siptunnel/internal/service/siptcp"
	"siptunnel/internal/tunnelmapping"
)

const (
	tunnelRelayTimeout       = 15 * time.Second
	rawUDPResponseBufferSize = 256 * 1024
	rawUDPSocketBufferBytes  = 1 << 20
)

var rawUDPResponseBufferPool = sync.Pool{New: func() any { return make([]byte, rawUDPResponseBufferSize) }}

type tunnelHTTPRelayRequest struct {
	sip.Header
	MappingID            string              `json:"mapping_id"`
	MappingName          string              `json:"mapping_name"`
	Method               string              `json:"method"`
	TargetURL            string              `json:"target_url"`
	Headers              map[string][]string `json:"headers"`
	BodyBase64           string              `json:"body_base64"`
	ConnectTimeoutMS     int                 `json:"connect_timeout_ms"`
	RequestTimeoutMS     int                 `json:"request_timeout_ms"`
	ResponseTimeoutMS    int                 `json:"response_timeout_ms"`
	MaxResponseBodyBytes int64               `json:"max_response_body_bytes"`
	RelayMode            string              `json:"relay_mode,omitempty"`
}

type tunnelHTTPRelayResponse struct {
	sip.Header
	MappingID     string              `json:"mapping_id"`
	StatusCode    int                 `json:"status_code"`
	Headers       map[string][]string `json:"headers"`
	BodyBase64    string              `json:"body_base64"`
	FailureReason string              `json:"failure_reason"`
	DurationMS    int64               `json:"duration_ms"`
}

var globalTunnelHTTPRelayExecutor func(context.Context, tunnelHTTPRelayRequest) (tunnelHTTPRelayResponse, error)

func setGlobalTunnelHTTPRelayExecutor(fn func(context.Context, tunnelHTTPRelayRequest) (tunnelHTTPRelayResponse, error)) {
	globalTunnelHTTPRelayExecutor = fn
}

type sipHTTPRelayHandler struct{}

func NewSIPHTTPRelayHandler() sipcontrol.Handler { return sipHTTPRelayHandler{} }

func (sipHTTPRelayHandler) MessageType() string { return sip.MessageTypeHTTPForwardRequest }

func (sipHTTPRelayHandler) Handle(ctx context.Context, req sipcontrol.RequestContext, body []byte) (sipcontrol.OutboundMessage, error) {
	var msg tunnelHTTPRelayRequest
	if err := json.Unmarshal(body, &msg); err != nil {
		return sipcontrol.OutboundMessage{}, fmt.Errorf("parse http.forward.request: %w", err)
	}
	exec := globalTunnelHTTPRelayExecutor
	if exec == nil {
		return sipcontrol.OutboundMessage{}, fmt.Errorf("tunnel relay executor is not configured")
	}
	resp, err := exec(ctx, msg)
	if err != nil {
		return sipcontrol.OutboundMessage{}, err
	}
	raw, err := marshalSignedSIPPayload(resp, tunnelSigner())
	if err != nil {
		return sipcontrol.OutboundMessage{}, err
	}
	return sipcontrol.OutboundMessage{Body: raw}, nil
}

type tunneledHTTPMappingForwarder struct {
	resolvePeer      func(tunnelmapping.TunnelMapping) (*PeerBinding, error)
	localNode        func() nodeconfig.LocalNodeConfig
	mappingRelayMode func() string
	rtpPortPool      filetransfer.RTPPortPool
}

func newTunneledHTTPMappingForwarder(resolvePeer func(tunnelmapping.TunnelMapping) (*PeerBinding, error), localNode func() nodeconfig.LocalNodeConfig, mappingRelayMode func() string, rtpPortPool filetransfer.RTPPortPool) mappingForwarder {
	return tunneledHTTPMappingForwarder{resolvePeer: resolvePeer, localNode: localNode, mappingRelayMode: mappingRelayMode, rtpPortPool: rtpPortPool}
}

func (f tunneledHTTPMappingForwarder) PrepareForward(_ context.Context, mapping tunnelmapping.TunnelMapping, req *http.Request) (*mappingForwardRequest, error) {
	return prepareMappingForwardRequest(mapping, req, true)
}

func (f tunneledHTTPMappingForwarder) ExecuteForward(ctx context.Context, prepared *mappingForwardRequest) (*http.Response, error) {
	if prepared == nil {
		return nil, fmt.Errorf("nil prepared forward request")
	}
	if f.resolvePeer == nil {
		return nil, fmt.Errorf("peer binding resolver is not configured")
	}
	binding, err := f.resolvePeer(prepared.Mapping)
	if err != nil {
		return nil, fmt.Errorf("resolve peer binding: %w", err)
	}
	if binding == nil || strings.TrimSpace(binding.PeerSignalingIP) == "" || binding.PeerSignalingPort <= 0 {
		return nil, fmt.Errorf("peer signaling endpoint is not configured")
	}
	relayMode := currentMappingRelayMode(f.mappingRelayMode)
	if err := validatePreparedRelayPayload(prepared, relayMode, localNodeValue(f.localNode).NetworkMode); err != nil {
		return nil, err
	}
	transport := "TCP"
	localNode := localNodeValue(f.localNode)
	if f.localNode != nil {
		transport = strings.ToUpper(strings.TrimSpace(localNode.SIPTransport))
		if transport == "" {
			transport = "TCP"
		}
	}
	if svc := currentGB28181TunnelService(); svc != nil {
		return svc.ExecuteForward(ctx, binding, localNode, prepared.Mapping, prepared, transport)
	}

	now := time.Now().UTC()
	relayReq := tunnelHTTPRelayRequest{
		Header: sip.Header{
			ProtocolVersion: sip.ProtocolVersionV1,
			MessageType:     sip.MessageTypeHTTPForwardRequest,
			RequestID:       prepared.MappingID + "-" + fmt.Sprintf("%d", now.UnixNano()),
			TraceID:         prepared.MappingID + "-trace-" + fmt.Sprintf("%d", now.UnixNano()),
			SessionID:       prepared.MappingID,
			ApiCode:         prepared.MappingID,
			SourceSystem:    "siptunnel",
			SourceNode:      safeLocalNodeID(f.localNode),
			Timestamp:       now,
			ExpireAt:        now.Add(2 * time.Minute),
			Nonce:           fmt.Sprintf("%d", now.UnixNano()),
			DigestAlg:       "SHA256",
			SignAlg:         "HMAC-SHA256",
		},
		MappingID:            prepared.MappingID,
		MappingName:          prepared.MappingID,
		Method:               prepared.Method,
		TargetURL:            prepared.TargetURL.String(),
		Headers:              map[string][]string(prepared.Headers),
		BodyBase64:           base64.StdEncoding.EncodeToString(prepared.Body),
		ConnectTimeoutMS:     int(prepared.ConnectTimeout / time.Millisecond),
		RequestTimeoutMS:     int(prepared.RequestTimeout / time.Millisecond),
		ResponseTimeoutMS:    int(prepared.ResponseHeaderTimeout / time.Millisecond),
		MaxResponseBodyBytes: prepared.MaxResponseBodyBytes,
		RelayMode:            relayMode,
	}
	payload, err := marshalSignedSIPPayload(relayReq, tunnelSigner())
	if err != nil {
		return nil, err
	}
	remoteAddr := net.JoinHostPort(strings.TrimSpace(binding.PeerSignalingIP), fmt.Sprintf("%d", binding.PeerSignalingPort))
	rawResp, err := sendSIPPayload(ctx, transport, remoteAddr, payload, localNodeValue(f.localNode), f.rtpPortPool, relayReq.RequestID)
	if err != nil {
		return nil, fmt.Errorf("send tunneled request via %s to %s: %w", transport, remoteAddr, err)
	}
	var relayResp tunnelHTTPRelayResponse
	if err := unmarshalVerifiedSIPPayload(rawResp, &relayResp, tunnelSigner()); err != nil {
		return nil, fmt.Errorf("parse relay response: %w", err)
	}
	if relayResp.FailureReason != "" {
		return nil, errors.New(strings.TrimSpace(relayResp.FailureReason))
	}
	body, err := base64.StdEncoding.DecodeString(relayResp.BodyBase64)
	if err != nil {
		return nil, fmt.Errorf("decode relay response body: %w", err)
	}
	resp := &http.Response{
		StatusCode: relayResp.StatusCode,
		Status:     fmt.Sprintf("%d %s", relayResp.StatusCode, http.StatusText(relayResp.StatusCode)),
		Header:     http.Header(relayResp.Headers),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	if prepared.MaxResponseBodyBytes > 0 {
		resp.Body = &limitedReadCloser{ReadCloser: resp.Body, limit: prepared.MaxResponseBodyBytes}
	}
	return resp, nil
}

func safeLocalNodeID(localFn func() nodeconfig.LocalNodeConfig) string {
	if localFn == nil {
		return "local"
	}
	local := localFn()
	if strings.TrimSpace(local.NodeID) != "" {
		return strings.TrimSpace(local.NodeID)
	}
	return "local"
}

func localNodeValue(localFn func() nodeconfig.LocalNodeConfig) nodeconfig.LocalNodeConfig {
	if localFn == nil {
		return nodeconfig.LocalNodeConfig{}
	}
	return localFn()
}

func sendSIPPayload(ctx context.Context, transport, remoteAddr string, payload []byte, local nodeconfig.LocalNodeConfig, portPool filetransfer.RTPPortPool, requestID string) ([]byte, error) {
	transport = strings.ToUpper(strings.TrimSpace(transport))
	localBindIP := strings.TrimSpace(local.RTPListenIP)
	if localBindIP == "" {
		localBindIP = strings.TrimSpace(local.SIPListenIP)
	}
	if transport == "UDP" {
		if msg, err := siptext.Parse(payload); err == nil && msg != nil && msg.IsRequest {
			if len(payload) > udpControlMaxBytes() {
				callID := strings.TrimSpace(firstNonEmpty(msg.Header("Call-ID"), msg.Header("Call-Id")))
				log.Printf("sip udp stage=oversize_request_risk remote=%s method=%s call_id=%s cseq=%s sip_bytes=%d limit=%d", remoteAddr, strings.ToUpper(strings.TrimSpace(msg.Method)), callID, strings.TrimSpace(msg.Header("CSeq")), len(payload), udpControlMaxBytes())
			}
			return sendSIPPayloadUDP(ctx, remoteAddr, payload, localBindIP, 0)
		}
		if len(payload) > udpControlMaxBytes() {
			return nil, fmt.Errorf("udp raw payload oversize: bytes=%d limit=%d request_id=%s", len(payload), udpControlMaxBytes(), strings.TrimSpace(requestID))
		}
		reservedPort, releasePort, err := reserveTunneledSourcePort(portPool, requestID)
		if err != nil {
			return nil, err
		}
		defer releasePort()
		return sendSIPPayloadUDP(ctx, remoteAddr, payload, localBindIP, reservedPort)
	}
	cfg := siptcp.Config{
		ListenAddress:        remoteAddr,
		LocalBindIP:          localBindIP,
		ReadTimeout:          tunnelRelayTimeout,
		WriteTimeout:         tunnelRelayTimeout,
		MaxMessageBytes:      sip.MaxBodyBytes(),
		TCPKeepAliveEnabled:  true,
		TCPKeepAliveInterval: 30 * time.Second,
		TCPReadBufferBytes:   256 * 1024,
		TCPWriteBufferBytes:  256 * 1024,
	}
	if portPool != nil {
		reservedPort, releasePort, err := reserveTunneledSourcePort(portPool, requestID)
		if err != nil {
			return nil, err
		}
		defer releasePort()
		cfg.LocalBindPort = reservedPort
		client, err := dialSIPTCPWithLocalFallback(ctx, cfg)
		if err != nil {
			return nil, err
		}
		defer client.Close()
		return client.Send(ctx, payload)
	}
	lease, err := globalSIPClientPool.acquire(ctx, cfg)
	if err != nil {
		if !isLocalBindContextError(err) || strings.TrimSpace(cfg.LocalBindIP) == "" {
			return nil, err
		}
		fallbackCfg := cfg
		fallbackCfg.LocalBindIP = ""
		lease, err = globalSIPClientPool.acquire(ctx, fallbackCfg)
		if err != nil {
			return nil, err
		}
		cfg = fallbackCfg
		log.Printf("sip tcp stage=local_bind_fallback remote=%s request_id=%s original_bind_ip=%s", remoteAddr, strings.TrimSpace(requestID), strings.TrimSpace(localBindIP))
	}
	defer lease.Close()
	resp, err := lease.client.Send(ctx, payload)
	if err != nil {
		retryable := lease.Reused() && shouldRetrySIPTCPSend(err)
		lease.MarkBroken()
		if !retryable {
			return nil, err
		}
		retryLease, retryErr := globalSIPClientPool.acquire(ctx, cfg)
		if retryErr != nil {
			return nil, err
		}
		defer retryLease.Close()
		retryResp, retryErr := retryLease.client.Send(ctx, payload)
		if retryErr != nil {
			retryLease.MarkBroken()
			return nil, retryErr
		}
		return retryResp, nil
	}
	return resp, nil
}

func dialSIPTCPWithLocalFallback(ctx context.Context, cfg siptcp.Config) (*siptcp.TCPClient, error) {
	client, err := siptcp.Dial(ctx, cfg)
	if err == nil {
		return client, nil
	}
	if !isLocalBindContextError(err) || (strings.TrimSpace(cfg.LocalBindIP) == "" && cfg.LocalBindPort == 0) {
		return nil, err
	}
	fallback := cfg
	fallback.LocalBindIP = ""
	client, retryErr := siptcp.Dial(ctx, fallback)
	if retryErr != nil {
		return nil, err
	}
	return client, nil
}

func isLocalBindContextError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "cannot assign requested address") || strings.Contains(msg, "not valid in its context")
}

func reserveTunneledSourcePort(portPool filetransfer.RTPPortPool, requestID string) (int, func(), error) {
	if portPool == nil {
		return 0, func() {}, nil
	}
	transferID := requestTransferID(requestID)
	port, err := portPool.Allocate(transferID)
	if err != nil {
		return 0, func() {}, fmt.Errorf("allocate rtp source port for tunneled request: %w", err)
	}
	return port, func() { portPool.Release(transferID) }, nil
}

func requestTransferID(requestID string) [16]byte {
	trimmed := strings.TrimSpace(requestID)
	if trimmed == "" {
		trimmed = fmt.Sprintf("anon-%d", time.Now().UTC().UnixNano())
	}
	return md5.Sum([]byte(trimmed))
}

func sendSIPPayloadUDP(ctx context.Context, remoteAddr string, payload []byte, localBindIP string, localBindPort int) ([]byte, error) {
	if msg, err := siptext.Parse(payload); err == nil && msg != nil && msg.IsRequest {
		return SendSIPUDPAndWait(ctx, remoteAddr, payload, tunnelRelayTimeout)
	}
	return sendRawUDPAndWait(ctx, remoteAddr, payload, localBindIP, localBindPort, tunnelRelayTimeout)
}

func sendRawUDPAndWait(ctx context.Context, remoteAddr string, payload []byte, localBindIP string, localBindPort int, timeout time.Duration) ([]byte, error) {
	resolved, err := cachedResolveUDPAddr(remoteAddr)
	if err != nil {
		return nil, err
	}
	var localAddr *net.UDPAddr
	if strings.TrimSpace(localBindIP) != "" || localBindPort > 0 {
		localAddr = &net.UDPAddr{IP: net.ParseIP(strings.TrimSpace(localBindIP)), Port: localBindPort}
		if localAddr.IP == nil && strings.TrimSpace(localBindIP) != "" {
			localAddr = &net.UDPAddr{Port: localBindPort}
		}
	}
	conn, err := net.DialUDP("udp", localAddr, resolved)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if timeout <= 0 {
		timeout = tunnelRelayTimeout
	}
	_ = conn.SetReadBuffer(rawUDPSocketBufferBytes)
	_ = conn.SetWriteBuffer(rawUDPSocketBufferBytes)
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}
	if _, err := conn.Write(payload); err != nil {
		return nil, err
	}
	buf := rawUDPResponseBufferPool.Get().([]byte)
	defer rawUDPResponseBufferPool.Put(buf)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), buf[:n]...), nil
}

func currentMappingRelayMode(modeFn func() string) string {
	if modeFn == nil {
		return "AUTO"
	}
	mode := strings.ToUpper(strings.TrimSpace(modeFn()))
	if mode == "SIP_ONLY" {
		return mode
	}
	return "AUTO"
}

func validatePreparedRelayPayload(prepared *mappingForwardRequest, relayMode string, networkMode config.NetworkMode) error {
	if prepared == nil {
		return nil
	}
	if relayMode != "SIP_ONLY" && networkMode.Normalize() != config.NetworkModeSenderSIPReceiverSIP {
		return nil
	}
	maxInline := maxInlineRelayBodyBytes()
	if len(prepared.Body) > maxInline {
		log.Printf("tunnel-relay mapping_id=%s stage=request_limit_reject relay_mode=%s network_mode=%s request_body_bytes=%d sip_only_inline_limit=%d target=%s", prepared.MappingID, relayMode, networkMode.Normalize(), len(prepared.Body), maxInline, firstNonEmpty(prepared.TargetURL.String(), "<nil>"))
		return fmt.Errorf("mapping request body exceeds SIP-only relay limit: %d > %d", len(prepared.Body), maxInline)
	}
	return nil
}

func maxInlineRelayBodyBytes() int {
	limit := (sip.MaxBodyBytes() * 3) / 8
	if limit < 32*1024 {
		return 32 * 1024
	}
	return limit
}

func marshalSignedSIPPayload(v interface{}, signer security.Signer) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	delete(payload, "payload_digest")
	delete(payload, "signature")
	digestPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(digestPayload)
	payload["payload_digest"] = hex.EncodeToString(sum[:])
	signedPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	sig, err := signer.Sign(signedPayload)
	if err != nil {
		return nil, err
	}
	payload["signature"] = sig
	return json.Marshal(payload)
}

func unmarshalVerifiedSIPPayload(raw []byte, out interface{}, signer security.Signer) error {
	var header sip.Header
	if err := json.Unmarshal(raw, &header); err != nil {
		return err
	}
	if err := header.ValidateEnvelope(time.Now().UTC()); err != nil {
		return err
	}
	digestPayload, err := canonicalPayload(raw, "signature", "payload_digest")
	if err != nil {
		return err
	}
	sum := sha256.Sum256(digestPayload)
	if !strings.EqualFold(header.PayloadDigest, hex.EncodeToString(sum[:])) {
		return fmt.Errorf("payload digest mismatch")
	}
	signedPayload, err := canonicalPayload(raw, "signature")
	if err != nil {
		return err
	}
	if signer != nil && !signer.Verify(signedPayload, header.Signature) {
		return fmt.Errorf("signature verification failed")
	}
	return json.Unmarshal(raw, out)
}

func canonicalPayload(raw []byte, remove ...string) ([]byte, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	for _, field := range remove {
		delete(payload, field)
	}
	return json.Marshal(payload)
}

func tunnelSigner() security.Signer {
	secret := strings.TrimSpace(os.Getenv("GATEWAY_TUNNEL_SIGNER_SECRET"))
	if secret == "" && strings.EqualFold(strings.TrimSpace(os.Getenv("GATEWAY_ALLOW_INSECURE_DEFAULT_SIGNER")), "true") {
		secret = "siptunnel-boundary-secret"
	}
	return security.NewHMACSigner(secret)
}

func executeTunnelRelayRequest(ctx context.Context, req tunnelHTTPRelayRequest, logs *accessLogStore, local nodeconfig.LocalNodeConfig, portPool filetransfer.RTPPortPool) (tunnelHTTPRelayResponse, error) {
	started := time.Now()
	now := time.Now().UTC()
	resp := tunnelHTTPRelayResponse{Header: sip.Header{ProtocolVersion: sip.ProtocolVersionV1, MessageType: sip.MessageTypeHTTPForwardResponse, RequestID: req.Header.RequestID, TraceID: req.Header.TraceID, SessionID: req.Header.SessionID, ApiCode: req.Header.ApiCode, SourceSystem: "siptunnel", SourceNode: req.Header.SourceNode, Timestamp: now, ExpireAt: now.Add(2 * time.Minute), Nonce: req.Header.Nonce + "-resp", DigestAlg: "SHA256", SignAlg: "HMAC-SHA256"}, MappingID: req.MappingID}
	targetURL, err := url.Parse(strings.TrimSpace(req.TargetURL))
	if err != nil {
		resp.StatusCode = http.StatusBadGateway
		resp.FailureReason = fmt.Sprintf("invalid target url: %v", err)
		return resp, nil
	}
	body, err := base64.StdEncoding.DecodeString(req.BodyBase64)
	if err != nil {
		resp.StatusCode = http.StatusBadGateway
		resp.FailureReason = fmt.Sprintf("decode body: %v", err)
		return resp, nil
	}
	prepared := &mappingForwardRequest{MappingID: req.MappingID, Method: req.Method, TargetURL: targetURL, Headers: http.Header(req.Headers), Body: body, ConnectTimeout: time.Duration(req.ConnectTimeoutMS) * time.Millisecond, RequestTimeout: time.Duration(req.RequestTimeoutMS) * time.Millisecond, ResponseHeaderTimeout: time.Duration(req.ResponseTimeoutMS) * time.Millisecond, MaxResponseBodyBytes: req.MaxResponseBodyBytes}
	log.Printf("tunnel-relay mapping_id=%s stage=peer_request_received source_node=%s relay_mode=%s method=%s target_url=%s body_bytes=%d request_timeout_ms=%d response_timeout_ms=%d max_response_body_bytes=%d", req.MappingID, strings.TrimSpace(req.Header.SourceNode), strings.TrimSpace(req.RelayMode), req.Method, targetURL.String(), len(body), prepared.RequestTimeout.Milliseconds(), prepared.ResponseHeaderTimeout.Milliseconds(), prepared.MaxResponseBodyBytes)
	upstream, err := executePreparedForward(ctx, prepared)
	if err != nil {
		resp.StatusCode = http.StatusBadGateway
		resp.FailureReason = err.Error()
		resp.DurationMS = time.Since(started).Milliseconds()
		if logs != nil {
			logs.Add(AccessLogEntry{ID: req.Header.RequestID + "-peer", OccurredAt: formatTimestamp(now), MappingName: req.MappingName, SourceIP: req.Header.SourceNode, Method: req.Method, Path: targetURL.RequestURI(), StatusCode: http.StatusBadGateway, DurationMS: resp.DurationMS, FailureReason: resp.FailureReason, RequestID: req.Header.RequestID, TraceID: req.Header.TraceID})
		}
		return resp, nil
	}
	defer upstream.Body.Close()
	rawBody, err := io.ReadAll(upstream.Body)
	if err != nil {
		resp.StatusCode = http.StatusBadGateway
		resp.FailureReason = err.Error()
	} else if (strings.ToUpper(strings.TrimSpace(req.RelayMode)) == "SIP_ONLY" || local.NetworkMode.Normalize() == config.NetworkModeSenderSIPReceiverSIP) && len(rawBody) > maxInlineRelayBodyBytes() {
		resp.StatusCode = http.StatusBadGateway
		resp.FailureReason = fmt.Sprintf("mapping response body exceeds SIP-only relay limit: %d > %d", len(rawBody), maxInlineRelayBodyBytes())
	} else {
		resp.StatusCode = upstream.StatusCode
		resp.Headers = map[string][]string(upstream.Header)
		resp.BodyBase64 = base64.StdEncoding.EncodeToString(rawBody)
	}
	resp.DurationMS = time.Since(started).Milliseconds()
	if logs != nil {
		logs.Add(AccessLogEntry{ID: req.Header.RequestID + "-peer", OccurredAt: formatTimestamp(now), MappingName: req.MappingName, SourceIP: req.Header.SourceNode, Method: req.Method, Path: targetURL.RequestURI(), StatusCode: maxIntVal(resp.StatusCode, http.StatusBadGateway), DurationMS: resp.DurationMS, FailureReason: resp.FailureReason, RequestID: req.Header.RequestID, TraceID: req.Header.TraceID})
	}
	return resp, nil
}
