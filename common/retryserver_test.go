package common

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"bitbucket.org/harfangapps/regis-companion/internal/testutils"
	"github.com/pkg/errors"
)

func nopDispatch(ctx context.Context, d Doner, conn net.Conn) {
	conn.Close()
	d.Done()
}

// The Listener should retry temporary errors with an increasing delay.
func TestAcceptRetryTemporary(t *testing.T) {
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			if i < 10 {
				return nil, testutils.ErrTemporaryTrue
			}
			return nil, io.EOF
		},
	}
	server := &RetryServer{
		Listener: listener,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	if err := server.Serve(ctx); errors.Cause(err) != io.EOF {
		t.Errorf("want io.EOF, got %v", err)
	}

	// retried 10 times for temporary errors, so the delays should be:
	want := (5 + 10 + 20 + 40 + 80 + 160 + 320 + 640 + 1000 + 1000) * time.Millisecond
	got := time.Since(start)
	if got < want || got > (want+(100*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, got)
	}
}

// The Listener should reset the temporary error retry delay after a successful Accept.
func TestAcceptRetryTemporaryReset(t *testing.T) {
	// the accepted connection
	conn := &testutils.MockConn{}

	// the listener that fails with temporary errors, accepts one,
	// and fails again with temporary errors to check the reset
	// of the temporary errors delay
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			if i < 5 {
				return nil, testutils.ErrTemporaryTrue
			}
			if i == 5 {
				return conn, nil
			}
			if i < 10 {
				return nil, testutils.ErrTemporaryTrue
			}
			return nil, io.EOF
		},
	}
	server := &RetryServer{
		Listener: listener,
		Dispatch: nopDispatch,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	if err := server.Serve(ctx); errors.Cause(err) != io.EOF {
		t.Errorf("want io.EOF, got %v", err)
	}

	// retried 5 times for temporary errors:
	firstDelay := (5 + 10 + 20 + 40 + 80) * time.Millisecond
	// then 4 more times after a delay reset:
	secondDelay := (5 + 10 + 20 + 40) * time.Millisecond
	want := firstDelay + secondDelay

	got := time.Since(start)
	if got < want || got > (want+(100*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, got)
	}
}

// The Listener should not retry when an error that implements Temporary returns false.
func TestNoRetryTemporaryFalse(t *testing.T) {
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			return nil, testutils.ErrTemporaryFalse
		},
	}
	server := &RetryServer{
		Listener: listener,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	if err := server.Serve(ctx); errors.Cause(err) != testutils.ErrTemporaryFalse {
		t.Errorf("want %v, got %v", testutils.ErrTemporaryFalse, err)
	}

	// error was a Temporary, but it returned false, so there should
	// be no delay
	got := time.Since(start)
	want := time.Duration(0)
	if got < want || got > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, got)
	}
}

func TestCancelContextStopsServer(t *testing.T) {
	closeListener := make(chan struct{})
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			<-closeListener
			return nil, io.EOF
		},
		CloseChan: closeListener,
	}
	server := &RetryServer{
		Listener: listener,
	}

	timeout := 10 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	if err := server.Serve(ctx); errors.Cause(err) != io.EOF {
		t.Errorf("want %v, got %v", io.EOF, err)
	}

	duration := time.Since(start)
	want := timeout
	if duration < want || duration > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, duration)
	}
}

func TestIdleTimeoutStopsServer(t *testing.T) {
	closeListener := make(chan struct{})
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			<-closeListener
			return nil, io.EOF
		},
		CloseChan: closeListener,
	}
	server := &RetryServer{
		Listener: listener,
	}
	idle := 50 * time.Millisecond
	server.IdleTracker.IdleTimeout = idle

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	if err := server.Serve(ctx); errors.Cause(err) != io.EOF {
		t.Errorf("want %v, got %v", io.EOF, err)
	}

	duration := time.Since(start)
	want := idle
	if duration < want || duration > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, duration)
	}
}
