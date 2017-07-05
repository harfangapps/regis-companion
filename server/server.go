package server

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"bitbucket.org/harfangapps/regis-companion/resp"

	"github.com/pkg/errors"
)

var (
	errEmptyCmd = errors.New("command is empty")
)

// Server defines the regis-companion Server that listens for incoming connections
// and manages SSH tunnels.
type Server struct {
	// The address the server listens on.
	Addr net.Addr

	// Duration before the tunnels stop if there is no active connection.
	TunnelIdleTimeout time.Duration
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
		Listener: l,
		Dispatch: s.serveConn,
		ErrChan:  s.ErrChan,
	}
	return server.serve(ctx)
}

func (s *Server) serveConn(ctx context.Context, serverWg *sync.WaitGroup, conn net.Conn) {
	wg := &sync.WaitGroup{}
	ctx, cancel := context.WithCancel(ctx)
	done := ctx.Done()

	defer func() {
		conn.Close()    // close the serviced connection
		cancel()        // required to release resources
		wg.Wait()       // wait for sub-goroutines to exit
		serverWg.Done() // signal the server that this connection is done
	}()

	wg.Add(1)
	go s.readWriteLoop(cancel, wg, conn)

	// block waiting for the stop signal
	<-done
}

func (s *Server) readWriteLoop(cancel func(), wg *sync.WaitGroup, conn net.Conn) {
	defer func() {
		cancel()
		wg.Done()
	}()

	dec := resp.NewDecoder(conn)
	enc := resp.NewEncoder(conn)
	for {
		// read the request
		req, err := dec.DecodeRequest()
		if err != nil {
			err = errors.Wrap(err, "decode request error")
			handleError(err, s.ErrChan)
			return
		}

		// handle the request
		res, err := s.execute(req)
		if err != nil {
			err = errors.Wrap(err, "execute request error")
			handleError(err, s.ErrChan)
			return
		}

		// write the response
		if s.WriteTimeout > 0 {
			if err := conn.SetWriteDeadline(time.Now().Add(s.WriteTimeout)); err != nil {
				err = errors.Wrap(err, "set write deadline")
				handleError(err, s.ErrChan)
				return
			}
		}
		if err := enc.Encode(res); err != nil {
			err = errors.Wrap(err, "encode response error")
			handleError(err, s.ErrChan)
			return
		}
	}
}

func (s *Server) execute(req []string) (interface{}, error) {
	if len(req) == 0 {
		return nil, errEmptyCmd
	}

	// TODO: assert command arity
	switch cmd := strings.ToLower(req[0]); cmd {
	case "command":
		// list the supported commands (redis-cli always executes this on connect)
		return []string{"command", "ping"}, nil
	case "ping":
		// only support the ping-pong version
		return resp.Pong{}, nil
	default:
		return resp.Error(fmt.Sprintf("ERR unknown command %v", cmd)), nil
	}
}
