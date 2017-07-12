package server

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"

	"bitbucket.org/harfangapps/regis-companion/addr"
	"bitbucket.org/harfangapps/regis-companion/internal/testutils"
	"bitbucket.org/harfangapps/regis-companion/resp"
	"bitbucket.org/harfangapps/regis-companion/sshconfig"
	"bitbucket.org/harfangapps/regis-companion/tunnel"
	"golang.org/x/crypto/ssh"
)

func mockListenFunc(mockListener net.Listener) func(net.Addr) (net.Listener, int, error) {
	port := 40000
	return func(addr net.Addr) (net.Listener, int, error) {
		port++
		return mockListener, port, nil
	}
}

func setAndDeferListenFunc(fn func(addr net.Addr) (net.Listener, int, error)) func() {
	addr.ListenFunc = fn
	return func() {
		addr.ListenFunc = addr.Listen
	}
}

func mockSSHDial(dc tunnel.DialCloser) func(n, a string, conf *ssh.ClientConfig) (tunnel.DialCloser, error) {
	return func(n, a string, conf *ssh.ClientConfig) (tunnel.DialCloser, error) {
		return dc, nil
	}
}

func setAndDeferSSHDial(fn func(n, a string, conf *ssh.ClientConfig) (tunnel.DialCloser, error)) func() {
	tunnel.SSHDialFunc = fn
	return func() {
		tunnel.SSHDialFunc = tunnel.DefaultSSHDial
	}
}

func TestGetTunnelAddrTwiceReturnsSameAddr(t *testing.T) {
	// create the server listener, that returns the conn that will
	// send the gettunneladdr command twice.
	closeConn := make(chan struct{})
	cmd := bufferForResp(t, []string{"gettunneladdr", "root@127.0.0.1", "remote:7000"})
	var res testutils.SyncBuffer
	theConn := &testutils.MockConn{
		ReadFunc: func(i int, b []byte) (int, error) {
			if i < 2 {
				r := strings.NewReader(cmd.String())
				return r.Read(b)
			}
			<-closeConn
			return 0, io.EOF
		},
		WriteFunc: func(i int, b []byte) (int, error) {
			if i < 2 {
				return res.Write(b)
			}
			<-closeConn
			return 0, io.EOF
		},
		CloseChan: closeConn,
	}

	closeServerListener := make(chan struct{})
	serverListener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			if i == 0 {
				return theConn, nil
			}
			<-closeServerListener
			return nil, io.EOF
		},
		CloseChan: closeServerListener,
	}

	// create the listener for the tunnel (returned by the mocked
	// ListenFunc).
	closeTunnelListener := make(chan struct{})
	tunnelListener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			<-closeTunnelListener
			return nil, io.EOF
		},
		CloseChan: closeTunnelListener,
	}
	defer setAndDeferListenFunc(mockListenFunc(tunnelListener))()

	// create the SSH client returned by the mocked SSHDialFunc
	sshClient := &testutils.MockSSHClient{}
	defer setAndDeferSSHDial(mockSSHDial(sshClient))()

	srv := &Server{
		Addr:       tcpAddr,
		MetaConfig: &sshconfig.MetaConfig{KnownHostsFile: "/dev/null"},
	}

	timeout := 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	if err := srv.serve(ctx, serverListener); errors.Cause(err) != io.EOF {
		t.Errorf("want %v, got %v", io.EOF, err)
	}

	dur := time.Since(start)
	want := timeout
	if dur < want || dur > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, dur)
	}

	r := strings.NewReader(res.String())
	dec := resp.NewDecoder(r)
	v1, err := dec.Decode()
	if err != nil {
		t.Fatalf("failed to decode first response: %v", err)
	}
	v2, err := dec.Decode()
	if err != nil {
		t.Fatalf("failed to decode second response: %v", err)
	}
	if v1 != v2 {
		t.Errorf("want same address to be returned, got %v and %v", v1, v2)
	}

	if n := sshClient.CloseCalls(); n != 1 {
		t.Errorf("want SSHClient.Close to be called once, got %d", n)
	}
	if n := theConn.CloseCalls(); n != 1 {
		t.Errorf("want Conn.Close to be called once, got %d", n)
	}
}

