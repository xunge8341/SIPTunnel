package server

import (
	"log"
	"net"
	"net/http"
	"sync"
	"syscall"
	"time"
)

type mappingTransportKey struct {
	DialTimeout           time.Duration
	ResponseHeaderTimeout time.Duration
	DisableKeepAlives     bool
}

const (
	mappingTransportSocketBufferBytes = 256 * 1024
	mappingTransportKeepAlive         = 30 * time.Second
)

var (
	mappingTransportCache sync.Map
	mappingClientCache    sync.Map
)

func cachedMappingTransport(dialTimeout, responseHeaderTimeout time.Duration) *http.Transport {
	decision := runtimeHTTPKeepAlivePolicy(mappingForwardClientScope)
	key := mappingTransportKey{DialTimeout: dialTimeout, ResponseHeaderTimeout: responseHeaderTimeout, DisableKeepAlives: decision.Disable}
	if existing, ok := mappingTransportCache.Load(key); ok {
		return existing.(*http.Transport)
	}
	dialer := &net.Dialer{Timeout: dialTimeout, KeepAlive: mappingTransportKeepAlive, Control: tuneOutboundTCPSocket}
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: dialer.DialContext, ForceAttemptHTTP2: false, DisableCompression: true, DisableKeepAlives: false, ResponseHeaderTimeout: responseHeaderTimeout, IdleConnTimeout: 120 * time.Second, TLSHandshakeTimeout: 5 * time.Second, ExpectContinueTimeout: 1 * time.Second, MaxIdleConns: 512, MaxIdleConnsPerHost: 128, MaxConnsPerHost: 256}
	decision = ApplyRuntimeHTTPTransportMitigations(mappingForwardClientScope, transport)
	log.Printf("mapping transport stage=client_transport_init scope=%s dial_timeout_ms=%d response_header_timeout_ms=%d keep_alives=%t max_idle_conns=%d max_idle_conns_per_host=%d max_conns_per_host=%d source=%s reason=%s", mappingForwardClientScope, dialTimeout.Milliseconds(), responseHeaderTimeout.Milliseconds(), !decision.Disable, transport.MaxIdleConns, transport.MaxIdleConnsPerHost, transport.MaxConnsPerHost, decision.Source, decision.Reason)
	actual, loaded := mappingTransportCache.LoadOrStore(key, transport)
	if loaded {
		transport.CloseIdleConnections()
		return actual.(*http.Transport)
	}
	return transport
}
func cachedMappingHTTPClient(dialTimeout, responseHeaderTimeout time.Duration) *http.Client {
	decision := runtimeHTTPKeepAlivePolicy(mappingForwardClientScope)
	key := mappingTransportKey{DialTimeout: dialTimeout, ResponseHeaderTimeout: responseHeaderTimeout, DisableKeepAlives: decision.Disable}
	if existing, ok := mappingClientCache.Load(key); ok {
		return existing.(*http.Client)
	}
	client := &http.Client{Transport: cachedMappingTransport(dialTimeout, responseHeaderTimeout)}
	actual, loaded := mappingClientCache.LoadOrStore(key, client)
	if loaded {
		return actual.(*http.Client)
	}
	return client
}
func tuneOutboundTCPSocket(_, _ string, c syscall.RawConn) error {
	var sockErr error
	err := c.Control(func(fd uintptr) {
		sockErr = setTCPSocketOptions(fd, mappingTransportSocketBufferBytes, mappingTransportSocketBufferBytes)
	})
	if err != nil {
		return err
	}
	return sockErr
}
