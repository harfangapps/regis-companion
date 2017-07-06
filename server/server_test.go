package server

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"bitbucket.org/harfangapps/regis-companion/internal/testutils"
	"github.com/pkg/errors"
)

// Stopping a started Server returns the error returned from Listener.Accept.
func TestStopUnblocksServerAccept(t *testing.T) {
	srv := &Server{Addr: tcpAddr}

	wantErr := errors.New("err")
	closeChan := make(chan struct{})
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			<-closeChan
			return nil, wantErr
		},
		CloseChan: closeChan,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := srv.serve(ctx, listener); errors.Cause(err) != wantErr {
		t.Errorf("want %v, got %v", wantErr, err)
	}

	// listener's Close should've been called twice (on context signal
	// and on exit from Tunnel.serve)
	if n := listener.CloseCalls(); n != 2 {
		t.Errorf("want listener.Close() to be called twice, got %v", n)
	}
	// listener's Accept should've been called once
	if n := listener.AcceptCalls(); n != 1 {
		t.Errorf("want listener.Close() to be called once, got %v", n)
	}
}

// Stopping the Server unblocks the active connections.
func TestStopServerUnblockConnection(t *testing.T) {
	// the close channel for the connection
	closeChan := make(chan struct{})
	readWriteErr := errors.New("read-write")
	conn := &testutils.MockConn{
		ReadFunc: func(i int, b []byte) (int, error) {
			<-closeChan // block until close
			return 0, readWriteErr
		},
		WriteFunc: func(i int, b []byte) (int, error) {
			<-closeChan // block until close
			return 0, readWriteErr
		},
		CloseChan: closeChan,
	}

	// the Server to test
	errChan := make(chan error, 1)
	srv := &Server{Addr: tcpAddr, ErrChan: errChan}

	listenerCloseChan := make(chan struct{})
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			// return one connection, then block until close
			if i == 0 {
				return conn, nil
			}
			<-listenerCloseChan
			return nil, io.EOF
		},
		CloseChan: listenerCloseChan,
	}

	// start the Server in a goroutine and stop it after a while
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		if err := srv.serve(ctx, listener); errors.Cause(err) != io.EOF {
			t.Errorf("want io.EOF, got %v", err)
		}
		close(errChan)
		wg.Done()
	}()
	// start the goroutine to process errors
	go func() {
		var n int
		for err := range errChan {
			if errors.Cause(err) != readWriteErr {
				t.Errorf("want %v, got %v", readWriteErr, err)
			}
			n++
		}
		if n != 1 {
			t.Errorf("want 1 error, got %v", n)
		}
		wg.Done()
	}()

	wg.Wait()

	// Connection should have been Closed once
	if n := conn.CloseCalls(); n != 1 {
		t.Errorf("want Conn.Close to be called once, got %v", n)
	}
}

func TestExecuteRequests(t *testing.T) {
	t.Skip()
}