func TestGetTunnelAddrTwiceAfterIdleTimeoutReturnsNewAddr(t *testing.T) {
	idleTimeout := 50 * time.Millisecond
	secondCall := 100 * time.Millisecond

	// create the server listener, that returns the conn that will
	// send the gettunneladdr command twice after a delay.
	closeConn := make(chan struct{})
	cmd := bufferForResp(t, []string{"gettunneladdr", "root@127.0.0.1", "remote:7000"})
	var res testutils.SyncBuffer
	theConn := &testutils.MockConn{
		ReadFunc: func(i int, b []byte) (int, error) {
			if i < 2 {
				if i == 1 {
					<-time.After(secondCall)
				}
				r := strings.NewReader(cmd.String())
				return r.Read(b)
			}
			<-closeConn
			return 0, io.EOF
		},
		WriteFunc: func(i int, b []byte) (int, error) {
			if i < 2 {
				return res.Write(b)
			}
			<-closeConn
			return 0, io.EOF
		},
		CloseChan: closeConn,
	}

	closeServerListener := make(chan struct{})
	serverListener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			if i == 0 {
				return theConn, nil
			}
			<-closeServerListener
			return nil, io.EOF
		},
		CloseChan: closeServerListener,
	}

	// create the listener for the tunnel (returned by the mocked
	// ListenFunc).
	closeTunnelListener := make(chan struct{})
	tunnelListener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			<-closeTunnelListener
			return nil, io.EOF
		},
		CloseChan: closeTunnelListener,
	}
	defer setAndDeferListenFunc(mockListenFunc(tunnelListener))()

	// create the SSH client returned by the mocked SSHDialFunc
	sshClient := &testutils.MockSSHClient{}
	defer setAndDeferSSHDial(mockSSHDial(sshClient))()

	srv := &Server{
		Addr:              tcpAddr,
		MetaConfig:        &sshconfig.MetaConfig{KnownHostsFile: "/dev/null"},
		TunnelIdleTimeout: idleTimeout,
	}

	cancelTimeout := 200 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), cancelTimeout)
	defer cancel()

	start := time.Now()
	if err := srv.serve(ctx, serverListener); errors.Cause(err) != io.EOF {
		t.Errorf("want %v, got %v", io.EOF, err)
	}

	dur := time.Since(start)
	want := cancelTimeout
	if dur < want || dur > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, dur)
	}

	r := strings.NewReader(res.String())
	dec := resp.NewDecoder(r)
	v1, err := dec.Decode()
	if err != nil {
		t.Fatalf("failed to decode first response: %v", err)
	}
	v2, err := dec.Decode()
	if err != nil {
		t.Fatalf("failed to decode second response: %v", err)
	}
	if v1 == v2 {
		t.Errorf("want different addresses to be returned, got %v and %v", v1, v2)
	}

	if n := sshClient.CloseCalls(); n != 2 {
		t.Errorf("want SSHClient.Close to be called twice, got %d", n)
	}
	if n := theConn.CloseCalls(); n != 1 {
		t.Errorf("want Conn.Close to be called once, got %d", n)
	}
}

func TestListenFuncError(t *testing.T) {
	// create the server listener, that returns the conn that will
	// send the gettunneladdr command.
	closeConn := make(chan struct{})
	cmd := bufferForResp(t, []string{"gettunneladdr", "root@127.0.0.1", "remote:7000"})
	var res testutils.SyncBuffer
	theConn := &testutils.MockConn{
		ReadFunc: func(i int, b []byte) (int, error) {
			if i == 0 {
				r := strings.NewReader(cmd.String())
				return r.Read(b)
			}
			<-closeConn
			return 0, io.EOF
		},
		WriteFunc: func(i int, b []byte) (int, error) {
			if i == 0 {
				return res.Write(b)
			}
			<-closeConn
			return 0, io.EOF
		},
		CloseChan: closeConn,
	}

	closeServerListener := make(chan struct{})
	serverListener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			if i == 0 {
				return theConn, nil
			}
			<-closeServerListener
			return nil, io.EOF
		},
		CloseChan: closeServerListener,
	}

	theErr := errors.New("listen func")
	defer setAndDeferListenFunc(func(addr net.Addr) (net.Listener, int, error) {
		return nil, 0, theErr
	})()

	srv := &Server{
		Addr:       tcpAddr,
		MetaConfig: &sshconfig.MetaConfig{KnownHostsFile: "/dev/null"},
	}

	timeout := 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	if err := srv.serve(ctx, serverListener); errors.Cause(err) != io.EOF {
		t.Errorf("want %v, got %v", io.EOF, err)
	}

	dur := time.Since(start)
	want := timeout
	if dur < want || dur > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, dur)
	}

	r := strings.NewReader(res.String())
	dec := resp.NewDecoder(r)
	v, err := dec.Decode()
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(v.(string), theErr.Error()) {
		t.Errorf("want error value to contain %q, got %q", theErr, err)
	}

	if n := theConn.CloseCalls(); n != 1 {
		t.Errorf("want Conn.Close to be called once, got %d", n)
	}
}

