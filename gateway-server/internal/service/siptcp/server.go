package siptcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type MessageHandler interface {
	HandleMessage(ctx context.Context, meta ConnectionMeta, payload []byte) ([]byte, error)
}

type MessageHandlerFunc func(ctx context.Context, meta ConnectionMeta, payload []byte) ([]byte, error)

func (f MessageHandlerFunc) HandleMessage(ctx context.Context, meta ConnectionMeta, payload []byte) ([]byte, error) {
	return f(ctx, meta, payload)
}

type Config struct {
	ListenAddress        string
	LocalBindIP          string
	LocalBindPort        int
	ReadTimeout          time.Duration
	WriteTimeout         time.Duration
	IdleTimeout          time.Duration
	MaxMessageBytes      int
	TCPKeepAliveEnabled  bool
	TCPKeepAliveInterval time.Duration
	TCPReadBufferBytes   int
	TCPWriteBufferBytes  int
	MaxConnections       int
}

type ConnectionMeta struct {
	ConnectionID string
	RemoteAddr   string
	LocalAddr    string
	Transport    string
}

type Server struct {
	cfg      Config
	handler  MessageHandler
	logger   *slog.Logger
	metrics  *ConnectionMetrics
	listener net.Listener
	closing  atomic.Bool
	connSeq  atomic.Uint64
	mu       sync.Mutex
	conns    map[string]net.Conn
	wg       sync.WaitGroup
}

func NewServer(cfg Config, handler MessageHandler, logger *slog.Logger, metrics *ConnectionMetrics) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if metrics == nil {
		metrics = &ConnectionMetrics{}
	}
	if cfg.MaxConnections <= 0 {
		cfg.MaxConnections = 1024
	}
	if cfg.MaxMessageBytes <= 0 {
		cfg.MaxMessageBytes = 64 * 1024
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = 5 * time.Second
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = 5 * time.Second
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = time.Minute
	}
	if cfg.ListenAddress == "" {
		cfg.ListenAddress = "0.0.0.0:5060"
	}
	return &Server{cfg: cfg, handler: handler, logger: logger, metrics: metrics, conns: map[string]net.Conn{}}
}

func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.ListenAddress)
	if err != nil {
		return err
	}
	s.listener = ln
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.acceptLoop(ctx)
	}()
	return nil
}

func (s *Server) Address() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Server) MetricsSnapshot() Snapshot {
	return s.metrics.Snapshot()
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.closing.Store(true)
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.mu.Lock()
	for _, conn := range s.conns {
		_ = conn.Close()
	}
	s.mu.Unlock()
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.closing.Load() || errors.Is(err, net.ErrClosed) {
				return
			}
			s.metrics.OnConnectionError()
			continue
		}
		if s.metrics.Snapshot().CurrentConnections >= int64(s.cfg.MaxConnections) {
			s.metrics.OnConnectionError()
			s.logger.Warn("sip tcp connection refused: max connections reached", "transport", "tcp", "remote_addr", conn.RemoteAddr().String(), "local_addr", conn.LocalAddr().String())
			_ = conn.Close()
			continue
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			s.handleConn(ctx, c)
		}(conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	id := fmt.Sprintf("tcp-%d", s.connSeq.Add(1))
	meta := ConnectionMeta{ConnectionID: id, RemoteAddr: conn.RemoteAddr().String(), LocalAddr: conn.LocalAddr().String(), Transport: "tcp"}
	s.configureTCP(conn)
	s.metrics.OnAccepted()
	s.mu.Lock()
	s.conns[id] = conn
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, id)
		s.mu.Unlock()
		s.metrics.OnClosed()
		_ = conn.Close()
		s.logger.Info("sip tcp connection closed", "transport", "tcp", "connection_id", id, "remote_addr", meta.RemoteAddr, "local_addr", meta.LocalAddr)
	}()

	s.logger.Info("sip tcp connection accepted", "transport", "tcp", "connection_id", id, "remote_addr", meta.RemoteAddr, "local_addr", meta.LocalAddr)

	framer := NewFramer(s.cfg.MaxMessageBytes)
	buf := make([]byte, 4096)
	lastRead := time.Now()
	for {
		if err := conn.SetReadDeadline(time.Now().Add(s.cfg.ReadTimeout)); err != nil {
			s.metrics.OnConnectionError()
			return
		}
		n, err := conn.Read(buf)
		if n > 0 {
			lastRead = time.Now()
			frames, ferr := framer.Feed(buf[:n])
			if ferr != nil {
				s.metrics.OnConnectionError()
				s.logger.Error("sip tcp frame parse failed", "error", ferr, "transport", "tcp", "connection_id", id, "remote_addr", meta.RemoteAddr, "local_addr", meta.LocalAddr)
				return
			}
			for _, frame := range frames {
				resp, herr := s.handler.HandleMessage(ctx, meta, frame)
				if herr != nil {
					s.metrics.OnConnectionError()
					s.logger.Error("sip tcp message handler failed", "error", herr, "transport", "tcp", "connection_id", id, "remote_addr", meta.RemoteAddr, "local_addr", meta.LocalAddr)
					return
				}
				if err = conn.SetWriteDeadline(time.Now().Add(s.cfg.WriteTimeout)); err != nil {
					s.metrics.OnConnectionError()
					return
				}
				if _, err = conn.Write(Encode(resp)); err != nil {
					if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
						s.metrics.OnWriteTimeout()
					}
					s.metrics.OnConnectionError()
					return
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				if time.Since(lastRead) >= s.cfg.IdleTimeout {
					s.logger.Info("sip tcp idle timeout reached", "transport", "tcp", "connection_id", id, "remote_addr", meta.RemoteAddr, "local_addr", meta.LocalAddr)
					return
				}
				s.metrics.OnReadTimeout()
				continue
			}
			s.metrics.OnConnectionError()
			return
		}
	}
}

func (s *Server) configureTCP(conn net.Conn) {
	tcp, ok := conn.(*net.TCPConn)
	if !ok {
		return
	}
	_ = tcp.SetNoDelay(true)
	if s.cfg.TCPReadBufferBytes > 0 {
		_ = tcp.SetReadBuffer(s.cfg.TCPReadBufferBytes)
	}
	if s.cfg.TCPWriteBufferBytes > 0 {
		_ = tcp.SetWriteBuffer(s.cfg.TCPWriteBufferBytes)
	}
	if s.cfg.TCPKeepAliveEnabled {
		_ = tcp.SetKeepAlive(true)
		if s.cfg.TCPKeepAliveInterval > 0 {
			_ = tcp.SetKeepAlivePeriod(s.cfg.TCPKeepAliveInterval)
		}
	}
}
