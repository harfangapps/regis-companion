package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// retryServer encapsulates the common logic to all servers that listen
// for connections, retry on temporary errors after a delay, and dispatch
// a goroutine to handle connections.
type retryServer struct {
	Listener    net.Listener
	Dispatch    func(ctx context.Context, wg *sync.WaitGroup, conn net.Conn)
	ErrChan     chan<- error
	IdleTimeout time.Duration

	// Atomic integer incremented whenever there's activity on the server.
	// The retryServer itself increments it when there's an accepted
	// connection, and wraps the connection in a net.Conn that automatically
	// increments that counter when there's a Read or Write activity.
	activityCounter int64

	wg sync.WaitGroup
	// TODO: support an idletimeout, expose the atomic int for sub-goros
	// to use and update. Start a goro that checks at idletimeout intervals
	// if the int is still the same, and if so cancels the context.
}

func (s *retryServer) serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)

	defer func() {
		// stop accepting new connections
		s.Listener.Close()
		// cancel the context
		cancel()
		// wait for goroutines to exit
		s.wg.Wait()
	}()

	// listen for the stop signal and close the server on receive
	s.wg.Add(1)
	done := ctx.Done()
	go func() {
		<-done
		s.Listener.Close()
		s.wg.Done()
	}()

	var delay time.Duration
	for {
		conn, err := s.Listener.Accept()
		if err != nil {
			err = errors.Wrap(err, "Accept error")

			// if the server was stopped, return immediately
			select {
			case <-done:
				return err
			default:
				// go on
			}

			// if the error is temporary, retry after a delay
			if s.handleTemporary(&delay, err) {
				continue
			}
			return err
		}
		delay = 0
		s.wg.Add(1)
		go s.Dispatch(ctx, &s.wg, conn)
	}
}

// handle temporary errors by delaying a retry. Returns false if the error is
// not temporary.
func (s *retryServer) handleTemporary(delay *time.Duration, err error) bool {
	root := errors.Cause(err)

	if te, ok := root.(interface {
		Temporary() bool
	}); ok && te.Temporary() {
		if *delay == 0 {
			*delay = 5 * time.Millisecond
		} else {
			*delay *= 2
		}

		if max := 1 * time.Second; *delay > max {
			*delay = max
		}

		handleError(errors.Wrap(err, fmt.Sprintf("temporary error, retrying in %v", *delay)), s.ErrChan)
		time.Sleep(*delay)
		return true
	}

	return false
}

// handle the error by sending it to the errChan or logging it.
func handleError(err error, errChan chan<- error) {
	select {
	case errChan <- err:
	default:
		// log if errChan is nil, drop otherwise
		if errChan == nil {
			log.Print(err)
		}
	}
}
