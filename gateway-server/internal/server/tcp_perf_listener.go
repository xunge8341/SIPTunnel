package server

import (
	"net"
	"time"
)

const (
	mappingListenerSocketBufferBytes = 256 * 1024
	mappingListenerKeepAlivePeriod   = 30 * time.Second
)

type tunedTCPListener struct {
	net.Listener
	readBufferBytes  int
	writeBufferBytes int
	keepAlivePeriod  time.Duration
	noDelay          bool
}

func newTunedTCPListener(inner net.Listener) net.Listener {
	if inner == nil {
		return nil
	}
	if _, ok := inner.(*net.TCPListener); !ok {
		return inner
	}
	return &tunedTCPListener{
		Listener:         inner,
		readBufferBytes:  mappingListenerSocketBufferBytes,
		writeBufferBytes: mappingListenerSocketBufferBytes,
		keepAlivePeriod:  mappingListenerKeepAlivePeriod,
		noDelay:          true,
	}
}

func (l *tunedTCPListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	tcp, ok := conn.(*net.TCPConn)
	if !ok {
		return conn, nil
	}
	if l.noDelay {
		_ = tcp.SetNoDelay(true)
	}
	_ = tcp.SetKeepAlive(true)
	if l.keepAlivePeriod > 0 {
		_ = tcp.SetKeepAlivePeriod(l.keepAlivePeriod)
	}
	if l.readBufferBytes > 0 {
		_ = tcp.SetReadBuffer(l.readBufferBytes)
	}
	if l.writeBufferBytes > 0 {
		_ = tcp.SetWriteBuffer(l.writeBufferBytes)
	}
	return tcp, nil
}