func TestGetTunnelAddrKillTunnel(t *testing.T) {
	// create the server listener, that returns the conn that will
	// send the gettunneladdr and killtunnel commands.
	closeConn := make(chan struct{})
	cmd1 := bufferForResp(t, []string{"gettunneladdr", "root@127.0.0.1", "remote:7000"})
	cmd2 := bufferForResp(t, []string{"killTUNNEL", "root@127.0.0.1", "remote:7000"})
	var res testutils.SyncBuffer
	theConn := &testutils.MockConn{
		ReadFunc: func(i int, b []byte) (int, error) {
			switch i {
			case 0, 2:
				r := strings.NewReader(cmd1.String())
				return r.Read(b)
			case 1:
				r := strings.NewReader(cmd2.String())
				return r.Read(b)
			default:
				<-closeConn
				return 0, io.EOF
			}
		},
		WriteFunc: func(i int, b []byte) (int, error) {
			if i < 3 {
				return res.Write(b)
			}
			<-closeConn
			return 0, io.EOF
		},
		CloseChan: closeConn,
	}

	closeServerListener := make(chan struct{})
	serverListener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			if i == 0 {
				return theConn, nil
			}
			<-closeServerListener
			return nil, io.EOF
		},
		CloseChan: closeServerListener,
	}

	// create the listener for the tunnel (returned by the mocked
	// ListenFunc).
	closeTunnelListener := make(chan struct{})
	tunnelListener := &testutils.MockListener{
		AcceptFunc: func(i int) (net.Conn, error) {
			<-closeTunnelListener
			return nil, io.EOF
		},
		CloseChan: closeTunnelListener,
	}
	defer setAndDeferListenFunc(mockListenFunc(tunnelListener))()

	// create the SSH client returned by the mocked SSHDialFunc
	sshClient := &testutils.MockSSHClient{}
	defer setAndDeferSSHDial(mockSSHDial(sshClient))()
	srv := &Server{
		Addr:       tcpAddr,
		MetaConfig: &sshconfig.MetaConfig{KnownHostsFile: "/dev/null"},
	}

	timeout := 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	if err := srv.serve(ctx, serverListener); errors.Cause(err) != io.EOF {
		t.Errorf("want %v, got %v", io.EOF, err)
	}

	dur := time.Since(start)
	want := timeout
	if dur < want || dur > (want+(10*time.Millisecond)) {
		t.Errorf("want duration of %v, got %v", want, dur)
	}

	r := strings.NewReader(res.String())
	dec := resp.NewDecoder(r)
	v1, err := dec.Decode()
	if err != nil {
		t.Fatalf("failed to decode first response: %v", err)
	}
	v2, err := dec.Decode()
	if err != nil {
		t.Fatalf("failed to decode second response: %v", err)
	}
	v3, err := dec.Decode()
	if err != nil {
		t.Fatalf("failed to decode third response: %v", err)
	}
	if v2 != "OK" {
		t.Errorf("want response 2 to be OK, got %v", v2)
	}
	if v1 == v3 {
		t.Errorf("want second address to be different, got %v and %v", v1, v3)
	}

	if n := sshClient.CloseCalls(); n != 2 {
		t.Errorf("want SSHClient.Close to be called twice, got %d", n)
	}
	if n := theConn.CloseCalls(); n != 1 {
		t.Errorf("want Conn.Close to be called once, got %d", n)
	}
}
