package server

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/protocol/siptext"
)

const sipUDPSocketBufferBytes = 4 << 20

type sipUDPTransport struct {
	mu      sync.Mutex
	conn    net.PacketConn
	waiters map[string]chan []byte
	lanes   map[string]chan struct{}
}

var globalSIPUDPTransport = &sipUDPTransport{waiters: map[string]chan []byte{}, lanes: map[string]chan struct{}{}}

func acquireSIPUDPLane(ctx context.Context, remoteAddr string) (func(), error) {
	key := strings.ToLower(strings.TrimSpace(remoteAddr))
	if key == "" {
		return func() {}, nil
	}
	globalSIPUDPTransport.mu.Lock()
	if globalSIPUDPTransport.lanes == nil {
		globalSIPUDPTransport.lanes = map[string]chan struct{}{}
	}
	lane, ok := globalSIPUDPTransport.lanes[key]
	if !ok {
		lane = make(chan struct{}, 1)
		globalSIPUDPTransport.lanes[key] = lane
	}
	globalSIPUDPTransport.mu.Unlock()
	select {
	case lane <- struct{}{}:
		return func() {
			select {
			case <-lane:
			default:
			}
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func RegisterSIPUDPTransport(conn net.PacketConn) {
	if udp, ok := conn.(*net.UDPConn); ok {
		_ = udp.SetReadBuffer(sipUDPSocketBufferBytes)
		_ = udp.SetWriteBuffer(sipUDPSocketBufferBytes)
	}
	globalSIPUDPTransport.mu.Lock()
	defer globalSIPUDPTransport.mu.Unlock()
	globalSIPUDPTransport.conn = conn
	if globalSIPUDPTransport.waiters == nil {
		globalSIPUDPTransport.waiters = map[string]chan []byte{}
	}
	if globalSIPUDPTransport.lanes == nil {
		globalSIPUDPTransport.lanes = map[string]chan struct{}{}
	}
}

func UnregisterSIPUDPTransport(conn net.PacketConn) {
	globalSIPUDPTransport.mu.Lock()
	defer globalSIPUDPTransport.mu.Unlock()
	if globalSIPUDPTransport.conn == conn {
		globalSIPUDPTransport.conn = nil
	}
}

func TryHandleSIPUDPResponse(remoteAddr string, payload []byte) bool {
	msg, err := siptext.Parse(payload)
	if err != nil || msg == nil || msg.IsRequest {
		return false
	}
	key := sipUDPTransactionKey(remoteAddr, msg)
	globalSIPUDPTransport.mu.Lock()
	ch, ok := globalSIPUDPTransport.waiters[key]
	if ok {
		delete(globalSIPUDPTransport.waiters, key)
	}
	globalSIPUDPTransport.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- append([]byte(nil), payload...):
	default:
	}
	return true
}

func SendSIPUDPAndWait(ctx context.Context, remoteAddr string, payload []byte, timeout time.Duration) ([]byte, error) {
	msg, err := siptext.Parse(payload)
	if err != nil {
		return nil, fmt.Errorf("parse outgoing sip payload: %w", err)
	}
	if msg == nil || !msg.IsRequest {
		return nil, fmt.Errorf("outgoing sip udp payload must be a request")
	}
	resolved, err := cachedResolveUDPAddr(remoteAddr)
	if err != nil {
		return nil, err
	}
	releaseLane, err := acquireSIPUDPLane(ctx, resolved.String())
	if err != nil {
		return nil, err
	}
	defer releaseLane()

	key := sipUDPTransactionKey(resolved.String(), msg)
	ch := make(chan []byte, 1)

	globalSIPUDPTransport.mu.Lock()
	if globalSIPUDPTransport.conn == nil {
		globalSIPUDPTransport.mu.Unlock()
		return nil, fmt.Errorf("sip udp transport is not registered")
	}
	globalSIPUDPTransport.waiters[key] = ch
	conn := globalSIPUDPTransport.conn
	globalSIPUDPTransport.mu.Unlock()

	defer func() {
		globalSIPUDPTransport.mu.Lock()
		if current, ok := globalSIPUDPTransport.waiters[key]; ok && current == ch {
			delete(globalSIPUDPTransport.waiters, key)
		}
		globalSIPUDPTransport.mu.Unlock()
	}()

	if timeout <= 0 {
		timeout = tunnelRelayTimeout
	}
	waitCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	requestMethod := strings.ToUpper(strings.TrimSpace(msg.Method))
	retryDelay := 500 * time.Millisecond
	maxRetryDelay := 4 * time.Second
	retryTimer := time.NewTimer(0)
	defer retryTimer.Stop()
	for {
		select {
		case <-retryTimer.C:
			if _, err := conn.WriteTo(payload, resolved); err != nil {
				return nil, err
			}
			if requestMethod == "ACK" || requestMethod == "CANCEL" {
				retryDelay = 0
				continue
			}
			nextDelay := retryDelay
			if nextDelay <= 0 {
				nextDelay = 500 * time.Millisecond
			}
			if deadline, ok := waitCtx.Deadline(); ok {
				remaining := time.Until(deadline)
				if remaining <= 0 {
					return nil, waitCtx.Err()
				}
				if nextDelay > remaining {
					nextDelay = remaining
				}
			}
			retryTimer.Reset(nextDelay)
			if retryDelay < maxRetryDelay {
				retryDelay *= 2
				if retryDelay > maxRetryDelay {
					retryDelay = maxRetryDelay
				}
			}
		case raw := <-ch:
			return raw, nil
		case <-waitCtx.Done():
			return nil, waitCtx.Err()
		}
	}
}

func SendSIPUDPNoResponse(remoteAddr string, payload []byte) error {
	resolved, err := cachedResolveUDPAddr(remoteAddr)
	if err != nil {
		return err
	}
	releaseLane, err := acquireSIPUDPLane(context.Background(), resolved.String())
	if err != nil {
		return err
	}
	defer releaseLane()
	globalSIPUDPTransport.mu.Lock()
	conn := globalSIPUDPTransport.conn
	globalSIPUDPTransport.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("sip udp transport is not registered")
	}
	_, err = conn.WriteTo(payload, resolved)
	return err
}

func sipUDPTransactionKey(remoteAddr string, msg *siptext.Message) string {
	if msg == nil {
		return strings.TrimSpace(remoteAddr)
	}
	callID := strings.TrimSpace(firstNonEmpty(msg.Header("Call-ID"), msg.Header("Call-Id")))
	cseq := strings.ToUpper(strings.TrimSpace(msg.Header("CSeq")))
	return strings.ToLower(strings.TrimSpace(remoteAddr)) + "|" + callID + "|" + cseq
}
