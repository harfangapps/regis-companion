package server

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"

	"bitbucket.org/harfangapps/regis-companion/internal/testutils"

	"golang.org/x/crypto/ssh"
)

// arbitrary valid address
var tcpAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8000}

func TestCloseUnstarted(t *testing.T) {
	tun := &Tunnel{}
	if e1 := tun.Close(); e1 != nil {
		t.Errorf("want nil, got %v", e1)
	}
	if e2 := tun.Close(); e2 != nil {
		t.Errorf("want nil, got %v", e2)
	}
}

func TestCloseAlreadyClosed(t *testing.T) {
	tun := &Tunnel{Local: tcpAddr, Server: tcpAddr, Remote: tcpAddr, Config: &ssh.ClientConfig{}}

	var closeErr = errors.New("err")
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			return nil, io.EOF
		},
		CloseErr: closeErr,
	}

	if err := tun.serve(listener); errors.Cause(err) != io.EOF {
		t.Errorf("want io.EOF, got %v", err)
	}

	// close once
	if err := tun.Close(); err != closeErr {
		t.Errorf("want %v, got %v", closeErr, err)
	}

	// close again, should return the same error
	if err := tun.Close(); err != closeErr {
		t.Errorf("want %v, got %v", closeErr, err)
	}

	// listener's Close should've been called twice (on exit of Tunnel.serve
	// and on the first call to Tunnel.Close)
	if n := listener.CloseCalls(); n != 2 {
		t.Errorf("want listener.Close() to be called twice, got %v", n)
	}
}

func TestAcceptRetryTemporary(t *testing.T) {
	tun := &Tunnel{Local: tcpAddr, Server: tcpAddr, Remote: tcpAddr, Config: &ssh.ClientConfig{}}

	var closeErr = errors.New("err")
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			if i < 10 {
				return nil, testutils.ErrTemporaryTrue
			}
			return nil, io.EOF
		},
		CloseErr: closeErr,
	}

	start := time.Now()
	if err := tun.serve(listener); errors.Cause(err) != io.EOF {
		t.Errorf("want io.EOF, got %v", err)
	}

	// retried 10 times for temporary errors, so the delays should be:
	want := (5 + 10 + 20 + 40 + 80 + 160 + 320 + 640 + 1000 + 1000) * time.Millisecond
	got := time.Since(start)
	if got < want || got > (want+(100*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, got)
	}

	// closing returns the error returned by the Listener, and this
	// MockListener returns nil.
	if err := tun.Close(); err != closeErr {
		t.Errorf("want %v, got %v", closeErr, err)
	}
}

func TestAcceptRetryTemporaryReset(t *testing.T) {
	// this test doesn't need to handle accepted connections
	sshDialFn = func(n, addr string, config *ssh.ClientConfig) (dialCloser, error) {
		return nil, io.EOF
	}
	defer func() { sshDialFn = defaultSSHDial }()

	// the tunnel to test
	tun := &Tunnel{Local: tcpAddr, Server: tcpAddr, Remote: tcpAddr, Config: &ssh.ClientConfig{}}

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

	start := time.Now()
	if err := tun.serve(listener); errors.Cause(err) != io.EOF {
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

func TestNoRetryTemporaryFalse(t *testing.T) {
	tun := &Tunnel{Local: tcpAddr, Server: tcpAddr, Remote: tcpAddr, Config: &ssh.ClientConfig{}}

	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			return nil, testutils.ErrTemporaryFalse
		},
	}

	start := time.Now()
	if err := tun.serve(listener); errors.Cause(err) != testutils.ErrTemporaryFalse {
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

func TestCloseUnblockAccept(t *testing.T) {
	tun := &Tunnel{Local: tcpAddr, Server: tcpAddr, Remote: tcpAddr, Config: &ssh.ClientConfig{}}

	var closeErr = errors.New("err")
	var closeChan = make(chan struct{})
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			<-closeChan // wait until closed
			return nil, io.EOF
		},
		CloseErr:  closeErr,
		CloseChan: closeChan,
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		if err := tun.serve(listener); errors.Cause(err) != io.EOF {
			t.Errorf("want io.EOF, got %v", err)
		}
		wg.Done()
	}()

	// wait for serve to start
	<-time.After(100 * time.Millisecond)
	// close the Tunnel
	if err := tun.Close(); err != closeErr {
		t.Errorf("want %v, got %v", closeErr, err)
	}
	wg.Wait()

	// listener's Close should've been called twice (on exit of Tunnel.serve
	// and on the first call to Tunnel.Close)
	if n := listener.CloseCalls(); n != 2 {
		t.Errorf("want listener.Close() to be called twice, got %v", n)
	}

	// listener's Accept should've been called just once.
	if n := listener.AcceptCalls(); n != 1 {
		t.Errorf("want listener.Accept() to be called once, got %v", n)
	}
}

func TestCloseUnblockConnection(t *testing.T) {
	// the close channel for the connections, shared because only the
	// local connection is closed, so to unblock the remote connection
	// it must use the same channel.
	closeChan := make(chan struct{})
	readWriteErr := errors.New("read-write")
	newBlockingConn := func() net.Conn {
		return &testutils.MockConn{
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
	}

	// return a mocked SSH client when dialing via SSH
	sshClient := &testutils.MockSSHClient{
		DialFunc: func(i int, n, addr string) (net.Conn, error) {
			return newBlockingConn(), nil
		},
	}
	sshDialFn = func(n, addr string, config *ssh.ClientConfig) (dialCloser, error) {
		return sshClient, nil
	}
	defer func() { sshDialFn = defaultSSHDial }()

	// the tunnel to test
	errChan := make(chan error, 1)
	tun := &Tunnel{Local: tcpAddr, Server: tcpAddr, Remote: tcpAddr, Config: &ssh.ClientConfig{}, ErrChan: errChan}

	listenerCloseChan := make(chan struct{})
	closeErr := errors.New("close")
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			// return one connection, then block until close
			if i == 0 {
				return newBlockingConn(), nil
			}
			<-listenerCloseChan
			return nil, io.EOF
		},
		CloseChan: listenerCloseChan,
		CloseErr:  closeErr,
	}

	// start the tunnel in a goroutine and close it after a while
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		if err := tun.serve(listener); errors.Cause(err) != io.EOF {
			t.Errorf("want io.EOF, got %v", err)
		}
		wg.Done()
	}()
	// start the goroutine to process errors
	go func() {
		var n int
		for err := range errChan {
			if errors.Cause(err) != readWriteErr {
				t.Errorf("want %v, got %v", readWriteErr, err)
			}
			n += 1
		}
		if n != 2 {
			t.Errorf("want 2 errors, got %v", n)
		}
		wg.Done()
	}()

	// wait for serve to start
	<-time.After(100 * time.Millisecond)
	// close the Tunnel
	if err := tun.Close(); err != closeErr {
		t.Errorf("want %v, got %v", closeErr, err)
	}
	wg.Wait()

	// assert the calls to the SSH client
	if n := sshClient.CloseCalls(); n != 1 {
		t.Errorf("want sshClient.Close to be callsed once, got %v", n)
	}
	if n := sshClient.DialCalls(); n != 1 {
		t.Errorf("want sshClient.Dial to be callsed once, got %v", n)
	}
}
