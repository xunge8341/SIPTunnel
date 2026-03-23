package loadtest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"

	"siptunnel/internal/protocol/rtpfile"
)

func buildOperations(cfg Config) map[string]opFunc {
	data := makePayload(cfg.FileSize)
	rtpTransport := normalizeTransport(cfg.RTPTransport)
	effectiveChunkSize := clampRTPChunkSize(cfg.ChunkSize, rtpTransport)
	chunks := buildChunks(data, effectiveChunkSize)
	udpFrames, tcpFrames := prebuildRTPFrames(chunks)
	httpClient := newLoadtestHTTPClient(cfg.Timeout, cfg.Concurrency)
	mappingPayload := buildMappingPayload(cfg.MappingBodySize)
	return map[string]opFunc{
		"sip-command-create": func(ctx context.Context) OperationResult { return OperationResult{Err: sendSIPCommandCreate(ctx, cfg)} },
		"sip-status-receipt": func(ctx context.Context) OperationResult { return OperationResult{Err: sendSIPStatusChain(ctx, cfg)} },
		"rtp-udp-upload": func(ctx context.Context) OperationResult {
			return OperationResult{Err: sendRTPUDPFrames(ctx, cfg.RTPAddress, udpFrames, cfg.Timeout)}
		},
		"rtp-tcp-upload": func(ctx context.Context) OperationResult {
			return OperationResult{Err: sendRTPTCPFrames(ctx, cfg.RTPAddress, tcpFrames, cfg.Timeout)}
		},
		"rtp-upload": func(ctx context.Context) OperationResult {
			if rtpTransport == "TCP" {
				return OperationResult{Err: sendRTPTCPFrames(ctx, cfg.RTPAddress, tcpFrames, cfg.Timeout)}
			}
			return OperationResult{Err: sendRTPUDPFrames(ctx, cfg.RTPAddress, udpFrames, cfg.Timeout)}
		},
		"http-invoke":     func(ctx context.Context) OperationResult { return invokeHTTP(ctx, httpClient, cfg.HTTPURL) },
		"mapping-forward": func(ctx context.Context) OperationResult { return invokeMapping(ctx, httpClient, cfg, mappingPayload) },
	}
}

func newLoadtestHTTPClient(timeout time.Duration, concurrency int) *http.Client {
	maxConns := clampInt(maxInt(concurrency*2, 32), 32, 128)
	idleConnsPerHost := clampInt(maxInt(concurrency, 16), 16, 64)
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     false,
		DisableKeepAlives:     false,
		DisableCompression:    true,
		MaxIdleConns:          maxConns * 2,
		MaxIdleConnsPerHost:   idleConnsPerHost,
		MaxConnsPerHost:       maxConns,
		IdleConnTimeout:       120 * time.Second,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: 250 * time.Millisecond,
		DialContext:           dialer.DialContext,
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}

func buildMappingPayload(size int) []byte {
	if size <= 0 {
		size = 1024
	}
	payload := makePayload(size)
	body := map[string]any{
		"request_id": randomHex(16),
		"trace_id":   randomHex(16),
		"payload":    hex.EncodeToString(payload),
	}
	raw, _ := json.Marshal(body)
	return raw
}

func sendSIPCommandCreate(ctx context.Context, cfg Config) error {
	msg := map[string]any{"protocol_version": "1.0", "message_type": "command.create", "request_id": randomHex(16), "trace_id": randomHex(16), "session_id": randomHex(16), "api_code": "asset.sync", "source_system": "loadtest", "source_node": "bench", "timestamp": time.Now().UTC(), "expire_at": time.Now().UTC().Add(5 * time.Minute), "nonce": randomHex(8), "digest_alg": "sha256", "payload_digest": randomHex(16), "sign_alg": "hmac-sha256", "signature": randomHex(32), "command_id": randomHex(8), "parameters": map[string]any{"mode": "loadtest"}}
	payload, _ := json.Marshal(msg)
	resp, err := sendSIPFrame(ctx, cfg.SIPTransport, cfg.SIPAddress, payload, cfg.Timeout)
	if err != nil {
		return err
	}
	if len(resp) == 0 {
		return errors.New("empty sip response")
	}
	return nil
}

