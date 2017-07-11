package server

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"

	"bitbucket.org/harfangapps/regis-companion/internal/testutils"
	"bitbucket.org/harfangapps/regis-companion/resp"
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

func TestStartAlreadyStarted(t *testing.T) {
	closeChan := make(chan struct{})
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			<-closeChan
			return nil, io.EOF
		},
		CloseChan: closeChan,
	}

	timeout := 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	srv := &Server{Addr: tcpAddr}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	start := time.Now()
	go func() {
		if err := srv.serve(ctx, listener); errors.Cause(err) != io.EOF {
			t.Errorf("want %v, got %v", io.EOF, err)
		}
		wg.Done()
	}()

	<-time.After(10 * time.Millisecond)
	if err := srv.serve(ctx, listener); err == nil {
		t.Errorf("want error, got nil")
	} else if !strings.Contains(err.Error(), "already started") {
		t.Errorf("want error to contain `already started`, got %v", err)
	}

	wg.Wait()

	dur := time.Since(start)
	want := timeout
	if dur < want || dur > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, dur)
	}

	// Close called twice: in goroutine that waits for context.Done,
	// and in defer of Serve.
	if n := listener.CloseCalls(); n != 2 {
		t.Errorf("want Listener.Close to be called twice, got %d", n)
	}
}

func TestExecutePingCommand(t *testing.T) {
	testExecuteCommand(t, []string{"PING"}, resp.Pong{})
}

func TestExecuteCommandCommand(t *testing.T) {
	testExecuteCommand(t, []string{"COMMAND"}, commandNames)
}

func TestExecuteUnknownCommand(t *testing.T) {
	testExecuteCommand(t, []string{"unknown"}, resp.Error("ERR unknown command unknown"))
}

func testExecuteCommand(t *testing.T, request []string, response interface{}) {
	// encode the request and the response
	var req bytes.Buffer
	if err := resp.NewEncoder(&req).Encode(request); err != nil {
		t.Fatal(err)
	}
	var res bytes.Buffer
	if err := resp.NewEncoder(&res).Encode(response); err != nil {
		t.Fatal(err)
	}

	// create the connection
	buf := testutils.SyncBuffer{}
	closeConn := make(chan struct{})
	conn := &testutils.MockConn{
		ReadFunc: func(i int, b []byte) (int, error) {
			if i == 0 {
				return req.Read(b)
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

	wantRes := res.String()
	if s := buf.String(); s != wantRes {
		t.Errorf("want response %q, got %v", wantRes, s)
	} else {
		t.Logf("got response %q", s)
	}
}

func TestEncodeErrorTerminatesConnection(t *testing.T) {
	// create the connection
	closeConn := make(chan struct{})
	theErr := errors.New("encode")
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
				return 0, theErr
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
	errChan := make(chan error, 1)
	srv := &Server{Addr: tcpAddr, ErrChan: errChan}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		var n int
		for err := range errChan {
			n++
			if errors.Cause(err) != theErr {
				t.Errorf("want %v, got %v", theErr, err)
			}
		}
		if n != 1 {
			t.Errorf("want 1 error, got %d", n)
		}
		wg.Done()
	}()

	start := time.Now()
	if err := srv.serve(ctx, listener); errors.Cause(err) != io.EOF {
		t.Errorf("want %v, got %v", io.EOF, err)
	}
	close(errChan)

	dur := time.Since(start)
	want := timeout
	if dur < want || dur > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, dur)
	}

	wg.Wait()

	// conn should've been closed just once, in the defer of serveConn
	if n := conn.CloseCalls(); n != 1 {
		t.Errorf("want conn.Close to be called once, got %d", n)
	}
	// conn should've been closed almost immediately
	dur = conn.ClosedAt().Sub(start)
	want = time.Duration(0)
	if dur < want || dur > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, dur)
	}
}
