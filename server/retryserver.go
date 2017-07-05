package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
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
	previousCounter int64

	// WaitGroup for all accepted connections, so that when the server returns,
	// all goroutines are properly terminated.
	wg sync.WaitGroup
}

func (s *retryServer) serve(ctx context.Context) error {
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

	// terminate on idle if requested
	if s.IdleTimeout > 0 {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()

			for {
				select {
				case <-time.After(s.IdleTimeout):
					current := atomic.LoadInt64(&s.activityCounter)
					previous := atomic.LoadInt64(&s.previousCounter)
					if current == previous {
						cancel()
						return
					}
					atomic.CompareAndSwapInt64(&s.previousCounter, previous, current)
				case <-done:
					return
				}
			}
		}()
	}

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

		delay = 0                              // reset the retry delay
		atomic.AddInt64(&s.activityCounter, 1) // indicate that there was activity
		s.wg.Add(1)                            // keep track of that goroutine

		// if there's an idle timeout, wrap the conn to track activity
		conn = activityConn{conn, &s.activityCounter}
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
