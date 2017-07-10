package tunnel

import (
	"context"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"

	"bitbucket.org/harfangapps/regis-companion/internal/testutils"
	"golang.org/x/crypto/ssh"
)

// arbitrary valid address
var tcpAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8000}

func errSSHDial(n, a string, conf *ssh.ClientConfig) (dialCloser, error) {
	return nil, io.EOF
}

func mockSSHDial(dc dialCloser) func(n, a string, conf *ssh.ClientConfig) (dialCloser, error) {
	return func(n, a string, conf *ssh.ClientConfig) (dialCloser, error) {
		return dc, nil
	}
}

func setAndDeferSSHDial(fn func(n, a string, conf *ssh.ClientConfig) (dialCloser, error)) func() {
	sshDialFunc = fn
	return func() {
		sshDialFunc = defaultSSHDial
	}
}

// Starting with a cancelled context returns almost immediately.
func TestStartWithCancelledContext(t *testing.T) {
	sshClient := &testutils.MockSSHClient{}
	defer setAndDeferSSHDial(mockSSHDial(sshClient))()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	closeListener := make(chan struct{})
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			<-closeListener
			return nil, io.EOF
		},
		CloseChan: closeListener,
	}

	tun := &Tunnel{Local: tcpAddr, SSH: tcpAddr}
	start := time.Now()
	if err := tun.Serve(ctx, listener); err == nil {
		t.Errorf("want error, got nil")
	} else {
		t.Logf("got error %v", err)
	}

	duration := time.Since(start)
	want := time.Duration(0)
	if duration < want || duration > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, duration)
	}

	// Touch returns false on a closed tunnel
	if ok := tun.Touch(); ok {
		t.Errorf("want false, got %v", ok)
	}
}

// Stopping a started Tunnel returns the error returned from Listener.Accept.
func TestStopUnblocksAccept(t *testing.T) {
	sshClient := &testutils.MockSSHClient{}
	defer setAndDeferSSHDial(mockSSHDial(sshClient))()

	timeout := 10 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	wantErr := errors.New("err")
	closeChan := make(chan struct{})
	listener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			<-closeChan
			return nil, wantErr
		},
		CloseChan: closeChan,
	}

	tun := &Tunnel{Local: tcpAddr, SSH: tcpAddr, Remote: tcpAddr, Config: &ssh.ClientConfig{}}
	start := time.Now()
	if err := tun.Serve(ctx, listener); errors.Cause(err) != wantErr {
		t.Errorf("want %v, got %v", wantErr, err)
	}

	duration := time.Since(start)
	want := timeout
	if duration < want || duration > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, duration)
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

// Cancelling the context unblocks the active local and remote connections.
func TestCancelUnblockConnection(t *testing.T) {
	readWriteErr := errors.New("read-write")
	newBlockingConn := func() net.Conn {
		close := make(chan struct{})
		return &testutils.MockConn{
			ReadFunc: func(i int, b []byte) (int, error) {
				<-close // block until close
				return 0, readWriteErr
			},
			WriteFunc: func(i int, b []byte) (int, error) {
				<-close // block until close
				return 0, readWriteErr
			},
			CloseChan: close,
		}
	}

	// return a mocked SSH client when dialing via SSH
	sshClient := &testutils.MockSSHClient{
		DialFunc: func(i int, n, addr string) (net.Conn, error) {
			return newBlockingConn(), nil
		},
	}
	defer setAndDeferSSHDial(mockSSHDial(sshClient))()

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

	// the tunnel to test
	errChan := make(chan error, 1)
	tun := &Tunnel{Local: tcpAddr, SSH: tcpAddr, Remote: tcpAddr, Config: &ssh.ClientConfig{}, ErrChan: errChan}

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
	readWriteErr := errors.New("read-write")
	newBlockingConn := func() net.Conn {
		close := make(chan struct{})
		return &testutils.MockConn{
			ReadFunc: func(i int, b []byte) (int, error) {
				<-close // block until close
				return 0, readWriteErr
			},
			WriteFunc: func(i int, b []byte) (int, error) {
				<-close // block until close
				return 0, readWriteErr
			},
			CloseChan: close,
		}
	}

	// return a mocked SSH client when dialing via SSH
	sshClient := &testutils.MockSSHClient{
		DialFunc: func(i int, n, addr string) (net.Conn, error) {
			return newBlockingConn(), nil
		},
	}
	defer setAndDeferSSHDial(mockSSHDial(sshClient))()

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

	// the tunnel to test
	errChan := make(chan error, 1)
	tun := &Tunnel{Local: tcpAddr, SSH: tcpAddr, Remote: tcpAddr, Config: &ssh.ClientConfig{}, ErrChan: errChan}

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

// An error returned by the SSH Dial fails in the call to Serve.
func TestSSHDialError(t *testing.T) {
	defer setAndDeferSSHDial(errSSHDial)()

	listener := &testutils.MockListener{}

	// the tunnel to test
	errChan := make(chan error, 1)
	tun := &Tunnel{Local: tcpAddr, SSH: tcpAddr, Remote: tcpAddr, Config: &ssh.ClientConfig{}, ErrChan: errChan}

	// start the tunnel in a goroutine and close it after a while
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if err := tun.Serve(ctx, listener); errors.Cause(err) != io.EOF {
		t.Errorf("want io.EOF, got %v", err)
	}

	duration := time.Since(start)
	want := time.Duration(0)
	if duration < want || duration > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, duration)
	}

	if n := listener.CloseCalls(); n != 0 {
		t.Errorf("want Listener.Close to be called 0 times, got %v", n)
	}
	if n := listener.AcceptCalls(); n != 0 {
		t.Errorf("want Listener.Accept to be called 0 times, got %v", n)
	}
}

// An error returned by the server SSH client Dial call closes the local
// connection.
func TestServerDialError(t *testing.T) {
	sshErr := errors.New("ssh")
	sshClient := &testutils.MockSSHClient{
		DialFunc: func(i int, n, addr string) (net.Conn, error) {
			return nil, sshErr
		},
	}
	defer setAndDeferSSHDial(mockSSHDial(sshClient))()

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

	// the tunnel to test
	errChan := make(chan error, 1)
	tun := &Tunnel{Local: tcpAddr, SSH: tcpAddr, Remote: tcpAddr, Config: &ssh.ClientConfig{}, ErrChan: errChan}

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
	defer setAndDeferSSHDial(mockSSHDial(sshClient))()

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

	// the tunnel to test
	errChan := make(chan error, 1)
	tun := &Tunnel{Local: tcpAddr, SSH: tcpAddr, Remote: tcpAddr, Config: &ssh.ClientConfig{}, ErrChan: errChan}

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

func TestServeAlreadyServing(t *testing.T) {
	t.Skip()
}

func TestServeClosed(t *testing.T) {
	t.Skip()
}

func TestTouchNil(t *testing.T) {
	t.Skip()
}

func TestTouchStarted(t *testing.T) {
	t.Skip()
}
