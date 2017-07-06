package server

import (
	"context"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"bitbucket.org/harfangapps/regis-companion/internal/testutils"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

// arbitrary valid address
var tcpAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8000}

// Stopping an unstarted Tunnel returns an error almost immediately.
func TestStopUnstarted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tun := &Tunnel{Local: tcpAddr}
	if err := tun.ListenAndServe(ctx); err == nil {
		t.Errorf("want error, got nil")
	} else {
		t.Logf("got error %v", err)
	}

	// stopping closes the tunnel
	if ok := tun.Closed(); !ok {
		t.Errorf("want true, got %v", ok)
	}
	// and Touch returns false on a closed tunnel
	if ok := tun.Touch(); ok {
		t.Errorf("want false, got %v", ok)
	}
}

// Stopping a started Tunnel returns the error returned from Listener.Accept.
func TestStopUnblocksAccept(t *testing.T) {
	tun := &Tunnel{Local: tcpAddr, Server: tcpAddr, Remote: tcpAddr, Config: &ssh.ClientConfig{}}

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
	if err := tun.Serve(ctx, listener); errors.Cause(err) != wantErr {
		t.Errorf("want %v, got %v", wantErr, err)
	}

	// listener's Close should've been called twice (on context signal
	// and on exit from Tunnel.Serve)
	if n := listener.CloseCalls(); n != 2 {
		t.Errorf("want listener.Close() to be called twice, got %v", n)
	}
	// listener's Accept should've been called once
	if n := listener.AcceptCalls(); n != 1 {
		t.Errorf("want listener.Close() to be called once, got %v", n)
	}
}

// The Listener should retry temporary errors with an increasing delay.
func TestAcceptRetryTemporary(t *testing.T) {
	tun := &Tunnel{Local: tcpAddr, Server: tcpAddr, Remote: tcpAddr, Config: &ssh.ClientConfig{}}

	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			if i < 10 {
				return nil, testutils.ErrTemporaryTrue
			}
			return nil, io.EOF
		},
	}

	start := time.Now()
	if err := tun.Serve(context.Background(), listener); errors.Cause(err) != io.EOF {
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
	if err := tun.Serve(context.Background(), listener); errors.Cause(err) != io.EOF {
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
	tun := &Tunnel{Local: tcpAddr, Server: tcpAddr, Remote: tcpAddr, Config: &ssh.ClientConfig{}}

	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			return nil, testutils.ErrTemporaryFalse
		},
	}

	start := time.Now()
	if err := tun.Serve(context.Background(), listener); errors.Cause(err) != testutils.ErrTemporaryFalse {
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

// Stopping the Tunnel unblocks the active local and remote connections.
func TestStopUnblockConnection(t *testing.T) {
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
	}

	// start the tunnel in a goroutine and stop it after a while
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		if err := tun.Serve(ctx, listener); errors.Cause(err) != io.EOF {
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
		if n != 2 {
			t.Errorf("want 2 errors, got %v", n)
		}
		wg.Done()
	}()

	wg.Wait()

	// assert the calls to the SSH client
	if n := sshClient.CloseCalls(); n != 1 {
		t.Errorf("want sshClient.Close to be called once, got %v", n)
	}
	if n := sshClient.DialCalls(); n != 1 {
		t.Errorf("want sshClient.Dial to be called once, got %v", n)
	}
}

// Accept error (non temporary) stops all active connections and returns.
func TestAcceptErrorUnblockConnection(t *testing.T) {
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

	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			// return one connection, then block a while
			if i == 0 {
				return newBlockingConn(), nil
			}
			<-time.After(10 * time.Millisecond)
			return nil, io.EOF
		},
	}

	// start the tunnel in a goroutine and stop it after a while, should return earlier than that
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	wg := sync.WaitGroup{}
	wg.Add(2)
	start := time.Now()
	go func() {
		if err := tun.Serve(ctx, listener); errors.Cause(err) != io.EOF {
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
		if n != 2 {
			t.Errorf("want 2 errors, got %v", n)
		}
		wg.Done()
	}()

	wg.Wait()

	// assert the duration
	want := 10 * time.Millisecond
	got := time.Since(start)
	if got < want || got > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, got)
	}

	// assert the calls to the SSH client
	if n := sshClient.CloseCalls(); n != 1 {
		t.Errorf("want sshClient.Close to be called once, got %v", n)
	}
	if n := sshClient.DialCalls(); n != 1 {
		t.Errorf("want sshClient.Dial to be called once, got %v", n)
	}
	// assert the calls to Accept
	if n := listener.AcceptCalls(); n != 2 {
		t.Errorf("want Listener.Accept to be called twice, got %v", n)
	}
}

