package siptcp

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"
)

type Client interface {
	Send(ctx context.Context, payload []byte) ([]byte, error)
	Close() error
}

type TCPClient struct {
	cfg     Config
	conn    net.Conn
	framer  *Framer
	readBuf []byte
}

func Dial(ctx context.Context, cfg Config) (*TCPClient, error) {
	dialTimeout := cfg.ReadTimeout
	if dialTimeout <= 0 {
		dialTimeout = 5 * time.Second
	}
	dialer := net.Dialer{Timeout: dialTimeout}
	if cfg.LocalBindPort > 0 || cfg.LocalBindIP != "" {
		addr := &net.TCPAddr{Port: cfg.LocalBindPort}
		if ip := net.ParseIP(cfg.LocalBindIP); ip != nil {
			addr.IP = ip
		}
		dialer.LocalAddr = addr
	}
	conn, err := dialer.DialContext(ctx, "tcp", cfg.ListenAddress)
	if err != nil {
		return nil, err
	}
	client := &TCPClient{cfg: cfg, conn: conn, framer: NewFramer(cfg.MaxMessageBytes), readBuf: make([]byte, initialClientReadBufferSize(cfg.MaxMessageBytes))}
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
		if cfg.TCPKeepAliveEnabled {
			_ = tcp.SetKeepAlive(true)
			if cfg.TCPKeepAliveInterval > 0 {
				_ = tcp.SetKeepAlivePeriod(cfg.TCPKeepAliveInterval)
			}
		}
		if cfg.TCPReadBufferBytes > 0 {
			_ = tcp.SetReadBuffer(cfg.TCPReadBufferBytes)
		}
		if cfg.TCPWriteBufferBytes > 0 {
			_ = tcp.SetWriteBuffer(cfg.TCPWriteBufferBytes)
		}
	}
	return client, nil
}

func initialClientReadBufferSize(maxMessageBytes int) int {
	switch {
	case maxMessageBytes >= 64*1024:
		return 64 * 1024
	case maxMessageBytes >= 32*1024:
		return 32 * 1024
	case maxMessageBytes >= 8*1024:
		return 8 * 1024
	default:
		return 4096
	}
}

func (c *TCPClient) Send(ctx context.Context, payload []byte) ([]byte, error) {
	if c == nil || c.conn == nil {
		return nil, net.ErrClosed
	}
	writeTimeout := c.cfg.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = 5 * time.Second
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return nil, err
	}
	if err := writeEncodedPayload(c.conn, payload); err != nil {
		return nil, err
	}
	buf := c.readBuf
	if len(buf) == 0 {
		buf = make([]byte, 4096)
		c.readBuf = buf
	}
	readTimeout := c.cfg.ReadTimeout
	if readTimeout <= 0 {
		readTimeout = 5 * time.Second
	}
	for {
		if err := c.conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
			return nil, err
		}
		n, err := c.conn.Read(buf)
		if n > 0 {
			frames, ferr := c.framer.Feed(buf[:n])
			if ferr != nil {
				return nil, ferr
			}
			if len(frames) > 0 {
				return frames[0], nil
			}
		}
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil, err
			}
			return nil, wrapReadResponseError(err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
}

func (c *TCPClient) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func writeEncodedPayload(conn net.Conn, payload []byte) error {
	if conn == nil {
		return net.ErrClosed
	}
	var header [96]byte
	buf := header[:0]
	buf = append(buf, "SIP-TUNNEL/1.0\r\nContent-Length: "...)
	buf = strconv.AppendInt(buf, int64(len(payload)), 10)
	buf = append(buf, "\r\n\r\n"...)
	buffers := net.Buffers{buf, payload}
	_, err := buffers.WriteTo(conn)
	return err
}

func wrapReadResponseError(err error) error {
	if err == nil {
		return nil
	}
	return &net.OpError{Op: "read", Net: "tcp", Err: err}
}
