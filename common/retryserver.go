package common

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// Doner is the interface for a sync.WaitGroup that can only call
// Done (signal the end).
type Doner interface {
	Done()
}

var _ Doner = (*sync.WaitGroup)(nil)

// RetryServer encapsulates the common logic to all servers that listen
// for connections, retry on temporary errors after a delay, and dispatch
// a goroutine to handle connections.
type RetryServer struct {
	// The Listener to use to listen for incoming connections.
	Listener net.Listener

	// The function to call in a goroutine to serve accepted connections.
	// On exit, the function should always close conn and call d.Done.
	Dispatch func(ctx context.Context, d Doner, conn net.Conn)

	// If non-nil, errors are reported on that channel. If the send would
	// block, the error is dropped. If it is nil, errors are logged.
	// It is the caller's responsibility to close the channel once the
	// server has exited.
	ErrChan chan<- error

	// If IdleTracker.IdleTimeout is greater than 0, terminates the
	// Server if there is no activity in that duration.
	IdleTracker IdleTracker

	// WaitGroup for all accepted connections, so that when the server returns,
	// all goroutines are properly terminated.
	wg sync.WaitGroup
}

// Serve starts accepting connections using RetryServer.Listener. It is a
// blocking call that always returns an error.
func (s *RetryServer) Serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	done := ctx.Done()

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
	go func() {
		<-done
		s.Listener.Close()
		s.wg.Done()
	}()

	// cancel the server if idle for IdleDuration
	s.wg.Add(1)
	s.IdleTracker.Start(ctx, cancel, &s.wg)

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

		// reset the retry delay
		delay = 0

		// signal activity
		s.IdleTracker.Touch()
		// keep track of that goroutine
		s.wg.Add(1)
		go s.Dispatch(ctx, &s.wg, s.IdleTracker.TrackConn(conn))
	}
}

// handle temporary errors by delaying a retry. Returns false if the error is
// not temporary.
func (s *RetryServer) handleTemporary(delay *time.Duration, err error) bool {
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

		HandleError(errors.Wrap(err, fmt.Sprintf("temporary error, retrying in %v", *delay)), s.ErrChan)
		time.Sleep(*delay)
		return true
	}

	return false
}

// HandleError handles the error by sending it to the errChan or
// logging it to standard error if errChan is nil.
func HandleError(err error, errChan chan<- error) {
	select {
	case errChan <- err:
	default:
		// log if errChan is nil, drop otherwise
		if errChan == nil {
			log.Print(err)
		}
	}
}
