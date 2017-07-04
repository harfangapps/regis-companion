package server

import (
	"net"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// TODO: redo this (Server) and Tunnel with a context.Context passed
// to ListenAndServe. Canceling the context (e.g. catching SIGINT)
// stops the Tunnels and the Server. Server waits on WaitGroup for
// all Tunnels to terminate properly. No Close on Tunnel nor Server
// (just cancel the context to close them).

type Server struct {
	Addr         net.Addr
	IdleTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	ErrChan      chan<- error

	// mu protects the following private fields
	mu       sync.Mutex
	listener net.Listener
	tunnels  []*Tunnel
	done     chan struct{}
	closeErr error
}

func (s *Server) ListenAndServe() error {
	l, err := net.Listen(s.Addr.Network(), s.Addr.String())
	if err != nil {
		return errors.Wrap(err, "listen error")
	}
	return s.serve(l)
}

func (s *Server) serve(l net.Listener) error {
	defer l.Close()

	s.mu.Lock()
	s.done = make(chan struct{})
	s.listener = l
	s.mu.Unlock()
}
