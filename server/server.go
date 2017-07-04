package server

import (
	"context"
	"net"
	"sync"
	"time"

	"bitbucket.org/harfangapps/regis-companion/resp"

	"github.com/pkg/errors"
)

// Server defines the regis-companion Server that listens for incoming connections
// and manages SSH tunnels.
type Server struct {
	// The address the server listens on.
	Addr net.Addr

	// Duration before the server stops if there is no active tunnel
	// and no connection attempt.
	IdleTimeout time.Duration
	// Read timeout before returning a network error on a read attempt.
	ReadTimeout time.Duration
	// Write timeout before returning a network error on a write attempt.
	WriteTimeout time.Duration

	// The channel to send errors to. If nil, the errors are logged.
	// If the send would block, the error is dropped. It is the responsibility
	// of the caller to close the channel once the Server is stopped.
	// If set, this ErrChain is used for all Tunnels started by this
	// Server.
	ErrChan chan<- error
}

// ListenAndServe starts the server on the specified Addr.
//
// This call is blocking, it returns only when an error is
// encountered. As such, it always returns a non-nil error.
func (s *Server) ListenAndServe(ctx context.Context) error {
	l, err := net.Listen(s.Addr.Network(), s.Addr.String())
	if err != nil {
		return errors.Wrap(err, "listen error")
	}
	return s.serve(ctx, l)
}

func (s *Server) serve(ctx context.Context, l net.Listener) error {
	server := retryServer{
		listener: l,
		dispatch: t.serveConn,
		errChan:  s.ErrChan,
	}
	return server.serve(ctx)
}

func (s *Server) serveConn(done <-chan struct{}, serverWg *sync.WaitGroup, conn net.Conn) {
	defer func() {
		conn.Close()    // close the serviced connection
		serverWg.Done() // signal the server that this connection is done
	}()

	dec := resp.NewDecoder(conn)
	enc := resp.NewEncoder(conn)
}