func sendSIPStatusChain(ctx context.Context, cfg Config) error {
	if err := sendSIPCommandCreate(ctx, cfg); err != nil {
		return fmt.Errorf("status-chain command.create: %w", err)
	}
	msg := map[string]any{"protocol_version": "1.0", "message_type": "task.status", "request_id": randomHex(16), "trace_id": randomHex(16), "session_id": randomHex(16), "api_code": "asset.sync", "source_system": "loadtest", "source_node": "bench", "timestamp": time.Now().UTC(), "expire_at": time.Now().UTC().Add(5 * time.Minute), "nonce": randomHex(8), "digest_alg": "sha256", "payload_digest": randomHex(16), "sign_alg": "hmac-sha256", "signature": randomHex(32), "task_id": randomHex(8), "status": "RUNNING", "progress": 66}
	payload, _ := json.Marshal(msg)
	_, err := sendSIPFrame(ctx, cfg.SIPTransport, cfg.SIPAddress, payload, cfg.Timeout)
	return err
}

func sendSIPFrame(ctx context.Context, transport, addr string, payload []byte, timeout time.Duration) ([]byte, error) {
	transport = normalizeTransport(transport)
	d := net.Dialer{Timeout: timeout}
	if transport == "UDP" {
		conn, err := d.DialContext(ctx, "udp", addr)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		if udp, ok := conn.(*net.UDPConn); ok {
			_ = udp.SetReadBuffer(loadtestSocketBufferBytes)
			_ = udp.SetWriteBuffer(loadtestSocketBufferBytes)
		}
		_ = conn.SetDeadline(time.Now().Add(timeout))
		if _, err := conn.Write(payload); err != nil {
			return nil, err
		}
		buf := make([]byte, 64*1024)
		n, err := conn.Read(buf)
		if err != nil {
			return nil, err
		}
		if body, derr := decodeSIPFrame(buf[:n]); derr == nil {
			return body, nil
		}
		return append([]byte(nil), buf[:n]...), nil
	}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
		_ = tcp.SetReadBuffer(loadtestSocketBufferBytes)
		_ = tcp.SetWriteBuffer(loadtestSocketBufferBytes)
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	frame := []byte(fmt.Sprintf("SIP-TUNNEL/1.0\r\nContent-Length: %d\r\n\r\n", len(payload)))
	frame = append(frame, payload...)
	if _, err := conn.Write(frame); err != nil {
		return nil, err
	}
	buf := make([]byte, 64*1024)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return decodeSIPFrame(buf[:n])
}

func decodeSIPFrame(data []byte) ([]byte, error) {
	raw := string(data)
	idx := strings.Index(raw, sipHeaderTerminator)
	if idx < 0 {
		return nil, errors.New("invalid sip frame")
	}
	body := data[idx+len(sipHeaderTerminator):]
	if len(body) == 0 {
		return nil, errors.New("empty sip frame body")
	}
	return body, nil
}

