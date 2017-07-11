package server

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"

	"bitbucket.org/harfangapps/regis-companion/internal/testutils"
)

var tcpAddr = &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8000}

func TestStartCancelledAndRestart(t *testing.T) {
	closeChan := make(chan struct{})
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			<-closeChan
			return nil, io.EOF
		},
		CloseChan: closeChan,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	srv := &Server{Addr: tcpAddr}
	start := time.Now()
	if err := srv.serve(ctx, listener); errors.Cause(err) != io.EOF {
		t.Errorf("want %v, got %v", io.EOF, err)
	}

	dur := time.Since(start)
	want := time.Duration(0)
	if dur < want || dur > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, dur)
	}

	// Close called twice: in goroutine that waits for context.Done,
	// and in defer of Serve.
	if n := listener.CloseCalls(); n != 2 {
		t.Errorf("want Listener.Close to be called twice, got %d", n)
	}

	// start again
	if err := srv.serve(ctx, listener); errors.Cause(err) == nil {
		t.Errorf("want error, got nil")
	} else if !strings.Contains(err.Error(), "server closed") {
		t.Errorf("want error to contain `server closed`, got %v", err)
	}
}

func TestExecutePingCommand(t *testing.T) {
	// create the connection that sends PING and receives PONG
	buf := testutils.SyncBuffer{}
	closeConn := make(chan struct{})
	conn := &testutils.MockConn{
		ReadFunc: func(i int, b []byte) (int, error) {
			if i == 0 {
				r := strings.NewReader("*1\r\n$4\r\nPING\r\n")
				return r.Read(b)
			}
			<-closeConn
			return 0, io.EOF
		},
		WriteFunc: func(i int, b []byte) (int, error) {
			if i == 0 {
				return buf.Write(b)
			}
			<-closeConn
			return 0, io.EOF
		},
		CloseChan: closeConn,
	}

	// create the listener that returns the conn on first Accept,
	// then waits for close.
	closeChan := make(chan struct{})
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			if i == 0 {
				return conn, nil
			}
			<-closeChan
			return nil, io.EOF
		},
		CloseChan: closeChan,
	}

	timeout := 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// create and run the server
	start := time.Now()
	srv := &Server{Addr: tcpAddr}
	if err := srv.serve(ctx, listener); errors.Cause(err) != io.EOF {
		t.Errorf("want %v, got %v", io.EOF, err)
	}

	dur := time.Since(start)
	want := timeout
	if dur < want || dur > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, dur)
	}

	pong := "+PONG\r\n"
	if s := buf.String(); s != pong {
		t.Errorf("want response %q, got %v", pong, s)
	} else {
		t.Logf("got response %q", s)
	}
}
