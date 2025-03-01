package network

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	Version          = "1.0.0"
	DefaultPoolSize  = 5000
	MaxHeaderSize    = 4096
	ShutdownTimeout  = 30 * time.Second
	DefaultKeepAlive = 3 * time.Minute
)

var (
	ErrServerClosed    = errors.New("server: closed")
	ErrTooManyConnects = errors.New("server: too many connections")
)

type Config struct {
	Addr         string
	TLSConfig    *tls.Config
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	MaxConns     int
	Logger       Logger
}

type Server struct {
	Config     *Config
	Listener   net.Listener
	ConnPool   chan struct{}
	WorkerPool chan net.Conn
	Shutdown   chan struct{}
	Once       sync.Once
	WaitGroup  sync.WaitGroup
	Metrics    *PrometheusMetrics
	Logger     Logger
	DoneChan   chan struct{}
}

type Metrics interface {
	IncConnections()
	DecConnections()
	IncErrors()
	IncRequests()
}

func NewServer(cfg *Config) (*Server, error) {
	if cfg.MaxConns == 0 {
		cfg.MaxConns = DefaultPoolSize
	}

	listener, err := createListener(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create listener: %w", err)
	}
	m := NewPrometheusMetrics()
	StartMetricsServer()
	return &Server{
		Config:     cfg,
		Listener:   listener,
		ConnPool:   make(chan struct{}, cfg.MaxConns),
		WorkerPool: make(chan net.Conn, cfg.MaxConns),
		Shutdown:   make(chan struct{}),
		Metrics:    m,
		Logger:     cfg.Logger,
		DoneChan:   make(chan struct{}),
	}, nil
}

func createListener(cfg *Config) (net.Listener, error) {
	if cfg.TLSConfig != nil {
		return tls.Listen("tcp", cfg.Addr, cfg.TLSConfig)
	}
	return net.Listen("tcp", cfg.Addr)
}

func (s *Server) Start() error {
	s.Logger.Info("Starting server version %s on %s", Version, s.Listener.Addr())

	// Start worker pool
	for i := 0; i < cap(s.WorkerPool); i++ {
		go s.worker()
	}

	go s.handleSignals()

	for {
		conn, err := s.Listener.Accept()
		if err != nil {
			select {
			case <-s.Shutdown:
				return ErrServerClosed
			default:
				s.Metrics.IncErrors()
				if ne, ok := err.(net.Error); ok && ne.Temporary() {
					s.Logger.Error("Temporary accept error: %v", err)
					time.Sleep(100 * time.Millisecond)
					continue
				}
				return fmt.Errorf("accept error: %w", err)
			}
		}

		if err := s.handleNewConnection(conn); err != nil {
			s.Metrics.IncErrors()
			s.Logger.Error("Connection handling error: %v", err)
		}
	}
}

func (s *Server) handleNewConnection(conn net.Conn) error {
	select {
	case s.ConnPool <- struct{}{}:
		s.WaitGroup.Add(1)
		s.Metrics.IncConnections()

		if tcpConn, ok := conn.(*net.TCPConn); ok {
			tcpConn.SetKeepAlive(true)
			tcpConn.SetKeepAlivePeriod(DefaultKeepAlive)
		}

		select {
		case s.WorkerPool <- conn:
			return nil
		case <-s.Shutdown:
			conn.Close()
			return ErrServerClosed
		}
	default:
		conn.SetWriteDeadline(time.Now().Add(s.Config.WriteTimeout))
		_, err := conn.Write([]byte("-ERR " + ErrTooManyConnects.Error() + "\r\n"))
		conn.Close()
		if err != nil {
			return fmt.Errorf("connection rejection failed: %w", err)
		}
		return ErrTooManyConnects
	}
}

func (s *Server) worker() {
	for conn := range s.WorkerPool {
		if err := s.handleConnection(conn); err != nil {
			s.Logger.Debug("Connection handling error: %v", err)
		}
	}
}

func (s *Server) handleConnection(conn net.Conn) error {
	defer func() {
		conn.Close()
		<-s.ConnPool
		s.WaitGroup.Done()
		s.Metrics.DecConnections()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-s.Shutdown
		cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			if s.Config.IdleTimeout > 0 {
				conn.SetDeadline(time.Now().Add(s.Config.IdleTimeout))
			}

			buf := make([]byte, MaxHeaderSize)
			n, err := conn.Read(buf)
			if err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					return nil
				}
				return fmt.Errorf("read error: %w", err)
			}

			s.Metrics.IncRequests()
			s.Logger.Debug("Received %d bytes from %s:", n, conn.RemoteAddr())

			conn.SetWriteDeadline(time.Now().Add(s.Config.WriteTimeout))
			if _, err := conn.Write([]byte("hello client")); err != nil {
				return fmt.Errorf("write error: %w", err)
			}
		}
	}
}

func (s *Server) Stop() error {
	s.Once.Do(func() {
		close(s.Shutdown)
		s.Listener.Close()

		// Drain worker pool
		go func() {
			close(s.WorkerPool)
			s.WaitGroup.Wait()
			close(s.DoneChan)
		}()
	})

	select {
	case <-s.DoneChan:
		return nil
	case <-time.After(ShutdownTimeout):
		return errors.New("shutdown timeout")
	}
}

func (s *Server) handleSignals() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	s.Logger.Info("Received shutdown signal")
	s.Stop()
}

type NoopMetrics struct{}

func (n *NoopMetrics) IncConnections() {}
func (n *NoopMetrics) DecConnections() {}
func (n *NoopMetrics) IncErrors()      {}
func (n *NoopMetrics) IncRequests()    {}