// An error returned by the SSH Dial call closes the accepted connection.
func TestSSHDialError(t *testing.T) {
	sshErr := errors.New("ssh")
	sshDialFn = func(n, addr string, config *ssh.ClientConfig) (dialCloser, error) {
		return nil, sshErr
	}
	defer func() { sshDialFn = defaultSSHDial }()

	// the tunnel to test
	errChan := make(chan error, 1)
	tun := &Tunnel{Local: tcpAddr, Server: tcpAddr, Remote: tcpAddr, Config: &ssh.ClientConfig{}, ErrChan: errChan}

	theConn := &testutils.MockConn{}
	closeChan := make(chan struct{})
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			// return one connection, then block until close
			if i == 0 {
				return theConn, nil
			}
			<-closeChan
			return nil, io.EOF
		},
		CloseChan: closeChan,
	}

	// start the tunnel in a goroutine and close it after a while
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		if err := tun.Serve(ctx, listener); errors.Cause(err) != io.EOF {
			t.Errorf("want io.EOF, got %v", err)
		}
		close(errChan)
		wg.Done()
	}()
	// start the goroutine to process errors
	go func() {
		var n int
		for err := range errChan {
			if errors.Cause(err) != sshErr {
				t.Errorf("want %v, got %v", sshErr, err)
			}
			n++
		}
		if n != 1 {
			t.Errorf("want 1 error, got %v", n)
		}
		wg.Done()
	}()

	wg.Wait()

	if n := theConn.CloseCalls(); n != 1 {
		t.Errorf("want localConn.Close to be called once, got %v", n)
	}
}

// An error returned by the server SSH client Dial call closes the local
// connection and the SSH server connection.
func TestServerDialError(t *testing.T) {
	sshErr := errors.New("ssh")
	sshClient := &testutils.MockSSHClient{
		DialFunc: func(i int, n, addr string) (net.Conn, error) {
			return nil, sshErr
		},
	}
	sshDialFn = func(n, addr string, config *ssh.ClientConfig) (dialCloser, error) {
		return sshClient, nil
	}
	defer func() { sshDialFn = defaultSSHDial }()

	// the tunnel to test
	errChan := make(chan error, 1)
	tun := &Tunnel{Local: tcpAddr, Server: tcpAddr, Remote: tcpAddr, Config: &ssh.ClientConfig{}, ErrChan: errChan}

	theConn := &testutils.MockConn{}
	closeChan := make(chan struct{})
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			// return one connection, then block until close
			if i == 0 {
				return theConn, nil
			}
			<-closeChan
			return nil, io.EOF
		},
		CloseChan: closeChan,
	}

	// start the tunnel in a goroutine and close it after a while
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		if err := tun.Serve(ctx, listener); errors.Cause(err) != io.EOF {
			t.Errorf("want io.EOF, got %v", err)
		}
		close(errChan)
		wg.Done()
	}()
	// start the goroutine to process errors
	go func() {
		var n int
		for err := range errChan {
			if errors.Cause(err) != sshErr {
				t.Errorf("want %v, got %v", sshErr, err)
			}
			n++
		}
		if n != 1 {
			t.Errorf("want 1 error, got %v", n)
		}
		wg.Done()
	}()

	wg.Wait()

	if n := theConn.CloseCalls(); n != 1 {
		t.Errorf("want localConn.Close to be called once, got %v", n)
	}
	if n := sshClient.CloseCalls(); n != 1 {
		t.Errorf("want serverConn.Close to be called once, got %v", n)
	}
	if n := sshClient.DialCalls(); n != 1 {
		t.Errorf("want serverConn.Dial to be called once, got %v", n)
	}
}

// The Tunnel forwards bytes from the local connection to the remote
// connection and vice-versa.
func TestRecordForwarding(t *testing.T) {
	// the buffer that records the exchange
	var buf testutils.SyncBuffer

	message := "hello"
	newRecordingConn := func() net.Conn {
		return &testutils.MockConn{
			ReadFunc: func(i int, b []byte) (int, error) {
				n, _ := strings.NewReader(message).Read(b)
				return n, io.EOF
			},
			WriteFunc: func(i int, b []byte) (int, error) {
				return buf.Write(b)
			},
		}
	}

	// return a mocked SSH client when dialing via SSH
	sshClient := &testutils.MockSSHClient{
		DialFunc: func(i int, n, addr string) (net.Conn, error) {
			return newRecordingConn(), nil
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
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			// return one connection, then block until close
			if i == 0 {
				return newRecordingConn(), nil
			}
			<-listenerCloseChan
			return nil, io.EOF
		},
		CloseChan: listenerCloseChan,
	}

	// start the tunnel in a goroutine and close it after a while
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		if err := tun.Serve(ctx, listener); errors.Cause(err) != io.EOF {
			t.Errorf("want io.EOF, got %v", err)
		}
		close(errChan)
		wg.Done()
	}()
	// start the goroutine to process errors
	go func() {
		for err := range errChan {
			t.Errorf("want no error, got %v", err)
		}
		wg.Done()
	}()

	wg.Wait()

	// check that the buffer contains "hellohello" (bytes copied in both directions)
	want := message + message
	if s := buf.String(); s != want {
		t.Errorf("want %q, got: %q", want, s)
	}
}
