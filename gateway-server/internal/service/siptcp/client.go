package siptcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

type Client interface {
	Send(ctx context.Context, payload []byte) ([]byte, error)
	Close() error
}

type TCPClient struct {
	cfg    Config
	conn   net.Conn
	framer *Framer
}

func Dial(ctx context.Context, cfg Config) (*TCPClient, error) {
	dialer := net.Dialer{Timeout: cfg.ReadTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", cfg.ListenAddress)
	if err != nil {
		return nil, err
	}
	client := &TCPClient{cfg: cfg, conn: conn, framer: NewFramer(cfg.MaxMessageBytes)}
	if tcp, ok := conn.(*net.TCPConn); ok {
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

func (c *TCPClient) Send(ctx context.Context, payload []byte) ([]byte, error) {
	if err := c.conn.SetWriteDeadline(time.Now().Add(c.cfg.WriteTimeout)); err != nil {
		return nil, err
	}
	if _, err := c.conn.Write(Encode(payload)); err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	for {
		if err := c.conn.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout)); err != nil {
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
			return nil, fmt.Errorf("read response: %w", err)
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
