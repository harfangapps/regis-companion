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
	listener net.Listener
	dispatch func(done <-chan struct{}, wg *sync.WaitGroup, conn net.Conn)
	errChan  chan<- error
	wg       sync.WaitGroup
}

func (s *retryServer) serve(ctx context.Context) error {
	defer s.listener.Close()

	done := ctx.Done()
	go func() {
		<-done
		s.listener.Close()
	}()

	var delay time.Duration
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			err = errors.Wrap(err, "Accept error")

			// if the server was stopped, return immediately
			select {
			case <-done:
				s.wg.Wait()
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
		go s.dispatch(done, &s.wg, conn)
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

		handleError(errors.Wrap(err, fmt.Sprintf("temporary error, retrying in %v", *delay)), s.errChan)
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