func invokeHTTP(ctx context.Context, client *http.Client, url string) OperationResult {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(`{"payload":"loadtest"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", randomHex(16))
	req.Header.Set("X-Trace-ID", randomHex(16))
	req.Header.Set("X-Api-Code", "asset.sync")
	req.Header.Set("X-Source-System", "loadtest")
	resp, trace, err := doHTTPRequestWithTrace(ctx, client, req)
	if err != nil {
		return OperationResult{Err: err, Trace: trace}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return OperationResult{Err: fmt.Errorf("http status %d", resp.StatusCode), Trace: trace}
	}
	return OperationResult{Trace: trace}
}

func invokeMapping(ctx context.Context, client *http.Client, cfg Config, payload []byte) OperationResult {
	targetURL := strings.TrimSpace(cfg.MappingURL)
	if targetURL == "" {
		targetURL = strings.TrimSpace(cfg.HTTPURL)
	}
	if targetURL == "" {
		return OperationResult{Err: errors.New("mapping url is empty")}
	}
	method := strings.ToUpper(strings.TrimSpace(cfg.MappingMethod))
	if method == "" {
		method = http.MethodPost
	}
	req, _ := http.NewRequestWithContext(ctx, method, targetURL, bytes.NewReader(payload))
	req.ContentLength = int64(len(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", randomHex(16))
	req.Header.Set("X-Trace-ID", randomHex(16))
	req.Header.Set("X-Api-Code", "mapping.forward")
	req.Header.Set("X-Source-System", "loadtest")
	resp, trace, err := doHTTPRequestWithTrace(ctx, client, req)
	if err != nil {
		return OperationResult{Err: err, Trace: trace}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return OperationResult{Err: fmt.Errorf("mapping status %d", resp.StatusCode), Trace: trace}
	}
	return OperationResult{Trace: trace}
}

// doHTTPRequestWithTrace captures the three Task 9 signals that matter for the
// keep-alive A/B decision: connect latency, first-byte latency, and whether the
// request reused an existing connection. Success rate still has the highest
// weight, but these transport metrics explain *why* throughput changes.
func doHTTPRequestWithTrace(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, HTTPTraceMetrics, error) {
	var trace HTTPTraceMetrics
	started := time.Now()
	var connectStart time.Time
	traceCtx := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		ConnectStart: func(_, _ string) { connectStart = time.Now() },
		ConnectDone: func(_, _ string, _ error) {
			if !connectStart.IsZero() {
				trace.ConnectLatency = time.Since(connectStart)
				trace.Sampled = true
			}
		},
		GotConn: func(info httptrace.GotConnInfo) {
			trace.ConnectionReused = info.Reused
			trace.ConnectionWasIdle = info.WasIdle
			trace.Sampled = true
		},
		GotFirstResponseByte: func() { trace.FirstByteLatency = time.Since(started); trace.Sampled = true },
	})
	req = req.Clone(traceCtx)
	resp, err := client.Do(req)
	return resp, trace, err
}

func sendRTPUDPFrames(ctx context.Context, addr string, frames [][]byte, timeout time.Duration) error {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "udp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if udp, ok := conn.(*net.UDPConn); ok {
		_ = udp.SetReadBuffer(loadtestSocketBufferBytes)
		_ = udp.SetWriteBuffer(loadtestSocketBufferBytes)
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	for _, frame := range frames {
		if len(frame) == 0 {
			continue
		}
		if _, err := conn.Write(frame); err != nil {
			return err
		}
	}
	return nil
}

func sendRTPTCPFrames(ctx context.Context, addr string, frames [][]byte, timeout time.Duration) error {
	d := net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
		_ = tcp.SetReadBuffer(loadtestSocketBufferBytes)
		_ = tcp.SetWriteBuffer(loadtestSocketBufferBytes)
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	for _, frame := range frames {
		if len(frame) == 0 {
			continue
		}
		if _, err := conn.Write(frame); err != nil {
			return err
		}
	}
	return nil
}

func marshalChunkPacket(packet rtpfile.ChunkPacket) ([]byte, error) {
	hdr, err := packet.Header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(hdr)+len(packet.Payload))
	copy(out, hdr)
	copy(out[len(hdr):], packet.Payload)
	return out, nil
}

func buildChunks(data []byte, chunkSize int) []rtpfile.ChunkPacket {
	var transferID [16]byte
	var requestID [16]byte
	var traceID [16]byte
	copy(transferID[:], randomBytes(16))
	copy(requestID[:], randomBytes(16))
	copy(traceID[:], randomBytes(16))
	chunks, _ := rtpfile.SplitFileToChunks(data, rtpfile.ChunkOptions{TransferID: transferID, RequestID: requestID, TraceID: traceID, ChunkSize: chunkSize, Extensions: []rtpfile.TLV{{Type: rtpfile.TLVTypeFileName, Value: []byte("loadtest.bin")}}})
	return chunks
}

func prebuildRTPFrames(chunks []rtpfile.ChunkPacket) (udpFrames [][]byte, tcpFrames [][]byte) {
	if len(chunks) == 0 {
		return nil, nil
	}
	udpFrames = make([][]byte, 0, len(chunks))
	tcpFrames = make([][]byte, 0, len(chunks))
	for _, c := range chunks {
		frame, err := marshalChunkPacket(c)
		if err != nil {
			continue
		}
		udpFrame := append([]byte(nil), frame...)
		udpFrames = append(udpFrames, udpFrame)
		tcpFrame := make([]byte, 4+len(frame))
		binary.BigEndian.PutUint32(tcpFrame[:4], uint32(len(frame)))
		copy(tcpFrame[4:], frame)
		tcpFrames = append(tcpFrames, tcpFrame)
	}
	return udpFrames, tcpFrames
}

func makePayload(size int) []byte {
	if size <= 0 {
		size = 1024
	}
	buf := make([]byte, size)
	for i := range buf {
		buf[i] = byte('A' + (i % 26))
	}
	sum := sha256.Sum256(buf)
	copy(buf[:min(16, len(buf))], sum[:16])
	return buf
}

func clampRTPChunkSize(chunkSize int, transport string) int {
	if chunkSize <= 0 {
		chunkSize = 64 * 1024
	}
	if normalizeTransport(transport) == "UDP" && chunkSize > loadtestUDPMaxChunkBytes {
		return loadtestUDPMaxChunkBytes
	}
	return chunkSize
}

func normalizeTransport(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "UDP") {
		return "UDP"
	}
	return "TCP"
}
